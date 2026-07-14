package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sync"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/dexterlb/misirka/go/msksrvbuilder"
	"github.com/goccy/go-json"
)

var evtHandlers = backends.EventHandlers{
	Err: func(err error) {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
	},
	Info: func(msg string, data map[string]any) {
		fmt.Fprintf(os.Stderr, "info: %s %v\n", msg, data)
	},
}

type Piper struct {
	buses map[string]*mskbus.BusOf[json.RawMessage]
	srv   *msksrv.Server
	loop  *msksrvbuilder.MainLoop

	enc     *json.Encoder
	encLock sync.Mutex

	callLock   sync.Mutex
	callReqs   map[uint64]chan callResult
	lastCallID uint64
}

type callResult struct {
	result json.RawMessage
	err    *mskdata.Error
}

type AddTopicReq struct {
	Path     string            `json:"path"`
	Descr    string            `json:"descr"`
	Examples []json.RawMessage `json:"examples"`
	Dedup    bool              `json:"dedup"`
}

func (p *Piper) HandleAddTopic(r *AddTopicReq) error {
	if p.srv == nil {
		return fmt.Errorf("server not initialised")
	}

	meta := msksrv.AddTopic[json.RawMessage](p.srv, r.Path)
	meta.Descr(r.Descr)
	for _, example := range r.Examples {
		meta.Example(example)
	}

	bus := meta.Bus()
	if r.Dedup {
		bus.DedupBy(func(prev, x json.RawMessage) bool {
			return bytes.Equal(prev, x)
		})
	}

	p.buses[r.Path] = bus
	return nil
}

type PublishReq struct {
	Path string          `json:"path"`
	Data json.RawMessage `json:"data"`
}

func (p *Piper) HandlePublish(r *PublishReq) error {
	bus, ok := p.buses[r.Path]
	if !ok {
		return fmt.Errorf("no such topic %s", r.Path)
	}
	bus.Send(r.Data)
	return nil
}

type AddCallReq struct {
	Path     string               `json:"path"`
	Descr    string               `json:"descr"`
	IsAsync  bool                 `json:"is_async"`
	Examples [][2]json.RawMessage `json:"examples"`
}

func (p *Piper) HandleAddCall(r *AddCallReq) error {
	if p.srv == nil {
		return fmt.Errorf("server not initialised")
	}

	path := r.Path
	meta := msksrv.AddCall[json.RawMessage, json.RawMessage](p.srv, path,
		func(param json.RawMessage) (json.RawMessage, error) {
			return p.handleCall(path, param)
		},
	)
	meta.Descr(r.Descr).Async(r.IsAsync)
	for _, example := range r.Examples {
		meta.Example(example[0], example[1])
	}

	return nil
}

func (p *Piper) handleCall(path string, param json.RawMessage) (json.RawMessage, error) {
	p.callLock.Lock()
	p.lastCallID++
	id := p.lastCallID
	ch := make(chan callResult, 1)
	p.callReqs[id] = ch
	p.callLock.Unlock()

	defer func() {
		p.callLock.Lock()
		delete(p.callReqs, id)
		p.callLock.Unlock()
	}()

	p.writeOut(&mskdata.RpcRequest{Method: path, Params: param, ID: &id})

	res := <-ch
	if res.err != nil {
		return nil, res.err
	}
	return res.result, nil
}

type SetDocsReq struct {
	Name  string `json:"name"`
	Descr string `json:"descr"`
}

func (p *Piper) HandleSetDocs(r *SetDocsReq) error {
	if p.srv == nil {
		return fmt.Errorf("server not initialised")
	}
	p.srv.Name(r.Name).Descr(r.Descr)
	return nil
}

func (p *Piper) HandleInit(cfg *msksrvbuilder.ServerBuildConfig) error {
	if p.srv != nil {
		return fmt.Errorf("server already initialised")
	}
	p.srv, p.loop = msksrvbuilder.BuildServer(evtHandlers, cfg)
	return nil
}

