package wsbackend

import (
	"fmt"
	"sync"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
)

type connInfo struct {
	WriteMutex    sync.Mutex
	Subscriptions map[string]subscription
	WS            *websocket.Conn
	errHandler    func(error)
}

func newConnInfo(ws *websocket.Conn, errHandler func(error)) *connInfo {
	return &connInfo{
		WS:            ws,
		Subscriptions: make(map[string]subscription),
		errHandler:    errHandler,
	}
}

type subscription struct {
	Bus   mskbus.Bus
	Token uint
}

func (s *subscription) Unsubscribe() {
	s.Bus.Unsubscribe(s.Token)
}

func (c *connInfo) UnsubscribeAll() {
	for _, sub := range c.Subscriptions {
		sub.Unsubscribe()
	}
	clear(c.Subscriptions)
}

func (c *connInfo) Subscribe(topic string, bus mskbus.Bus) {
	handler := func(msg interface{}) {
		c.publishTopicMsg(topic, msg)
	}
	c.Subscriptions[topic] = subscription{
		Bus:   bus,
		Token: bus.SubscribeT(handler),
	}
	return
}

func (c *connInfo) Unsubscribe(topic string) error {
	sub, ok := c.Subscriptions[topic]
	if !ok {
		return fmt.Errorf("could not unsubscribe topic %s (it is not subscribed)", topic)
	}
	sub.Unsubscribe()
	delete(c.Subscriptions, topic)
	return nil
}

func (c *connInfo) Cleanup() error {
	c.UnsubscribeAll()

	err := c.WS.Close()
	if err != nil {
		return fmt.Errorf("could not close websocket: %w", err)
	}

	return nil
}

func (c *connInfo) Send(data []byte) {
	c.WriteMutex.Lock()
	defer c.WriteMutex.Unlock()

	err := c.WS.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		c.errHandler(fmt.Errorf("could not write data to websocket: %w", err))
		return
	}
}

type pubMsg struct {
	Topic string      `json:"topic"`
	Msg   interface{} `json:"msg"`
}

func (c *connInfo) publishTopicMsg(topic string, msg interface{}) {
	wsmsg := &pubMsg{
		Topic: topic,
		Msg:   msg,
	}
	mdata, err := json.Marshal(wsmsg)
	if err != nil {
		c.errHandler(fmt.Errorf("could not encode websocket message: %w", err))
		return
	}

	c.Send(mdata)
}

func (c *connInfo) Respond(id *uint64, result interface{}) {
	resp := &rpcResponse{
		JsonRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		c.RespondWithErr(id, &mskdata.Error{
			Err:  fmt.Errorf("could not encode response: %w", err),
			Code: -37000,
		})
		return
	}
	c.Send(respBytes)
}

func (c *connInfo) RespondWithErr(id *uint64, merr *mskdata.Error) {
	resp := &rpcError{
		JsonRPC: "2.0",
		MErr:    *merr,
		ID:      id,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		c.errHandler(fmt.Errorf("could not encode error: %w", err))
		return
	}
	c.Send(respBytes)
}
