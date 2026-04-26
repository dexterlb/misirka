package mskdata

// WARNING: this file was written by an LLM and is probably buggy.
// At some point it has to be rewritten.
import (
	"reflect"
	"strconv"
	"sync"
)

type fieldInfo struct {
	index    int
	name     string
	kind     reflect.Kind
	elemKind reflect.Kind
	isPtr    bool
	canSet   bool
}

type typeInfo struct {
	fields      []fieldInfo
	knownFields map[string]bool
}

var typeCache sync.Map

func ValsToStruct(vm map[string]string, p any) error {
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Ptr {
		return &invalidTypeError{kind: rv.Kind().String()}
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return &invalidTypeError{kind: rv.Kind().String()}
	}
	t := rv.Type()
	info, ok := typeCache.Load(t)
	if !ok {
		info = cacheType(t)
		typeCache.Store(t, info)
	}
	ti := info.(*typeInfo)

	for _, f := range ti.fields {
		val, ok := vm[f.name]
		if !ok {
			continue
		}

		fieldVal := rv.Field(f.index)
		if !f.canSet {
			continue
		}

		if val == "" {
			continue
		}

		kind := f.kind
		if f.isPtr {
			kind = f.elemKind
		}

		switch kind {
		case reflect.String:
			if f.isPtr {
				fieldVal.Set(reflect.ValueOf(&val))
			} else {
				fieldVal.SetString(val)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return err
			}
			if f.isPtr {
				switch f.elemKind {
				case reflect.Int8:
					v := int8(n)
					fieldVal.Set(reflect.ValueOf(&v))
				case reflect.Int16:
					v := int16(n)
					fieldVal.Set(reflect.ValueOf(&v))
				case reflect.Int32:
					v := int32(n)
					fieldVal.Set(reflect.ValueOf(&v))
				case reflect.Int64:
					fieldVal.Set(reflect.ValueOf(&n))
				default:
					v := int(n)
					fieldVal.Set(reflect.ValueOf(&v))
				}
			} else {
				fieldVal.SetInt(n)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return err
			}
			if f.isPtr {
				switch f.elemKind {
				case reflect.Uint8:
					v := uint8(n)
					fieldVal.Set(reflect.ValueOf(&v))
				case reflect.Uint16:
					v := uint16(n)
					fieldVal.Set(reflect.ValueOf(&v))
				case reflect.Uint32:
					v := uint32(n)
					fieldVal.Set(reflect.ValueOf(&v))
				case reflect.Uint64:
					fieldVal.Set(reflect.ValueOf(&n))
				default:
					v := uint(n)
					fieldVal.Set(reflect.ValueOf(&v))
				}
			} else {
				fieldVal.SetUint(n)
			}
		case reflect.Float32, reflect.Float64:
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return err
			}
			if f.isPtr {
				if f.elemKind == reflect.Float32 {
					v := float32(n)
					fieldVal.Set(reflect.ValueOf(&v))
				} else {
					fieldVal.Set(reflect.ValueOf(&n))
				}
			} else {
				fieldVal.SetFloat(n)
			}
		case reflect.Bool:
			var b bool
			if val == "true" || val == "1" {
				b = true
			} else if val == "false" || val == "0" {
				b = false
			} else {
				parsed, err := strconv.ParseBool(val)
				if err != nil {
					return err
				}
				b = parsed
			}
			if f.isPtr {
				fieldVal.Set(reflect.ValueOf(&b))
			} else {
				fieldVal.SetBool(b)
			}
		}
	}

	for k := range vm {
		if !ti.knownFields[k] {
			return &unknownFieldError{field: k}
		}
	}

	return nil
}

func cacheType(rt reflect.Type) *typeInfo {
	ti := &typeInfo{
		knownFields: make(map[string]bool),
	}
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" {
			tag = field.Tag.Get("codec")
			if tag == "" {
				tag = field.Name
			}
		}
		name := splitTag(tag)
		if name == "-" || name == "" {
			continue
		}
		ti.knownFields[name] = true
		isPtr := field.Type.Kind() == reflect.Ptr
		var elemKind reflect.Kind
		if isPtr {
			elemKind = field.Type.Elem().Kind()
		}
		ti.fields = append(ti.fields, fieldInfo{
			index:    i,
			name:     name,
			kind:     field.Type.Kind(),
			elemKind: elemKind,
			isPtr:    isPtr,
			canSet:   field.PkgPath == "",
		})
	}
	return ti
}

func splitTag(tag string) (name string) {
	for i, c := range tag {
		if c == ',' {
			name = tag[:i]
			return
		}
	}
	return tag
}

type unknownFieldError struct {
	field string
}

func (e *unknownFieldError) Error() string {
	return "unknown field: " + e.field
}

type invalidTypeError struct {
	kind string
}

func (e *invalidTypeError) Error() string {
	return "invalid type: " + e.kind
}
