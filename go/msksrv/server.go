package msksrv

import (
	"fmt"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/dexterlb/misirka/go/msksrv/doc"
)

type Server struct {
	errHandler func(error)
	apiDescr   string

	calls  map[string]*backends.CallInfo
	topics map[string]*backends.TopicInfo

	begun bool

	docWanted bool
	doc       *doc.RenderedDoc

	backends []backends.Backend
}

func New(errHandler func(error)) *Server {
	s := &Server{}
	s.errHandler = errHandler
	s.topics = make(map[string]*backends.TopicInfo)
	s.calls = make(map[string]*backends.CallInfo)
	return s
}

func (s *Server) AddBackend(b backends.Backend) {
	s.assertNotBegun()
	s.backends = append(s.backends, b)
}

func AddTopic[T any](s *Server, path string) *TopicMeta[T] {
	b := mskbus.New[T]()
	return AddTopicWith(s, path, b)
}

func AddTopicWith[T any](s *Server, path string, bus *mskbus.BusOf[T]) *TopicMeta[T] {
	s.assertNotBegun()
	assertPath(path)

	info := &backends.TopicInfo{Bus: bus}
	s.topics[path] = info

	return &TopicMeta[T]{info: info, bus: bus, s: s}
}

// AddCall registers a callable at the given path. The returned result must not
// be modified after it has been returned.
func AddCall[P any, R any](s *Server, path string, callee mskdata.Callee[P, R]) *CallMeta[P, R] {
	cr := func(param P, respond func(R)) error {
		result, err := callee(param)
		if err != nil {
			return err
		}
		respond(result)
		return nil
	}
	return AddCallR(s, path, cr)
}

// AddCallR registers a callable at the given path. The callable is passed a `respond`
// callback that can be used to respond with the result. When `respond()` finishes,
// the result is guaranteed to no longer be used and can be recycled. For cases
// where such a guarantee is not needed, use `AddCall` instead. Do not call `respond()`
// if returning a non-nil error.
func AddCallR[P any, R any](s *Server, path string, callee mskdata.CalleeR[P, R]) *CallMeta[P, R] {
	s.assertNotBegun()
	assertPath(path)
	if _, ok := s.calls[path]; ok {
		panic(fmt.Sprintf("AddCall called twice for path %s", path))
	}

	handler := backends.MkCallHandler(callee)
	info := &backends.CallInfo{Handler: handler}
	s.calls[path] = info

	return &CallMeta[P, R]{s: s, info: info, callee: callee}
}

// Begin must be called after all calls and topics have been set up
func (s *Server) Begin() {
	s.assertNotBegun()
	s.begun = true

	err := s.buildDocIfNeeded()
	if err != nil {
		panic(fmt.Sprintf("could not build documentation: %s", err))
	}

	for _, backend := range s.backends {
		for path, call := range s.calls {
			backend.AddCall(path, call)
		}
		for path, topic := range s.topics {
			backend.AddTopic(path, topic)
		}
	}
}

type CallMeta[P any, R any] struct {
	info   *backends.CallInfo
	s      *Server
	callee mskdata.CalleeR[P, R]
}

type TopicMeta[T any] struct {
	info *backends.TopicInfo
	s    *Server
	bus  *mskbus.BusOf[T]
}

func (t *TopicMeta[T]) Bus() *mskbus.BusOf[T] {
	return t.bus
}

func (c *CallMeta[P, R]) PathValueAlias(pathWithWildcards string) *CallMeta[P, R] {
	c.s.assertNotBegun()
	// TODO: verify that P is a pointer to a struct and its fields match
	// the given wildcards (reflect goes brr)
	c.info.PathValueAliases = append(c.info.PathValueAliases, pathWithWildcards)
	c.info.Doc.PathValueAliases = append(c.info.Doc.PathValueAliases, pathWithWildcards)
	return c
}

func (s *Server) assertNotBegun() {
	if s.begun {
		panic("you cannot do this after having called Begin()")
	}
}
