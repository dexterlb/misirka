package misirka

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
)

type Misirka struct {
	mux        *http.ServeMux
	errHandler func(error)
	apiDescr   string

	calls  map[string]*callInfo
	topics map[string]*topicInfo

	wsMutex        sync.Mutex
	wsWriteMutexes map[*websocket.Conn]*sync.Mutex
}

func New(errHandler func(error)) *Misirka {
	m := &Misirka{}
	m.mux = http.NewServeMux()
	m.errHandler = errHandler
	m.topics = make(map[string]*topicInfo)
	m.calls = make(map[string]*callInfo)
	m.wsWriteMutexes = make(map[*websocket.Conn]*sync.Mutex)
	return m
}

func AddTopic(m *Misirka, path string) *TopicMeta {
	assertPath(path)
	info := &topicInfo{
		WSSubscribers: make(map[*websocket.Conn]struct{}),
	}
	m.topics[path] = info
	handleCallHttp(m, path, func(args *getArgs) (json.RawMessage, *MErr) {
		return json.RawMessage(info.LastVal), nil
	})
	return &TopicMeta{info: info}
}

func Publish(m *Misirka, path string, item any) {
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

func HandleCall[P any, R any](m *Misirka, path string, callee Callee[P, R]) *CallMeta[P, R] {
	assertPath(path)
	if _, ok := m.calls[path]; ok {
		panic(fmt.Sprintf("HandleCall called twice for path %s", path))
	}

	call := &callInfo{
		rawHandler: func(param json.RawMessage) (json.RawMessage, *MErr) {
			return rawJsonHandler(m, callee, param)
		},
	}

	m.calls[path] = call
	handleCallHttp(m, path, callee)

	return &CallMeta[P, R]{m: m, info: call, callee: callee}
}

type callInfo struct {
	rawHandler (func(json.RawMessage) (json.RawMessage, *MErr))
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
	m.mux.Handle(url, m.WebsocketHandler())
}

type CallMeta[P any, R any] struct {
	info   *callInfo
	m      *Misirka
	callee Callee[P, R]
}

type TopicMeta struct {
	info *topicInfo
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

	handleDoc := func(arg struct{}) (*fullDoc, *MErr) {
		return doc, nil
	}

	htmlgz, err := m.docHTMLgz(doc)
	if err != nil {
		panic(fmt.Sprintf("documentation doesn't render, %s", err))
	}

	handleDocHTMLgz := func(arg struct{}) (*RawData, *MErr) {
		return &RawData{
			Data:            bytes.NewReader(htmlgz),
			MimeType:        "text/html",
			ContentEncoding: "gzip",
		}, nil
	}

	exampleDoc := &fullDoc{APIDescr: "<this documentation>"}

	HandleCall(m, path, handleDoc).
		Descr("get documentation for this API").
		Example(struct{}{}, exampleDoc)

	if htmlPath != "" {
		HandleCall(m, htmlPath, handleDocHTMLgz).
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
	LastVal       []byte
	WSSubscribers map[*websocket.Conn]struct{}
	pubMutex      sync.Mutex
	doc           topicDoc
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
	var param P

	if req.Method == "GET" {
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
			err := valsToStruct(paramMap, &param)
			if err != nil {
				m.writeError(w, &MErr{
					Code: -32700,
					Err:  fmt.Errorf("could not decode stringmap from URL query: %w", err),
				})
				return
			}
		}
		finishHttpCall(m, callee, param, w)
	} else if rawParam, ok := any(param).(*RawData); ok {
		rawParam.MimeType = req.Header.Get("Content-Type")
		rawParam.ContentEncoding = req.Header.Get("Content-Encoding")
		rawParam.Data = req.Body
		finishHttpCall(m, callee, param, w)
	} else {
		dec := json.NewDecoder(req.Body)
		err := dec.Decode(&param)
		if err != nil {
			m.writeError(w, &MErr{
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
	err := valsToStruct(paramMap, &param)
	if err != nil {
		m.writeError(w, &MErr{
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

	if raw, ok := any(result).(*RawData); ok {
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

func (m *Misirka) writeError(w http.ResponseWriter, merr *MErr) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	err := enc.Encode(merr)
	if err != nil {
		m.errHandler(fmt.Errorf("could not write error response: %w", err))
	}
}
