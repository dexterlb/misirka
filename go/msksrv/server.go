package msksrv

import (
	"fmt"
	"sync"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/backends"
)

type Server struct {
	errHandler func(error)
	apiDescr   string

	calls  map[string]*callInfo
	topics map[string]*topicInfo

	backends []backends.Backend
}

func New(errHandler func(error)) *Server {
	s := &Server{}
	s.errHandler = errHandler
	s.topics = make(map[string]*topicInfo)
	s.calls = make(map[string]*callInfo)
	return s
}

func (s *Server) AddBackend(b backends.Backend) {
	s.backends = append(s.backends, b)
}

func AddTopic[T any](s *Server, path string) *TopicMeta[T] {
	b := mskbus.New[T]()
	return AddTopicWith(s, path, b)
}

func AddTopicWith[T any](s *Server, path string, bus *mskbus.BusOf[T]) *TopicMeta[T] {
	assertPath(path)

	info := &topicInfo{}
	s.topics[path] = info

	for _, backend := range s.backends {
		backend.AddTopic(path, bus)
	}

	return &TopicMeta[T]{info: info, bus: bus}
}

func AddCall[P any, R any](s *Server, path string, callee mskdata.Callee[P, R]) *CallMeta[P, R] {
	assertPath(path)
	if _, ok := s.calls[path]; ok {
		panic(fmt.Sprintf("AddCall called twice for path %s", path))
	}

	handler := backends.MkCallHandler(callee)

	for _, backend := range s.backends {
		backend.AddCall(path, handler)
	}

	callInfo := &callInfo{handler: handler}
	s.calls[path] = callInfo

	return &CallMeta[P, R]{s: s, info: callInfo, callee: callee}
}

type callInfo struct {
	doc     callDoc
	handler backends.CallHandler
}

type topicInfo struct {
	pubMutex sync.Mutex
	doc      topicDoc
}

type CallMeta[P any, R any] struct {
	info   *callInfo
	s      *Server
	callee mskdata.Callee[P, R]
}

type TopicMeta[T any] struct {
	info *topicInfo
	bus  *mskbus.BusOf[T]
}

func (t *TopicMeta[T]) Bus() *mskbus.BusOf[T] {
	return t.bus
}

func (c *CallMeta[P, R]) PathValueAlias(pathWithWildcards string) *CallMeta[P, R] {
	// TODO: verify that P is a pointer to a struct and its fields match
	// the given wildcards (reflect goes brr)
	for _, backend := range c.s.backends {
		backend.AddPathValueCallHandler(pathWithWildcards, c.info.handler)
	}
	c.info.doc.PathValueAliases = append(c.info.doc.PathValueAliases, pathWithWildcards)
	return c
}
