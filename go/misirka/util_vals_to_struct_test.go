package misirka

import "testing"

type foo struct {
	FieldA string  `json:"field_a"`
	FieldB int64   `json:"field_b",omitempty`
	FieldC float32 `json:"field_c"`
	FieldD bool    `json:"field_d"`
	FieldE uint8   `json:"field_e"`
	FieldQ string
}

type fooPtr struct {
	FieldA *string  `json:"field_a"`
	FieldB *int64   `json:"field_b",omitempty`
	FieldC *float32 `json:"field_c"`
	FieldD *bool    `json:"field_d"`
	FieldE *uint8   `json:"field_e"`
	FieldQ *string
}

var t1 = map[string]string{
	"field_a": "bar",
	"field_b": "42",
	"field_c": "3.14",
	"field_d": "true",
	"field_e": "26",
	"FieldQ":  "baz",
}

var t2 = map[string]string{
	"field_a": "bar: \"qux\"", // should parse correctly, FieldA should be `bar: "qux"`
	"field_b": "-42",
	"field_c": "-3.14",
	"field_d": "false",
	"field_e": "26",
	"FieldQ":  "baz",
}

var t3 = map[string]string{
	"field_a": "bar", // should parse correctly, FieldA should be `bar: "qux"`
	"field_b": "-42",
	"field_c": "nan", // should parse to a nan
	"field_d": "false",
	// missing fields should not cause an error and should be left alone
}

var t4 = map[string]string{
	"field_a": "bar",
	"field_b": "42",
	"field_c": "3.14",
	"field_d": "true",
	"field_e": "26",
	"FieldQ":  "baz",
	"field_p": "qux", // should cause error (unknown field field)
}

var t5 = map[string]string{
	"field_a": "bar",
	"field_b": "42",
	"field_c": "3.14",
	"field_d": "1", // 1 is true, 0 is false
	"field_e": "26",
	"FieldQ":  "baz",
}

func TestValsToStruct(t *testing.T) {
	t.Run("t1 basic case", func(t *testing.T) {
		var f foo
		err := valsToStruct(t1, &f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.FieldA != "bar" {
			t.Errorf("FieldA = %q, want %q", f.FieldA, "bar")
		}
		if f.FieldB != 42 {
			t.Errorf("FieldB = %d, want 42", f.FieldB)
		}
		if f.FieldC != 3.14 {
			t.Errorf("FieldC = %f, want 3.14", f.FieldC)
		}
		if f.FieldD != true {
			t.Errorf("FieldD = %v, want true", f.FieldD)
		}
		if f.FieldE != 26 {
			t.Errorf("FieldE = %d, want 26", f.FieldE)
		}
		if f.FieldQ != "baz" {
			t.Errorf("FieldQ = %q, want %q", f.FieldQ, "baz")
		}
	})

	t.Run("t2 with quotes and negatives", func(t *testing.T) {
		var f foo
		err := valsToStruct(t2, &f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.FieldA != `bar: "qux"` {
			t.Errorf("FieldA = %q, want %q", f.FieldA, `bar: "qux"`)
		}
		if f.FieldB != -42 {
			t.Errorf("FieldB = %d, want -42", f.FieldB)
		}
		if f.FieldC != -3.14 {
			t.Errorf("FieldC = %f, want -3.14", f.FieldC)
		}
		if f.FieldD != false {
			t.Errorf("FieldD = %v, want false", f.FieldD)
		}
		if f.FieldE != 26 {
			t.Errorf("FieldE = %d, want 26", f.FieldE)
		}
		if f.FieldQ != "baz" {
			t.Errorf("FieldQ = %q, want %q", f.FieldQ, "baz")
		}
	})

	t.Run("t3 missing fields and nan", func(t *testing.T) {
		var f foo
		err := valsToStruct(t3, &f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.FieldA != "bar" {
			t.Errorf("FieldA = %q, want %q", f.FieldA, "bar")
		}
		if f.FieldB != -42 {
			t.Errorf("FieldB = %d, want -42", f.FieldB)
		}
		if !isNaN(f.FieldC) {
			t.Errorf("FieldC = %f, want NaN", f.FieldC)
		}
		if f.FieldD != false {
			t.Errorf("FieldD = %v, want false", f.FieldD)
		}
	})

	t.Run("t4 unknown field error", func(t *testing.T) {
		var f foo
		err := valsToStruct(t4, &f)
		if err == nil {
			t.Fatal("expected error for unknown field, got nil")
		}
	})

	t.Run("t5 bool as 1/0", func(t *testing.T) {
		var f foo
		err := valsToStruct(t5, &f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.FieldD != true {
			t.Errorf("FieldD = %v, want true", f.FieldD)
		}
	})
}

func isNaN(f float32) bool {
	return f != f
}

func TestValsToStructPointers(t *testing.T) {
	t.Run("t1p all fields provided", func(t *testing.T) {
		var f fooPtr
		err := valsToStruct(t1, &f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.FieldA == nil || *f.FieldA != "bar" {
			t.Errorf("FieldA = %v, want %q", f.FieldA, "bar")
		}
		if f.FieldB == nil || *f.FieldB != 42 {
			t.Errorf("FieldB = %v, want 42", f.FieldB)
		}
		if f.FieldC == nil || *f.FieldC != 3.14 {
			t.Errorf("FieldC = %v, want 3.14", f.FieldC)
		}
		if f.FieldD == nil || *f.FieldD != true {
			t.Errorf("FieldD = %v, want true", f.FieldD)
		}
		if f.FieldE == nil || *f.FieldE != 26 {
			t.Errorf("FieldE = %v, want 26", f.FieldE)
		}
		if f.FieldQ == nil || *f.FieldQ != "baz" {
			t.Errorf("FieldQ = %v, want %q", f.FieldQ, "baz")
		}
	})

	t.Run("t3p missing fields are nil", func(t *testing.T) {
		var f fooPtr
		err := valsToStruct(t3, &f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.FieldA == nil || *f.FieldA != "bar" {
			t.Errorf("FieldA = %v, want %q", f.FieldA, "bar")
		}
		if f.FieldB == nil || *f.FieldB != -42 {
			t.Errorf("FieldB = %v, want -42", f.FieldB)
		}
		if f.FieldC == nil || !isNaN(*f.FieldC) {
			t.Errorf("FieldC = %v, want NaN", f.FieldC)
		}
		if f.FieldD == nil || *f.FieldD != false {
			t.Errorf("FieldD = %v, want false", f.FieldD)
		}
		if f.FieldE != nil {
			t.Errorf("FieldE = %v, want nil", f.FieldE)
		}
		if f.FieldQ != nil {
			t.Errorf("FieldQ = %v, want nil", f.FieldQ)
		}
	})
}
