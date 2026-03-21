package misirka

import (
	"fmt"
)

type MErr struct {
	Code int32 `json: "code"`
	Err  error `json: "message"`
}

func (m *MErr) Error() string {
	return fmt.Sprintf("[code %d] %s", m.Code, m.Err)
}
