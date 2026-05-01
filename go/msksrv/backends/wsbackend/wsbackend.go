package wsbackend

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/goccy/go-json"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool {
		return true
	},
}

type WSBackend struct {
	opMutex    sync.Mutex
	topics     map[string]mskbus.Bus
	calls      map[string]backends.CallHandler
	errHandler func(error)
}

func New(errHandler func(error)) *WSBackend {
	return &WSBackend{
		topics:     make(map[string]mskbus.Bus),
		calls:      make(map[string]backends.CallHandler),
		errHandler: errHandler,
	}
}

func (w *WSBackend) AddTopic(path string, bus mskbus.Bus) {
	w.opMutex.Lock()
	defer w.opMutex.Unlock()
	w.topics[path] = bus
}

func (w *WSBackend) AddCallR(path string, handler backends.CallHandler) {
	w.opMutex.Lock()
	defer w.opMutex.Unlock()
	w.calls[path] = handler
}

func (w *WSBackend) WSHTTPHandler() http.Handler {
	return http.HandlerFunc(w.handleWebsocket)
}

func (w *WSBackend) AddPathValueCallHandler(pathWithWildcards string, handler backends.CallHandler) {
	// do nothing (the websocket backend doesn't support these aliases)
}

func (w *WSBackend) handleWebsocket(writer http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(writer, req, nil)
	if err != nil {
		// FIXME: the following call outputs a `superflous response.WriteHeader call`
		// warning, we probably should not be calling http.Error because
		// the upgrader probably sets the error header itself, but we should
		// investigate to be sure
		http.Error(writer, fmt.Sprintf("couldn't make websocket: %s", err), 400)
		return
	}

	conn := newConnInfo(ws, w.errHandler)

	defer func(conn *connInfo) {
		err := conn.Cleanup()
		if err != nil {
			w.errHandler(fmt.Errorf("cleanup failed: %w", err))
		}
	}(conn)

	for {
		_, msg, err := ws.ReadMessage()

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				w.errHandler(fmt.Errorf("error reading from websocket: %w", err))
			}
			break
		}

		w.handleWebsocketMsg(conn, msg)
	}
}

type rpcResponse struct {
	ID      *uint64     `json:"id"`
	Result  interface{} `json:"result"`
	JsonRPC string      `json:"jsonrpc"`
}

type rpcError struct {
	MErr    mskdata.Error `json:"error"`
	ID      *uint64       `json:"id"`
	JsonRPC string        `json:"jsonrpc"`
}

type rpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     *uint64         `json:"id"`
}

func (w *WSBackend) handleWebsocketMsg(conn *connInfo, message []byte) {
	var msg rpcRequest
	if err := json.Unmarshal(message, &msg); err != nil {
		conn.RespondWithErr(nil, mskdata.Errorf(-37000, "could not decode message: %w", err))
		return
	}

	if msg.Method == "" {
		conn.RespondWithErr(msg.ID, mskdata.Errorf(-37000, "method name unspecified"))
		return
	}

	w.handleRpcCall(conn, msg.Method, []byte(msg.Params), msg.ID)
}

func (w *WSBackend) handleSubscribe(conn *connInfo, topics []string, id *uint64) {
	w.opMutex.Lock()
	defer w.opMutex.Unlock()

	if !w.checkTopicList(conn, id, topics, false) {
		return
	}

	for _, topic := range topics {
		conn.Subscribe(topic, w.topics[topic])
	}

	conn.Respond(id, "ok")
}

func (w *WSBackend) handleUnsubscribe(conn *connInfo, topics []string, id *uint64) {
	w.opMutex.Lock()
	defer w.opMutex.Unlock()

	if !w.checkTopicList(conn, id, topics, true) {
		return
	}

	for _, topic := range topics {
		conn.Unsubscribe(topic)
	}

	conn.Respond(id, "ok")
}

func (w *WSBackend) checkTopicList(conn *connInfo, id *uint64, topics []string, mustExist bool) bool {
	for _, topic := range topics {
		if _, ok := w.topics[topic]; !ok {
			conn.RespondWithErr(id, mskdata.Errorf(-37000, "topic %s is not available", topic))
			return false
		}
		if _, ok := conn.Subscriptions[topic]; ok != mustExist {
			if mustExist {
				conn.RespondWithErr(id, mskdata.Errorf(-37000, "topic %s is not subscribed", topic))
			} else {
				conn.RespondWithErr(id, mskdata.Errorf(-37000, "topic %s is already subscribed", topic))
			}
			return false
		}
	}
	return true
}

func (w *WSBackend) handleRpcCall(conn *connInfo, method string, paramData []byte, id *uint64) {
	if method == "ms-subscribe" || method == "ms-unsubscribe" {
		var topics []string
		if err := json.Unmarshal(paramData, &topics); err != nil {
			conn.RespondWithErr(id, mskdata.Errorf(-37000, "could not parse params as list of topics: %w", err))
		}
		if method == "ms-subscribe" {
			w.handleSubscribe(conn, topics, id)
		}
		if method == "ms-unsubscribe" {
			w.handleUnsubscribe(conn, topics, id)
		}
		return
	}

	respond := func(x interface{}) {
		conn.Respond(id, x)
	}

	if method == "ms-ping" {
		if paramData == nil {
			paramData = []byte("\"pong\"")
		}
		respond(json.RawMessage(paramData))
		return
	}

	if method == "ms-get" {
		var topic string
		if err := json.Unmarshal(paramData, &topic); err != nil {
			conn.RespondWithErr(id, mskdata.Errorf(-37000, "could not parse params as a single string (topic): %w", err))
		}
		bus, ok := w.topics[topic]
		if !ok {
			conn.RespondWithErr(id, mskdata.Errorf(-37000, "topic %s is not available", topic))
		}
		bus.UseT(respond)
		return
	}

	decoder := func(param any) error {
		return json.Unmarshal(paramData, param)
	}

	call, ok := w.calls[method]
	if !ok {
		conn.RespondWithErr(id, mskdata.Errorf(-37000, "no such method: %s", method))
		return
	}
	err := call(decoder, respond)
	if err != nil {
		conn.RespondWithErr(id, mskdata.GetError(err))
		return
	}
}
