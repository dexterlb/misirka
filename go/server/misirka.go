package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/dexterlb/misirka/go/bus"
	"github.com/dexterlb/misirka/go/data"
	"github.com/dexterlb/misirka/go/server/backends"
	"github.com/dexterlb/misirka/go/server/backends/wsbackend"
	"github.com/goccy/go-json"
)

type Misirka struct {
	mux        *http.ServeMux
	errHandler func(error)
	apiDescr   string

	calls  map[string]*callInfo
	topics map[string]*topicInfo

	backends []backends.Backend

	// TODO: instead of doing this, make a "misirkamaker" function that
	// initialises all backends and then passes them to a new Misirka
	wsBackend *wsbackend.WSBackend
}

func New(errHandler func(error)) *Misirka {
	m := &Misirka{}
	m.mux = http.NewServeMux()
	m.errHandler = errHandler
	m.topics = make(map[string]*topicInfo)
	m.calls = make(map[string]*callInfo)
	m.wsBackend = wsbackend.New(errHandler)
	m.backends = []backends.Backend{m.wsBackend}
	return m
}

func AddTopic[T any](m *Misirka, path string) *TopicMeta[T] {
	b := bus.New[T]()
	return AddTopicWith(m, path, b)
}

func AddTopicWith[T any](m *Misirka, path string, bus *bus.BusOf[T]) *TopicMeta[T] {
	assertPath(path)

	for _, backend := range m.backends {
		backend.AddTopic(path, bus)
	}

	// TODO: move into http backend
	info := &topicInfo{}
	m.topics[path] = info
	handleCallHttp(m, path, func(args *getArgs) (interface{}, *data.Error) {
		return bus.GetT(), nil
	})

	return &TopicMeta[T]{info: info, bus: bus}
}

type Callee[P any, R any] func(param P) (R, *data.Error)

func AddCall[P any, R any](m *Misirka, path string, callee Callee[P, R]) *CallMeta[P, R] {
	assertPath(path)
	if _, ok := m.calls[path]; ok {
		panic(fmt.Sprintf("AddCall called twice for path %s", path))
	}

	handler := func(param json.RawMessage) (json.RawMessage, *data.Error) {
		return rawJsonHandler(m, callee, param)
	}

	for _, backend := range m.backends {
		backend.AddCall(path, handler)
	}

	call := &callInfo{
		rawHandler: handler,
	}

	m.calls[path] = call
	handleCallHttp(m, path, callee)

	return &CallMeta[P, R]{m: m, info: call, callee: callee}
}

type callInfo struct {
	rawHandler (func(json.RawMessage) (json.RawMessage, *data.Error))
	doc        callDoc
}

func (m *Misirka) HTTPHandler() http.Handler {
	return m.mux
}

// HandleWebsocket registers a websocket handler under `/ws`. To use another
// URL for the websocket, use `HandleWebsocketAt()`. To handle the Misirka
// websocket manually, use `WebsocketHandler()`.
func (m *Misirka) HandleWebsocket() {
	m.HandleWebsocketAt("/ws")
}

// HandleWebsocket registers a websocket handler under the given url.
// The URL should begin with a leading slash and is handled at http level.
// To handle the Misirka websocket manually, use `WebsocketHandler()`.
func (m *Misirka) HandleWebsocketAt(url string) {
	m.mux.Handle(url, m.wsBackend.WSHTTPHandler())
}

type CallMeta[P any, R any] struct {
	info   *callInfo
	m      *Misirka
	callee Callee[P, R]
}

type TopicMeta[T any] struct {
	info *topicInfo
	bus  *bus.BusOf[T]
}

func (t *TopicMeta[T]) Bus() *bus.BusOf[T] {
	return t.bus
}

func (m *Misirka) HandleDoc() {
	m.HandleDocAt("doc", "doc.html")
}

func (m *Misirka) HandleDocAt(path string, htmlPath string) {
	doc := &fullDoc{
		APIDescr: m.apiDescr,
		Topics:   make(map[string]*topicDoc),
		Calls:    make(map[string]*callDoc),
	}
	for tp := range m.topics {
		doc.Topics[tp] = &m.topics[tp].doc
	}
	for cp := range m.calls {
		doc.Calls[cp] = &m.calls[cp].doc
	}

	doc.Validate()

	handleDoc := func(arg struct{}) (*fullDoc, *data.Error) {
		return doc, nil
	}

	htmlgz, err := m.docHTMLgz(doc)
	if err != nil {
		panic(fmt.Sprintf("documentation doesn't render, %s", err))
	}

	handleDocHTMLgz := func(arg struct{}) (*data.RawData, *data.Error) {
		return &data.RawData{
			Data:            bytes.NewReader(htmlgz),
			MimeType:        "text/html",
			ContentEncoding: "gzip",
		}, nil
	}

	exampleDoc := &fullDoc{APIDescr: "<this documentation>"}

	AddCall(m, path, handleDoc).
		Descr("get documentation for this API").
		Example(struct{}{}, exampleDoc)

	if htmlPath != "" {
		AddCall(m, htmlPath, handleDocHTMLgz).
			Descr("get documentation for this API in human-readeble HTML")
	}
}

