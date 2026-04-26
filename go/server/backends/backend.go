package backends

import (
	"github.com/dexterlb/misirka/go/bus"
	"github.com/dexterlb/misirka/go/data"
	"github.com/goccy/go-json"
)

type Backend interface {
	AddTopic(path string, bus bus.Bus)
	AddCall(path string, handler CallHandler)
}

type CallHandler (func(json.RawMessage) (json.RawMessage, *data.Error))
