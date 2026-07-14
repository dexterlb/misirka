package mskdata

import (
	"github.com/goccy/go-json"
)

type RpcResponse struct {
	ID      *uint64     `json:"id"`
	Result  interface{} `json:"result"`
	JsonRPC string      `json:"jsonrpc"`
}

type RpcError struct {
	MErr    Error   `json:"error"`
	ID      *uint64 `json:"id"`
	JsonRPC string  `json:"jsonrpc"`
}

type RpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     *uint64         `json:"id"`
}
