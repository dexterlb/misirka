package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"

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
	Info: func(msg string, data map[string]interface{}) {
		fmt.Fprintf(os.Stderr, "info: %s %v\n", msg, data)
	},
}

type Piper struct {
	buses map[string]*mskbus.BusOf[json.RawMessage]
	srv   *msksrv.Server
	loop  *msksrvbuilder.MainLoop
}

type AddTopicReq struct {
	Path     string
	Descr    string
	Examples []json.RawMessage
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

	p.buses[r.Path] = meta.Bus()
	return nil
}

type PublishReq struct {
	Path string
	Data json.RawMessage
}

func (p *Piper) HandlePublish(r *PublishReq) error {
	bus, ok := p.buses[r.Path]
	if !ok {
		return fmt.Errorf("no such topic %s", r.Path)
	}
	bus.Send(r.Data)
	return nil
}

type SetDocsReq struct {
	Name  string
	Descr string
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

func (p *Piper) HandleRun(param *struct{}) error {
	if p.srv == nil {
		return fmt.Errorf("server not initialised")
	}
	return p.loop.Run()
}

func jsonHandler[A any](params json.RawMessage, dflt *A, handler func(*A) error, respond func(result interface{}, merr *mskdata.Error)) {
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
		buses: make(map[string]*mskbus.BusOf[json.RawMessage]),
	}

	enc := json.NewEncoder(os.Stdout)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		p.handleLine(enc, line)
	}

	if err := scanner.Err(); err != nil {
		evtHandlers.Err(fmt.Errorf("error reading stdin: %w", err))
		os.Exit(1)
	}
}

func (p *Piper) handleLine(enc *json.Encoder, line []byte) {
	var req mskdata.RpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		respond(enc, nil, nil, mskdata.Errorf(-37000, "could not decode message: %w", err))
		return
	}

	p.dispatch(req.Method, req.Params, func(result interface{}, merr *mskdata.Error) {
		respond(enc, req.ID, result, merr)
	})
}

func (p *Piper) dispatch(method string, params json.RawMessage, respond func(result interface{}, merr *mskdata.Error)) {
	switch method {
	case "add_topic":
		jsonHandler(params, nil, p.HandleAddTopic, respond)
	case "publish":
		jsonHandler(params, nil, p.HandlePublish, respond)
	case "init":
		jsonHandler(params, &msksrvbuilder.DefaultServerBuildConfig, p.HandleInit, respond)
	case "set_docs":
		jsonHandler(params, nil, p.HandleSetDocs, respond)
	case "run":
		go jsonHandler(params, nil, p.HandleRun, respond)
	default:
		respond(nil, mskdata.Errorf(-37000, "no such method: %s", method))
	}
}

func respond(enc *json.Encoder, id *uint64, result interface{}, merr *mskdata.Error) {
	var err error
	if merr != nil {
		err = enc.Encode(&mskdata.RpcError{MErr: *merr, ID: id, JsonRPC: "2.0"})
	} else {
		err = enc.Encode(&mskdata.RpcResponse{ID: id, Result: result, JsonRPC: "2.0"})
	}
	if err != nil {
		evtHandlers.Err(fmt.Errorf("could not write response: %w", err))
	}
}
