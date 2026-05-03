package mqttbackend

import (
	"context"
	"fmt"
	"net/url"

	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type Cfg struct {
	BrokerURL string `yaml:"broker_url"`
	ClientID  string `yaml:"client_id"`
}

type MQTTBackend struct {
	cfg        *Cfg
	errHandler func(error)
	conn       *autopaho.ConnectionManager
}

func New(cfg *Cfg, errHandler func(error)) *MQTTBackend {
	m := &MQTTBackend{cfg: cfg, errHandler: errHandler}

	return m
}

func (m *MQTTBackend) Start(ctx context.Context) error {
	brokerUrl, err := url.Parse(m.cfg.BrokerURL)
	if err != nil {
		return fmt.Errorf("could not parse URL %s: %w", m.cfg.BrokerURL, err)
	}

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
	}

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
	// implement me
}

func (m *MQTTBackend) AddCall(path string, call *backends.CallInfo) {
	// implement me
}

func (m *MQTTBackend) handleConnUp(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
	fmt.Println("mqtt connection up")
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
	if d.Properties != nil {
		m.errHandler(fmt.Errorf("server requested disconnect: %s\n", d.Properties.ReasonString))
	} else {
		m.errHandler(fmt.Errorf("server requested disconnect; reason code: %d\n", d.ReasonCode))
	}
}

func (m *MQTTBackend) handleClientError(err error) {
	m.errHandler(fmt.Errorf("MQTT client error: %w", err))
}

func (m *MQTTBackend) handleConnError(err error) {
	m.errHandler(fmt.Errorf("MQTT connection error: %w", err))
}

func (m *MQTTBackend) handleIncomingMsg(pr paho.PublishReceived) (bool, error) {
	fmt.Printf("received message on topic %s; body: %s (retain: %t)\n", pr.Packet.Topic, pr.Packet.Payload, pr.Packet.Retain)
	return true, nil
}
