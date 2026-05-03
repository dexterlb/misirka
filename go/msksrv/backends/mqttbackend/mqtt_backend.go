package mqttbackend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/goccy/go-json"

	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type Cfg struct {
	BrokerURL   string `yaml:"broker_url"`
	ClientID    string `yaml:"client_id"`
	Prefix      string `yaml:"prefix"`
	OnlineTopic string `yaml:"online_topic"`
}

type MQTTBackend struct {
	cfg         *Cfg
	evtHandlers backends.EventHandlers
	conn        *autopaho.ConnectionManager
	topics      map[string]*backends.TopicInfo
	calls       map[string]*backends.CallInfo
	connected   bool
	ctx         context.Context
}

func New(cfg *Cfg, evtHandlers backends.EventHandlers) *MQTTBackend {
	if cfg.OnlineTopic == "" {
		cfg.OnlineTopic = "online"
	}

	m := &MQTTBackend{
		cfg:         cfg,
		evtHandlers: evtHandlers,
		topics:      make(map[string]*backends.TopicInfo),
		calls:       make(map[string]*backends.CallInfo),
	}

	return m
}

func (m *MQTTBackend) Start(ctx context.Context) error {
	brokerUrl, err := url.Parse(m.cfg.BrokerURL)
	if err != nil {
		return fmt.Errorf("could not parse URL %s: %w", m.cfg.BrokerURL, err)
	}

	m.ctx = ctx

	cliCfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{brokerUrl},
		KeepAlive:                     20, // seconds
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         0, // do not queue messages while connection is down
		OnConnectionUp:                m.handleConnUp,
		OnConnectError:                m.handleConnError,
		ClientConfig: paho.ClientConfig{
			ClientID:           m.cfg.ClientID,
			OnPublishReceived:  []func(paho.PublishReceived) (bool, error){m.handleIncomingMsg},
			OnClientError:      m.handleClientError,
			OnServerDisconnect: m.handleServerDisconnect,
		},
		WillMessage: m.willMsg(),
	}

	m.evtHandlers.Info("Starting MQTT connection", map[string]interface{}{
		"broker_url": brokerUrl,
	})

	m.conn, err = autopaho.NewConnection(ctx, cliCfg)
	if err != nil {
		return err
	}
	return nil
}

func (m *MQTTBackend) Done() <-chan struct{} {
	return m.conn.Done()
}

func (m *MQTTBackend) AddTopic(path string, tinfo *backends.TopicInfo) {
	path = m.cfg.Prefix + path
	m.topics[path] = tinfo
	tinfo.Bus.SubscribeT(func(data interface{}) {
		m.publishOn(path, data)
	})
}

func (m *MQTTBackend) AddCall(path string, call *backends.CallInfo) {
	path = m.cfg.Prefix + path
	m.calls[path] = call
}

func (m *MQTTBackend) publishOn(path string, x any) {
	if !m.connected {
		return // all values will be published later, on connect
	}

	pub := &paho.Publish{
		QoS:    0,
		Topic:  path,
		Retain: true,
	}

	err := m.encodeInto(x, pub)
	if err != nil {
		m.errorf("could not encode value on topic %s: %w", path, x)
		return
	}

	m.publish(pub)
}

func (m *MQTTBackend) publish(pub *paho.Publish) {
	_, err := m.conn.Publish(m.ctx, pub)
	if err != nil {
		m.errorf("could not publish message on topic %s: %w", pub.Topic, err)
		return
	}
}

func (m *MQTTBackend) sendInitialTopicStates() {
	for path, tinfo := range m.topics {
		tinfo.Bus.UseT(func(data interface{}) {
			m.publishOn(path, data)
		})
	}
}

func (m *MQTTBackend) subscribeToCallRequests() {
	subs := make([]paho.SubscribeOptions, 0, len(m.calls))
	for path, _ := range m.calls {
		sub := paho.SubscribeOptions{
			Topic: path,
			QoS:   0,
		}
		subs = append(subs, sub)
	}

	_, err := m.conn.Subscribe(m.ctx, &paho.Subscribe{
		Subscriptions: subs,
	})

	if err != nil {
		m.errorf("Could not subscribe to call topics: %w; this is very bad: calls over MQTT will not work", err)
	}
}

