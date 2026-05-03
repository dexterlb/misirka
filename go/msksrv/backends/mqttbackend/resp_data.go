package mqttbackend

import (
	"fmt"

	"github.com/eclipse/paho.golang/paho"
)

type respData struct {
	Topic     string // the response topic
	Nonce     string // token that is used to identify this particular request
	CallTopic string // the original topic used for the call
}

func getRespData(pr paho.PublishReceived) (*respData, error) {
	if pr.Packet.Properties == nil {
		return nil, fmt.Errorf("received a call request (topic %s) with properties", pr.Packet.Topic)
	}

	respTopic := pr.Packet.Properties.ResponseTopic
	if respTopic == "" {
		return nil, fmt.Errorf("received a call request (topic %s) with no response topic set", pr.Packet.Topic)
	}
	for _, b := range pr.Packet.Properties.CorrelationData {
		if b == 0 || b > 127 {
			return nil, fmt.Errorf("non-ascii symbol in correlation data field - daring today, aren't we?")
		}
	}
	nonce := string(pr.Packet.Properties.CorrelationData)

	return &respData{
		CallTopic: pr.Packet.Topic,
		Topic:     respTopic,
		Nonce:     nonce,
	}, nil
}
