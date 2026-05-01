package mskdata

// CalleeR is a generic callable that cares about the lifetime of its response
// It should call `respond` only when returning a non-nil error,
// and the response is guaranteed to not be used by Misirka after
// respond exits
type CalleeR[P any, R any] func(param P, respond func(R)) error

// Callee is a generic callable
type Callee[P any, R any] func(param P) (R, error)
