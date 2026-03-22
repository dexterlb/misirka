package misirka

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool {
		return true
	},
}

func (m *Misirka) websocketHandler(w http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("couldn't make websocket: %s", err), 400)
		return
	}
	defer func(ws *websocket.Conn) {
		m.unsubscribeWS(ws)
		err := ws.Close()
		if err != nil {
			m.errHandler(fmt.Errorf("could not close websocket", err))
		}
	}(ws)

	for {
		_, msg, err := ws.ReadMessage()

		if err != nil {
			m.errHandler(fmt.Errorf("error reading from websocket: %w", err))
			break
		}

		m.handleWebsocketMsg(ws, msg)
	}
}

type rpcResponse struct {
	ID      *uint64 `json:"id"`
	Result  interface{}
	JsonRPC string `json:"jsonrpc"`
}

type rpcError struct {
	MErr    `json:"error"`
	ID      *uint64 `json:"id"`
	JsonRPC string  `json:"jsonrpc"`
}

type rpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     *uint64         `json:"id"`
}

func (m *Misirka) respond(ws *websocket.Conn, id *uint64, result interface{}) {
	resp := &rpcResponse{
		JsonRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		m.respondWithErr(ws, id, &MErr{
			Err:  fmt.Errorf("could not encode response: %w", err),
			Code: -37000,
		})
		return
	}
	err = ws.WriteMessage(websocket.TextMessage, respBytes)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write error to websocket: %w", err))
		return
	}
}

func (m *Misirka) respondWithErr(ws *websocket.Conn, id *uint64, merr *MErr) {
	resp := &rpcError{
		JsonRPC: "2.0",
		MErr:    *merr,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		m.errHandler(fmt.Errorf("could not encode error: %w", err))
		return
	}
	err = ws.WriteMessage(websocket.TextMessage, respBytes)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write error to websocket: %w", err))
		return
	}
}

func (m *Misirka) handleWebsocketMsg(ws *websocket.Conn, message []byte) {
	var msg rpcRequest
	if err := json.Unmarshal(message, &msg); err != nil {
		m.respondWithErr(ws, nil, &MErr{
			Err:  err,
			Code: -37000,
		})
		return
	}

	if msg.Method == "" {
		m.respondWithErr(ws, msg.ID, &MErr{
			Err:  fmt.Errorf("method name unspecified"),
			Code: -37000,
		})
		return
	}

	m.handleRpcCall(ws, msg.Method, []byte(msg.Params), msg.ID)
}

func (m *Misirka) handleSubscribe(ws *websocket.Conn, topics []string, id *uint64) {
	m.subscriptionMutex.Lock()
	defer m.subscriptionMutex.Unlock()

	for _, topic := range topics {
		if _, ok := m.topics[topic]; !ok {
			m.respondWithErr(ws, id, &MErr{
				Err:  fmt.Errorf("topic %s is not available for subscribing", topic),
				Code: -37000,
			})
			return
		}
	}
	for _, topic := range topics {
		tinfo := m.topics[topic]
		tinfo.WSSubscribers[ws] = struct{}{}
	}
	m.respond(ws, id, "ok")
}

func (m *Misirka) unsubscribeWS(ws *websocket.Conn) {
	m.subscriptionMutex.Lock()
	defer m.subscriptionMutex.Unlock()

	for _, tinfo := range m.topics {
		if _, ok := tinfo.WSSubscribers[ws]; ok {
			delete(tinfo.WSSubscribers, ws)
		}
	}
}

type pubMsg struct {
	Topic string          `json:"topic"`
	Msg   json.RawMessage `json:"msg"`
}

func (m *Misirka) publishToWebsockets(topic string, msg []byte) {
	wsmsg := &pubMsg{
		Topic: topic,
		Msg:   msg,
	}
	mdata, err := json.Marshal(wsmsg)
	if err != nil {
		m.errHandler(fmt.Errorf("could not encode websocket message", err))
		return
	}

	for ws := range m.topics[topic].WSSubscribers {
		err = ws.WriteMessage(websocket.TextMessage, mdata)
		if err != nil {
			m.errHandler(fmt.Errorf("could not write message to websocket (topic %s): %w", topic, err))
			return
		}
	}
}

func (m *Misirka) handleRpcCall(ws *websocket.Conn, method string, data []byte, id *uint64) {
	if method == "ms-subscribe" {
		var topics []string
		if err := json.Unmarshal(data, &topics); err != nil {
			m.respondWithErr(ws, id, &MErr{
				Err:  fmt.Errorf("could not parse params as list of topics: %w", err),
				Code: -37000,
			})
		}
		m.handleSubscribe(ws, topics, id)
		return
	}
	panic("not implemented")
}
