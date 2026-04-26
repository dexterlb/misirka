package mskdata

// Callee is a generic callable
type Callee[P any, R any] func(param P) (R, error)