func handleCallHttp[P any, R any](m *Misirka, path string, callee Callee[P, R]) {
	fullPath := fmt.Sprintf("/%s", path)
	m.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		httpCallHandler(m, callee, w, req)
	})
}

func (c *CallMeta[P, R]) PathValueAlias(pathWithWildcards string) *CallMeta[P, R] {
	fullPath := fmt.Sprintf("/%s", pathWithWildcards)
	wildcards := extractWildcards(pathWithWildcards)
	c.m.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		httpPathValueCallHandler(c.m, wildcards, c.callee, w, req)
	})
	c.info.doc.PathValueAliases = append(c.info.doc.PathValueAliases, pathWithWildcards)
	return c
}

type getArgs struct {
	// TODO: let the caller issue options here
}

type topicInfo struct {
	pubMutex sync.Mutex
	doc      topicDoc
}

func rawJsonHandler[P any, R any](m *Misirka, callee Callee[P, R], paramData json.RawMessage) (json.RawMessage, *data.Error) {
	var param P

	err := json.Unmarshal(paramData, &param)
	if err != nil {
		return nil, &data.Error{
			Code: -32700,
			Err:  fmt.Errorf("could not read request body: %w", err),
		}
	}

	result, merr := callee(param)
	if merr != nil {
		return nil, merr
	}

	jdata, err := json.Marshal(result)
	if err != nil {
		return nil, &data.Error{
			Code: -32700,
			Err:  fmt.Errorf("could not encode response: %w", err),
		}
	}

	return jdata, nil
}

func httpCallHandler[P any, R any](m *Misirka, callee Callee[P, R], w http.ResponseWriter, req *http.Request) {
	var param P

	if req.Method == "GET" {
		if len(req.URL.Query()) != 0 {
			paramMap := make(map[string]string)
			for k, vals := range req.URL.Query() {
				if len(vals) != 1 {
					m.writeError(w, &data.Error{
						Code: -32700,
						Err:  fmt.Errorf("parameter %s specified more than once, refusing to process", k),
					})
					return
				}
				paramMap[k] = vals[0]
			}
			err := data.ValsToStruct(paramMap, &param)
			if err != nil {
				m.writeError(w, &data.Error{
					Code: -32700,
					Err:  fmt.Errorf("could not decode stringmap from URL query: %w", err),
				})
				return
			}
		}
		finishHttpCall(m, callee, param, w)
	} else if rawParam, ok := any(param).(*data.RawData); ok {
		rawParam.MimeType = req.Header.Get("Content-Type")
		rawParam.ContentEncoding = req.Header.Get("Content-Encoding")
		rawParam.Data = req.Body
		finishHttpCall(m, callee, param, w)
	} else {
		dec := json.NewDecoder(req.Body)
		err := dec.Decode(&param)
		if err != nil {
			m.writeError(w, &data.Error{
				Code: -32700,
				Err:  fmt.Errorf("could not read request body: %w", err),
			})
			return
		}
		finishHttpCall(m, callee, param, w)
	}
}

func httpPathValueCallHandler[P any, R any](m *Misirka, wildcards []string, callee Callee[P, R], w http.ResponseWriter, req *http.Request) {
	paramMap := make(map[string]string)
	for _, wildcard := range wildcards {
		paramMap[wildcard] = req.PathValue(wildcard)
	}
	var param P
	err := data.ValsToStruct(paramMap, &param)
	if err != nil {
		m.writeError(w, &data.Error{
			Code: -32700,
			Err:  fmt.Errorf("could not decode stringmap from URL query: %w", err),
		})
		return
	}

	finishHttpCall(m, callee, param, w)
}

func finishHttpCall[P any, R any](m *Misirka, callee Callee[P, R], param P, w http.ResponseWriter) {
	result, merr := callee(param)
	if merr != nil {
		m.writeError(w, merr)
		return
	}

	if raw, ok := any(result).(*data.RawData); ok {
		if raw.MimeType != "" {
			w.Header().Set("Content-Type", raw.MimeType)
		}
		if raw.ContentEncoding != "" {
			w.Header().Set("Content-Encoding", raw.ContentEncoding)
		}
		_, err := io.Copy(w, raw.Data)
		if err != nil {
			m.errHandler(fmt.Errorf("could not write raw data response: %w", err))
			return
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		err := enc.Encode(result)
		if err != nil {
			m.errHandler(fmt.Errorf("could not write json response: %w", err))
			return
		}
	}
}

func (m *Misirka) writeError(w http.ResponseWriter, merr *data.Error) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	err := enc.Encode(merr)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write error response: %w", err))
	}
}
