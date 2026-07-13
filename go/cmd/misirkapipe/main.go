package main

import (
	"fmt"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/msksrv"
	"github.com/dexterlb/misirka/go/msksrvbuilder"
	"github.com/goccy/go-json"
)

type Piper struct {
	buses map[string]mskbus.BusOf[json.RawMessage]
	srv   *msksrv.Server
}

type AddTopicReq struct {
	Path     string
	Descr    string
	Examples []json.RawMessage
}

func (p *Piper) HandleAddTopic(r *AddTopicReq) error {
	meta := p.srv.AddTopic[json.RawMessage](r.Path)
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
	bus.Publish(r.Data)
	return nil
}

func (p *Piper) HandleStart(cfg *msksrvbuilder.ServerBuildConfig) error {
	p.srv, p.loop = msksrvbuilder.BuildServer(evtHandlers, cfg)
	return nil
}

func main() {

}
