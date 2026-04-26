package msksrv

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/dexterlb/misirka/go/msksrv/backends/wsbackend"
	"github.com/goccy/go-json"
)

type Server struct {
	mux        *http.ServeMux
	errHandler func(error)
	apiDescr   string

	calls  map[string]*callInfo
	topics map[string]*topicInfo

	backends []backends.Backend

	// TODO: instead of doing this, make a "misirkamaker" function that
	// initialises all backends and then passes them to a new Server
	wsBackend *wsbackend.WSBackend
}

func New(errHandler func(error)) *Server {
	s := &Server{}
	s.mux = http.NewServeMux()
	s.errHandler = errHandler
	s.topics = make(map[string]*topicInfo)
	s.calls = make(map[string]*callInfo)
	s.wsBackend = wsbackend.New(errHandler)
	s.backends = []backends.Backend{s.wsBackend}
	return s
}

func AddTopic[T any](s *Server, path string) *TopicMeta[T] {
	b := mskbus.New[T]()
	return AddTopicWith(s, path, b)
}

func AddTopicWith[T any](s *Server, path string, bus *mskbus.BusOf[T]) *TopicMeta[T] {
	assertPath(path)

	for _, backend := range s.backends {
		backend.AddTopic(path, bus)
	}

	// TODO: move into http backend
	info := &topicInfo{}
	s.topics[path] = info
	handleCallHttp(s, path, func(args *getArgs) (interface{}, *mskdata.Error) {
		return bus.GetT(), nil
	})

	return &TopicMeta[T]{info: info, bus: bus}
}

type Callee[P any, R any] func(param P) (R, *mskdata.Error)

func AddCall[P any, R any](s *Server, path string, callee Callee[P, R]) *CallMeta[P, R] {
	assertPath(path)
	if _, ok := s.calls[path]; ok {
		panic(fmt.Sprintf("AddCall called twice for path %s", path))
	}

	handler := func(param json.RawMessage) (json.RawMessage, *mskdata.Error) {
		return rawJsonHandler(s, callee, param)
	}

	for _, backend := range s.backends {
		backend.AddCall(path, handler)
	}

	call := &callInfo{
		rawHandler: handler,
	}

	s.calls[path] = call
	handleCallHttp(s, path, callee)

	return &CallMeta[P, R]{s: s, info: call, callee: callee}
}

type callInfo struct {
	rawHandler (func(json.RawMessage) (json.RawMessage, *mskdata.Error))
	doc        callDoc
}

func (s *Server) HTTPHandler() http.Handler {
	return s.mux
}

// HandleWebsocket registers a websocket handler under `/ws`. To use another
// URL for the websocket, use `HandleWebsocketAt()`. To handle the Server
// websocket manually, use `WebsocketHandler()`.
func (s *Server) HandleWebsocket() {
	s.HandleWebsocketAt("/ws")
}

// HandleWebsocket registers a websocket handler under the given url.
// The URL should begin with a leading slash and is handled at http level.
// To handle the Server websocket manually, use `WebsocketHandler()`.
func (s *Server) HandleWebsocketAt(url string) {
	s.mux.Handle(url, s.wsBackend.WSHTTPHandler())
}

type CallMeta[P any, R any] struct {
	info   *callInfo
	s      *Server
	callee Callee[P, R]
}

type TopicMeta[T any] struct {
	info *topicInfo
	bus  *mskbus.BusOf[T]
}

func (t *TopicMeta[T]) Bus() *mskbus.BusOf[T] {
	return t.bus
}

func (s *Server) HandleDoc() {
	s.HandleDocAt("doc", "doc.html")
}

func (s *Server) HandleDocAt(path string, htmlPath string) {
	doc := &fullDoc{
		APIDescr: s.apiDescr,
		Topics:   make(map[string]*topicDoc),
		Calls:    make(map[string]*callDoc),
	}
	for tp := range s.topics {
		doc.Topics[tp] = &s.topics[tp].doc
	}
	for cp := range s.calls {
		doc.Calls[cp] = &s.calls[cp].doc
	}

	doc.Validate()

	handleDoc := func(arg struct{}) (*fullDoc, *mskdata.Error) {
		return doc, nil
	}

	htmlgz, err := s.docHTMLgz(doc)
	if err != nil {
		panic(fmt.Sprintf("documentation doesn't render, %s", err))
	}

	handleDocHTMLgz := func(arg struct{}) (*mskdata.RawData, *mskdata.Error) {
		return &mskdata.RawData{
			Data:            bytes.NewReader(htmlgz),
			MimeType:        "text/html",
			ContentEncoding: "gzip",
		}, nil
	}

	exampleDoc := &fullDoc{APIDescr: "<this documentation>"}

	AddCall(s, path, handleDoc).
		Descr("get documentation for this API").
		Example(struct{}{}, exampleDoc)

	if htmlPath != "" {
		AddCall(s, htmlPath, handleDocHTMLgz).
			Descr("get documentation for this API in human-readeble HTML")
	}
}

