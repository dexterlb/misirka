package backends

import (
	"fmt"
	"maps"
)

type EventHandlers struct {
	Err  func(error)
	Info func(msg string, data map[string]interface{})
}

func (e *EventHandlers) ForBackend(backendName string) EventHandlers {
	return EventHandlers{
		Err: func(err error) {
			e.Err(&BackendSpecificError{
				BackendName: backendName,
				Err:         err,
			})
		},
		Info: func(msg string, data map[string]interface{}) {
			nd := maps.Clone(data)
			nd["backend"] = backendName
			e.Info(msg, nd)
		},
	}
}

type BackendSpecificError struct {
	BackendName string
	Err         error
}

func (b *BackendSpecificError) Error() string {
	return fmt.Sprintf("[backend %s] %w", b.BackendName, b.Err)
}

func (b *BackendSpecificError) Unwrap() error {
	return b.Err
}
