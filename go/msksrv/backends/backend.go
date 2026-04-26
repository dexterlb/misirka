package backends

import (
	"fmt"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
)

type Backend interface {
	AddTopic(path string, bus mskbus.Bus)
	AddCall(path string, handler CallHandler)
}

// CallHandler is a callable that operates with opaque values
type CallHandler (func(ParamDecoder) (interface{}, error))

// ParamDecoder populates the given param with data
type ParamDecoder func(any) error

func MkCallHandler[P any, R any](callee mskdata.Callee[P, R]) CallHandler {
	return func(decoder ParamDecoder) (interface{}, error) {
		var param P
		err := decoder(&param)
		if err != nil {
			return nil, fmt.Errorf("could not decode parameter: %w", err)
		}
		result, err := callee(param)
		return result, err
	}
}