func (m *MQTTBackend) handleConnUp(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
	m.evtHandlers.Info("MQTT connection up", map[string]interface{}{})
	m.connected = true
	m.sendInitialTopicStates()
	m.publishOn(m.onlineTopic(), true)
	m.subscribeToCallRequests()
}

func (m *MQTTBackend) handleCall(pr paho.PublishReceived) {
	respData, err := getRespData(pr)
	if err != nil {
		m.evtHandlers.Err(err)
		return
	}

	call, ok := m.calls[pr.Packet.Topic]
	if !ok {
		m.respondWithErr(respData, mskdata.Errorf(-32700, "no such method: %s", pr.Packet.Topic))
		return
	}

	decoder := func(param any) error {
		if raw, ok := param.(*mskdata.RawData); ok {
			raw.MimeType = pr.Packet.Properties.ContentType
			raw.Data = bytes.NewReader(pr.Packet.Payload)
			return nil
		} else {
			return json.Unmarshal(pr.Packet.Payload, param)
		}
	}

	respond := func(x any) {
		m.respond(respData, x)
	}

	handle := func() {
		err := call.Handler(decoder, respond)
		if err != nil {
			m.respondWithErr(respData, mskdata.GetError(err))
		}
	}

	if call.Async {
		go handle()
	} else {
		handle()
	}
}

func (m *MQTTBackend) respond(respData *respData, x any) {
	var pub paho.Publish

	pub.Topic = respData.Topic
	pub.Properties = &paho.PublishProperties{
		CorrelationData: []byte(respData.Nonce + ".result"),
	}
	pub.Retain = false

	err := m.encodeInto(x, &pub)
	if err != nil {
		m.respondWithErr(respData, mskdata.Errorf(-32700, "could not encode value: %w", err))
		return
	}

	m.publish(&pub)
}

func (m *MQTTBackend) respondWithErr(respData *respData, merr *mskdata.Error) {
	var pub paho.Publish

	pub.Topic = respData.Topic
	pub.Properties = &paho.PublishProperties{
		CorrelationData: []byte(respData.Nonce + ".error"),
	}
	pub.Retain = false

	err := m.encodeInto(merr, &pub)
	if err != nil {
		m.errorf(
			"could not encode error response (original topic %s) to send on topic %s: %w",
			respData.CallTopic,
			respData.Topic,
			err,
		)
		return
	}

	m.publish(&pub)
}

func (m *MQTTBackend) encodeInto(x any, pub *paho.Publish) error {
	if pub.Properties == nil {
		pub.Properties = &paho.PublishProperties{}
	}

	var err error
	if raw, ok := x.(*mskdata.RawData); ok {
		pub.Properties.ContentType = raw.MimeType
		pub.Payload, err = io.ReadAll(raw.Data)
	} else {
		pub.Properties.ContentType = "application/json"
		pub.Payload, err = json.Marshal(x)
	}
	return err
}

func (m *MQTTBackend) handleServerDisconnect(d *paho.Disconnect) {
	m.connected = false
	if d.Properties != nil {
		m.errorf("server requested disconnect: %s\n", d.Properties.ReasonString)
	} else {
		m.errorf("server requested disconnect; reason code: %d\n", d.ReasonCode)
	}
}

func (m *MQTTBackend) handleClientError(err error) {
	m.errorf("MQTT client error: %w", err)
}

func (m *MQTTBackend) handleConnError(err error) {
	m.connected = false
	m.errorf("MQTT connection error: %w", err)
}

func (m *MQTTBackend) handleIncomingMsg(pr paho.PublishReceived) (bool, error) {
	m.handleCall(pr)
	return true, nil
}

func (m *MQTTBackend) willMsg() *paho.WillMessage {
	return &paho.WillMessage{
		Retain:  true,
		Payload: []byte("false"), // FIXME: properly encode this
		Topic:   m.onlineTopic(),
		QoS:     0,
	}
}

func (m *MQTTBackend) errorf(msg string, args ...any) {
	m.evtHandlers.Err(fmt.Errorf(msg, args...))
}

func (m *MQTTBackend) onlineTopic() string {
	return m.cfg.Prefix + m.cfg.OnlineTopic
}
