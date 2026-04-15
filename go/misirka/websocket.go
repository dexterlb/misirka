package misirka

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

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

	m.wsMutex.Lock()
	m.wsWriteMutexes[ws] = &sync.Mutex{}
	m.wsMutex.Unlock()

	defer func(ws *websocket.Conn) {
		m.unsubscribeWS(ws)

		m.wsMutex.Lock()
		delete(m.wsWriteMutexes, ws)
		m.wsMutex.Unlock()

		err := ws.Close()
		if err != nil {
			m.errHandler(fmt.Errorf("could not close websocket", err))
		}
	}(ws)

	for {
		_, msg, err := ws.ReadMessage()

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				m.errHandler(fmt.Errorf("error reading from websocket: %w", err))
			}
			break
		}

		m.handleWebsocketMsg(ws, msg)
	}
}

func (m *Misirka) websocketWrite(ws *websocket.Conn, data []byte) {
	mutex, ok := m.wsWriteMutexes[ws]
	if !ok {
		m.errHandler(fmt.Errorf("could not lock websocket for writing (it probably just closed)"))
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	err := ws.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write data to websocket: %w", err))
		return
	}
}

type rpcResponse struct {
	ID      *uint64     `json:"id"`
	Result  interface{} `json:"result"`
	JsonRPC string      `json:"jsonrpc"`
}

type rpcError struct {
	MErr    MErr    `json:"error"`
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
	m.websocketWrite(ws, respBytes)
}

func (m *Misirka) respondWithErr(ws *websocket.Conn, id *uint64, merr *MErr) {
	resp := &rpcError{
		JsonRPC: "2.0",
		MErr:    *merr,
		ID:      id,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		m.errHandler(fmt.Errorf("could not encode error: %w", err))
		return
	}
	m.websocketWrite(ws, respBytes)
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
	m.wsMutex.Lock()
	defer m.wsMutex.Unlock()

	if !m.checkTopicList(ws, id, topics) {
		return
	}
	for _, topic := range topics {
		tinfo := m.topics[topic]
		tinfo.WSSubscribers[ws] = struct{}{}
		if tinfo.LastVal != nil {
			wsmsg := &pubMsg{
				Topic: topic,
				Msg:   tinfo.LastVal,
			}
			mdata, err := json.Marshal(wsmsg)
			if err != nil {
				m.errHandler(fmt.Errorf("could not encode websocket message", err))
				continue
			}
			m.websocketWrite(ws, mdata)
		}
	}
	m.respond(ws, id, "ok")
}

func (m *Misirka) handleUnsubscribe(ws *websocket.Conn, topics []string, id *uint64) {
	m.wsMutex.Lock()
	defer m.wsMutex.Unlock()

	if !m.checkTopicList(ws, id, topics) {
		return
	}

	for _, topic := range topics {
		tinfo := m.topics[topic]
		if _, ok := tinfo.WSSubscribers[ws]; ok {
			delete(tinfo.WSSubscribers, ws)
		}
	}
	m.respond(ws, id, "ok")
}

func (m *Misirka) checkTopicList(ws *websocket.Conn, id *uint64, topics []string) bool {
	for _, topic := range topics {
		if _, ok := m.topics[topic]; !ok {
			m.respondWithErr(ws, id, &MErr{
				Err:  fmt.Errorf("topic %s is not available for subscribing", topic),
				Code: -37000,
			})
			return false
		}
	}
	return true
}

func (m *Misirka) unsubscribeWS(ws *websocket.Conn) {
	m.wsMutex.Lock()
	defer m.wsMutex.Unlock()

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
		m.websocketWrite(ws, mdata)
	}
}

func (m *Misirka) handleRpcCall(ws *websocket.Conn, method string, paramData []byte, id *uint64) {
	if method == "ms-subscribe" || method == "ms-unsubscribe" {
		var topics []string
		if err := json.Unmarshal(paramData, &topics); err != nil {
			m.respondWithErr(ws, id, &MErr{
				Err:  fmt.Errorf("could not parse params as list of topics: %w", err),
				Code: -37000,
			})
		}
		if method == "ms-subscribe" {
			m.handleSubscribe(ws, topics, id)
		}
		if method == "ms-unsubscribe" {
			m.handleUnsubscribe(ws, topics, id)
		}
		return
	}

	if method == "ms-ping" {
		if paramData == nil {
			paramData = []byte("\"pong\"")
		}
		m.respond(ws, id, json.RawMessage(paramData))
		return
	}

	if method == "ms-get" {
		var topic string
		if err := json.Unmarshal(paramData, &topic); err != nil {
			m.respondWithErr(ws, id, &MErr{
				Err:  fmt.Errorf("could not parse params as a single string (topic): %w", err),
				Code: -37000,
			})
		}
		tinfo, ok := m.topics[topic]
		if !ok {
			m.respondWithErr(ws, id, &MErr{
				Err:  fmt.Errorf("topic %s is not available", topic),
				Code: -37000,
			})
		}
		m.respond(ws, id, json.RawMessage(tinfo.LastVal))
		return
	}

	call, ok := m.calls[method]
	if !ok {
		m.respondWithErr(ws, id, &MErr{
			Err:  fmt.Errorf("no such method: %s", method),
			Code: -37000,
		})
		return
	}
	respData, merr := call.rawHandler(paramData)
	if merr != nil {
		m.respondWithErr(ws, id, merr)
		return
	}
	m.respond(ws, id, respData)
}
