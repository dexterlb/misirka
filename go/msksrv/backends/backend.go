package backends

import (
	"fmt"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
)

type Backend interface {
	AddTopic(path string, bus mskbus.Bus)
	AddCallR(path string, handler CallHandler)
	AddPathValueCallHandler(pathWithWildcards string, handler CallHandler)
}

// CallHandler is a callable that operates with opaque values
type CallHandler func(ParamDecoder, Responder) error

// ParamDecoder populates the given param with data
type ParamDecoder func(any) error

// Responder is given the call's response and does stuff with it
type Responder func(interface{})

func MkCallHandler[P any, R any](callee mskdata.CalleeR[P, R]) CallHandler {
	return func(decoder ParamDecoder, responder Responder) error {
		var param P
		err := decoder(&param)
		if err != nil {
			return fmt.Errorf("could not decode parameter: %w", err)
		}
		gresp := func(resp R) {
			responder(resp)
		}
		return callee(param, gresp)
	}
}
