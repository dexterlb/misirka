package misirka

import (
	"fmt"

	"github.com/goccy/go-json"
)

type MErr struct {
	Code int32
	Err  error
}

func (m MErr) MarshalJSON() ([]byte, error) {
	var errStr *string
	if m.Err != nil {
		s := m.Err.Error()
		errStr = &s
	}

	return json.Marshal(struct {
		Code int32   `json:"code"`
		Err  *string `json:"message"`
	}{
		Code: m.Code,
		Err:  errStr,
	})
}

func (m *MErr) Error() string {
	return fmt.Sprintf("[code %d] %s", m.Code, m.Err)
}
