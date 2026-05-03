package mqttbackend

import (
	"context"
	"fmt"
	"net/url"

	"github.com/goccy/go-json"

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
	// implement me
}

func (m *MQTTBackend) publishOn(path string, data interface{}) {
	if !m.connected {
		return // all values will be published later, on connect
	}

	jdata, err := json.Marshal(data)
	if err != nil {
		m.errorf("could not encode message: %w", err)
		return
	}

	pub := &paho.Publish{
		QoS:     0,
		Topic:   path,
		Payload: jdata,
		Retain:  true,
	}

	_, err = m.conn.Publish(m.ctx, pub)
	if err != nil {
		m.errorf("could not publish message on topic %s: %w", path, err)
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

func (m *MQTTBackend) handleConnUp(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
	m.evtHandlers.Info("MQTT connection up", map[string]interface{}{})
	m.connected = true
	m.sendInitialTopicStates()
	m.publishOn(m.onlineTopic(), true)
	// if _, err := cm.Subscribe(context.Background(), &paho.Subscribe{
	// 	Subscriptions: []paho.SubscribeOptions{
	// 		{Topic: topic, QoS: 1},
	// 	},
	// }); err != nil {
	// 	fmt.Printf("failed to subscribe (%s). This is likely to mean no messages will be received.", err)
	// }
	// fmt.Println("mqtt subscription made")
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
	fmt.Printf("received message on topic %s; body: %s (retain: %t)\n", pr.Packet.Topic, pr.Packet.Payload, pr.Packet.Retain)
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
