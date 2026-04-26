package backends

import (
	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/goccy/go-json"
)

type Backend interface {
	AddTopic(path string, bus mskbus.Bus)
	AddCall(path string, handler CallHandler)
}

type CallHandler (func(json.RawMessage) (json.RawMessage, error))
