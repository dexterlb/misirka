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

		err = m.handleWebsocketMsg(ws, msg)
		if err != nil {
			m.respondWithErr(ws, &MErr{
				Err:  err,
				Code: -37000,
			})
		}
	}
}

type rpcError struct {
	MErr    `json:"error"`
	JsonRPC string `json:"jsonrpc"`
}

type rpcMessage struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     uint64          `json:"id"`
}

func (m *Misirka) respondWithErr(ws *websocket.Conn, merr *MErr) {
	resp := &rpcError{
		JsonRPC: "2.0",
		MErr:    *merr,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		m.errHandler(fmt.Errorf("could not encode error: %w", err))
	}
	err = ws.WriteMessage(websocket.TextMessage, respBytes)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write error to websocket: %w", err))
	}
}

func (m *Misirka) handleWebsocketMsg(ws *websocket.Conn, message []byte) error {
	var msg rpcMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("failed to parse JSON-RPC message: %w", err)
	}

	if msg.Method == "" {
		return fmt.Errorf("missing method field")
	}

	if msg.Method == "ms-subscribe" {
		var topics []string
		if err := json.Unmarshal(msg.Params, &topics); err != nil {
			return fmt.Errorf("failed to decode subscribe params as []string: %w", err)
		}
		m.handleSubscribe(ws, topics)
		return nil
	}

	m.handleRpcCall(ws, msg.Method, []byte(msg.Params), msg.ID)
	return nil
}

func (m *Misirka) handleSubscribe(ws *websocket.Conn, topics []string) {
	panic("not implemented")
}

func (m *Misirka) handleRpcCall(ws *websocket.Conn, msg string, data []byte, id uint64) {
	panic("not implemented")
}
