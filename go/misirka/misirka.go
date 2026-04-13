package misirka

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"

	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
)

type Misirka struct {
	mux        *http.ServeMux
	errHandler func(error)

	prefix string

	rawJsonHandlers map[string](func(json.RawMessage) (json.RawMessage, *MErr))

	topics            map[string]*topicInfo
	subscriptionMutex sync.Mutex
}

func New(prefix string, errHandler func(error)) *Misirka {
	m := &Misirka{}
	m.prefix = prefix
	m.mux = http.NewServeMux()
	m.errHandler = errHandler
	m.rawJsonHandlers = make(map[string](func(json.RawMessage) (json.RawMessage, *MErr)))
	m.topics = make(map[string]*topicInfo)
	m.mux.HandleFunc(fmt.Sprintf("%sws", m.prefix), m.websocketHandler)
	return m
}

func (m *Misirka) AddTopic(path string) {
	assertPath(path)
	m.topics[path] = &topicInfo{
		WSSubscribers: make(map[*websocket.Conn]struct{}),
	}
	handleCallHttp(m, path, func(args *getArgs) (json.RawMessage, *MErr) {
		return json.RawMessage(m.topics[path].LastVal), nil
	})
}

func Publish[P any](m *Misirka, path string, item P) {
	info := m.topics[path]
	if info == nil {
		m.errHandler(fmt.Errorf("trying to publish to topic %s but it doesn't exist", path))
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		m.errHandler(fmt.Errorf("trying to publish to topic %s but the value failed to encode: %w", path, err))
		return
	}

	info.pubMutex.Lock()
	defer info.pubMutex.Unlock()

	if bytes.Equal(info.LastVal, data) {
		return
	}
	info.LastVal = data
	m.publishToWebsockets(path, data)
}

type Callee[P any, R any] func(param P) (R, *MErr)

func HandleCall[P any, R any](m *Misirka, path string, callee Callee[P, R]) {
	assertPath(path)
	if _, ok := m.rawJsonHandlers[path]; ok {
		panic(fmt.Sprintf("HandleCall called twice for path %s", path))
	}

	m.rawJsonHandlers[path] = func(param json.RawMessage) (json.RawMessage, *MErr) {
		return rawJsonHandler(m, callee, param)
	}

	handleCallHttp(m, path, callee)
}

func (m *Misirka) Handler() http.Handler {
	return m.mux
}

func handleCallHttp[P any, R any](m *Misirka, path string, callee Callee[P, R]) {
	fullPath := fmt.Sprintf("%s%s", m.prefix, path)
	m.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		httpCallHandler(m, callee, w, req)
	})
}

type getArgs struct {
	// TODO: let the caller issue options here
}

type topicInfo struct {
	LastVal       []byte
	WSSubscribers map[*websocket.Conn]struct{}
	pubMutex      sync.Mutex
}

func rawJsonHandler[P any, R any](m *Misirka, callee Callee[P, R], paramData json.RawMessage) (json.RawMessage, *MErr) {
	var param P

	err := json.Unmarshal(paramData, &param)
	if err != nil {
		return nil, &MErr{
			Code: -32700,
			Err:  fmt.Errorf("could not read request body: %w", err),
		}
	}

	result, merr := callee(param)
	if merr != nil {
		return nil, merr
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, &MErr{
			Code: -32700,
			Err:  fmt.Errorf("could not encode response: %w", err),
		}
	}

	return data, nil
}

func httpCallHandler[P any, R any](m *Misirka, callee Callee[P, R], w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var param P

	switch req.Method {
	case "POST":
		dec := json.NewDecoder(req.Body)
		err := dec.Decode(&param)
		if err != nil {
			m.writeError(w, &MErr{
				Code: -32700,
				Err:  fmt.Errorf("could not read request body: %w", err),
			})
			return
		}
		result, merr := callee(param)
		if merr != nil {
			m.writeError(w, merr)
			return
		}
		enc := json.NewEncoder(w)
		err = enc.Encode(result)
		if err != nil {
			m.writeError(w, &MErr{
				Code: -32700,
				Err:  fmt.Errorf("could not encode data: %w", err),
			})
			return
		}
	case "GET":
		if len(req.URL.Query()) != 0 {
			paramMap := make(map[string]string)
			for k, vals := range req.URL.Query() {
				if len(vals) != 1 {
					m.writeError(w, &MErr{
						Code: -32700,
						Err:  fmt.Errorf("parameter %s specified more than once, refusing to process", k),
					})
					return
				}
				paramMap[k] = vals[0]
			}
			paramJson, _ := json.Marshal(paramMap)
			err := json.Unmarshal(paramJson, &param)
			if err != nil {
				m.errHandler(fmt.Errorf("could not decode stringmap from URL query: %w", err))
				return
			}
		}
		result, merr := callee(param)
		if merr != nil {
			m.writeError(w, merr)
			return
		}
		enc := json.NewEncoder(w)
		err := enc.Encode(result)
		if err != nil {
			m.errHandler(fmt.Errorf("could not write response: %w", err))
			return
		}
	}
}

func (m *Misirka) writeError(w http.ResponseWriter, merr *MErr) {
	enc := json.NewEncoder(w)
	err := enc.Encode(merr)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write error response: %w", err))
	}
}
