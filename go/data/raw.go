package data

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/goccy/go-json"
)

type RawData struct {
	Data            io.Reader
	MimeType        string
	ContentEncoding string
}

func (r RawData) MarshalJSON() ([]byte, error) {
	data, err := io.ReadAll(r.Data)
	if err != nil {
		return nil, fmt.Errorf("cannot read data: %w", err)
	}
	return json.Marshal(base64.StdEncoding.EncodeToString(data))
}

func (r *RawData) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}

	r.Data = bytes.NewReader(decoded)
	return nil
}