func (p *Piper) HandleServe(param *struct{}) error {
	if p.srv == nil {
		return fmt.Errorf("server not initialised")
	}
	return p.loop.Run()
}

func jsonHandler[A any](params json.RawMessage, dflt *A, handler func(*A) error, respond func(result any, merr *mskdata.Error)) {
	var x A
	if dflt != nil {
		x = *dflt
	}
	if len(params) > 0 {
		err := json.Unmarshal(params, &x)
		if err != nil {
			respond(nil, mskdata.Errorf(-37000, "could not parse params: %w", err))
			return
		}
	}
	if err := handler(&x); err != nil {
		respond(nil, mskdata.GetError(err))
		return
	}
	respond("ok", nil)
}

func main() {
	p := &Piper{
		buses:    make(map[string]*mskbus.BusOf[json.RawMessage]),
		callReqs: make(map[uint64]chan callResult),
		enc:      json.NewEncoder(os.Stdout),
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		p.handleLine(line)
	}

	if err := scanner.Err(); err != nil {
		evtHandlers.Err(fmt.Errorf("error reading stdin: %w", err))
		os.Exit(1)
	}
}

// messages received on the pipe can be either a jsonrpc request or
// a jsonrpc response. This struct has the fields from both.
type pipeMsg struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     *uint64         `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *mskdata.Error  `json:"error"`
}

func (p *Piper) handleLine(line []byte) {
	var msg pipeMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		p.respond(nil, nil, mskdata.Errorf(-37000, "could not decode message: %w", err))
		return
	}

	if msg.Method != "" {
		p.dispatch(msg.Method, msg.Params, func(result any, merr *mskdata.Error) {
			p.respond(msg.ID, result, merr)
		})
		return
	}

	if msg.ID != nil {
		p.resolveCall(&msg)
		return
	}

	evtHandlers.Err(fmt.Errorf("received message with neither method nor id"))
}

func (p *Piper) resolveCall(msg *pipeMsg) {
	p.callLock.Lock()
	ch, ok := p.callReqs[*msg.ID]
	p.callLock.Unlock()
	if !ok {
		evtHandlers.Err(fmt.Errorf("received response for unknown call id %d", *msg.ID))
		return
	}

	if msg.Error != nil {
		ch <- callResult{err: msg.Error}
	} else {
		ch <- callResult{result: msg.Result}
	}
}

func (p *Piper) dispatch(method string, params json.RawMessage, respond func(result any, merr *mskdata.Error)) {
	switch method {
	case "add_topic":
		jsonHandler(params, nil, p.HandleAddTopic, respond)
	case "publish":
		jsonHandler(params, nil, p.HandlePublish, respond)
	case "init":
		jsonHandler(params, &msksrvbuilder.DefaultServerBuildConfig, p.HandleInit, respond)
	case "set_docs":
		jsonHandler(params, nil, p.HandleSetDocs, respond)
	case "add_call":
		jsonHandler(params, nil, p.HandleAddCall, respond)
	case "serve":
		go jsonHandler(params, nil, p.HandleServe, respond)
	default:
		respond(nil, mskdata.Errorf(-37000, "no such method: %s", method))
	}
}

func (p *Piper) respond(id *uint64, result any, merr *mskdata.Error) {
	if merr != nil {
		p.writeOut(&mskdata.RpcError{MErr: *merr, ID: id, JsonRPC: "2.0"})
	} else {
		p.writeOut(&mskdata.RpcResponse{ID: id, Result: result, JsonRPC: "2.0"})
	}
}

func (p *Piper) writeOut(v any) {
	p.encLock.Lock()
	defer p.encLock.Unlock()
	if err := p.enc.Encode(v); err != nil {
		evtHandlers.Err(fmt.Errorf("could not write to stdout: %w", err))
	}
}