func handleCallHttp[P any, R any](s *Server, path string, callee Callee[P, R]) {
	fullPath := fmt.Sprintf("/%s", path)
	s.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		httpCallHandler(s, callee, w, req)
	})
}

func (c *CallMeta[P, R]) PathValueAlias(pathWithWildcards string) *CallMeta[P, R] {
	fullPath := fmt.Sprintf("/%s", pathWithWildcards)
	wildcards := extractWildcards(pathWithWildcards)
	c.s.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		httpPathValueCallHandler(c.s, wildcards, c.callee, w, req)
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

func rawJsonHandler[P any, R any](s *Server, callee Callee[P, R], paramData json.RawMessage) (json.RawMessage, *mskdata.Error) {
	var param P

	err := json.Unmarshal(paramData, &param)
	if err != nil {
		return nil, &mskdata.Error{
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
		return nil, &mskdata.Error{
			Code: -32700,
			Err:  fmt.Errorf("could not encode response: %w", err),
		}
	}

	return jdata, nil
}

func httpCallHandler[P any, R any](s *Server, callee Callee[P, R], w http.ResponseWriter, req *http.Request) {
	var param P

	if req.Method == "GET" {
		if len(req.URL.Query()) != 0 {
			paramMap := make(map[string]string)
			for k, vals := range req.URL.Query() {
				if len(vals) != 1 {
					s.writeError(w, &mskdata.Error{
						Code: -32700,
						Err:  fmt.Errorf("parameter %s specified more than once, refusing to process", k),
					})
					return
				}
				paramMap[k] = vals[0]
			}
			err := mskdata.ValsToStruct(paramMap, &param)
			if err != nil {
				s.writeError(w, &mskdata.Error{
					Code: -32700,
					Err:  fmt.Errorf("could not decode stringmap from URL query: %w", err),
				})
				return
			}
		}
		finishHttpCall(s, callee, param, w)
	} else if rawParam, ok := any(param).(*mskdata.RawData); ok {
		rawParam.MimeType = req.Header.Get("Content-Type")
		rawParam.ContentEncoding = req.Header.Get("Content-Encoding")
		rawParam.Data = req.Body
		finishHttpCall(s, callee, param, w)
	} else {
		dec := json.NewDecoder(req.Body)
		err := dec.Decode(&param)
		if err != nil {
			s.writeError(w, &mskdata.Error{
				Code: -32700,
				Err:  fmt.Errorf("could not read request body: %w", err),
			})
			return
		}
		finishHttpCall(s, callee, param, w)
	}
}

func httpPathValueCallHandler[P any, R any](s *Server, wildcards []string, callee Callee[P, R], w http.ResponseWriter, req *http.Request) {
	paramMap := make(map[string]string)
	for _, wildcard := range wildcards {
		paramMap[wildcard] = req.PathValue(wildcard)
	}
	var param P
	err := mskdata.ValsToStruct(paramMap, &param)
	if err != nil {
		s.writeError(w, &mskdata.Error{
			Code: -32700,
			Err:  fmt.Errorf("could not decode stringmap from URL query: %w", err),
		})
		return
	}

	finishHttpCall(s, callee, param, w)
}

func finishHttpCall[P any, R any](s *Server, callee Callee[P, R], param P, w http.ResponseWriter) {
	result, merr := callee(param)
	if merr != nil {
		s.writeError(w, merr)
		return
	}

	if raw, ok := any(result).(*mskdata.RawData); ok {
		if raw.MimeType != "" {
			w.Header().Set("Content-Type", raw.MimeType)
		}
		if raw.ContentEncoding != "" {
			w.Header().Set("Content-Encoding", raw.ContentEncoding)
		}
		_, err := io.Copy(w, raw.Data)
		if err != nil {
			s.errHandler(fmt.Errorf("could not write raw data response: %w", err))
			return
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		err := enc.Encode(result)
		if err != nil {
			s.errHandler(fmt.Errorf("could not write json response: %w", err))
			return
		}
	}
}

func (s *Server) writeError(w http.ResponseWriter, merr *mskdata.Error) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	err := enc.Encode(merr)
	if err != nil {
		s.errHandler(fmt.Errorf("could not write error response: %w", err))
	}
}
