package httpbackend

import (
	"fmt"
	"io"
	"net/http"

	"github.com/dexterlb/misirka/go/mskbus"
	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/goccy/go-json"
)

type HTTPBackend struct {
	mux        *http.ServeMux
	errHandler func(error)
}

func New(errHandler func(error)) *HTTPBackend {
	return &HTTPBackend{
		mux:        http.NewServeMux(),
		errHandler: errHandler,
	}
}

func (h *HTTPBackend) AddTopic(path string, bus mskbus.Bus) {
	getter := func(args *getterArgs) (interface{}, error) {
		return bus.GetT(), nil
	}
	h.AddCall(path, backends.MkCallHandler(getter))

	// TODO: implement long polling when a specific header is set
}

type getterArgs struct {
	// TODO: do we want getters to be able to want specific stuff?
}

func (h *HTTPBackend) AddCall(path string, handler backends.CallHandler) {
	fullPath := fmt.Sprintf("/%s", path)
	h.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		h.httpCallHandler(handler, w, req)
	})
}

func (h *HTTPBackend) AddPathValueCallHandler(pathWithWildcards string, handler backends.CallHandler) {
	fullPath := fmt.Sprintf("/%s", pathWithWildcards)
	wildcards := extractWildcards(pathWithWildcards)
	h.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		h.pathValueCallHandler(wildcards, handler, w, req)
	})
}

func (h *HTTPBackend) AddRawHttpHandler(url string, handler http.Handler) {
	h.mux.Handle(url, handler)
}

func (h *HTTPBackend) Handler() http.Handler {
	return h.mux
}

func (h *HTTPBackend) rawJsonHandler(handler backends.CallHandler, paramData json.RawMessage) (json.RawMessage, error) {
	decoder := func(param any) error {
		err := json.Unmarshal(paramData, &param)
		if err != nil {
			return mskdata.Errorf(
				-32700,
				"could not read request body: %w", err,
			)
		}
		return nil
	}

	result, merr := handler(decoder)
	if merr != nil {
		return nil, merr
	}

	jdata, err := json.Marshal(result)
	if err != nil {
		return nil, mskdata.Errorf(
			-32700,
			"could not encode response: %w", err,
		)
	}

	return jdata, nil
}

func (h *HTTPBackend) httpCallHandler(handler backends.CallHandler, w http.ResponseWriter, req *http.Request) {
	decoder := func(param any) error {
		if req.Method == "GET" {
			if len(req.URL.Query()) != 0 {
				paramMap := make(map[string]string)
				for k, vals := range req.URL.Query() {
					if len(vals) != 1 {
						return mskdata.Errorf(
							-32700,
							"parameter %s specified more than once, refusing to process", k,
						)
					}
					paramMap[k] = vals[0]
				}
				err := mskdata.ValsToStruct(paramMap, &param)
				if err != nil {
					return mskdata.Errorf(
						-32700,
						"could not decode stringmap from URL query: %w", err,
					)
				}
			}
		} else if rawParam, ok := param.(*mskdata.RawData); ok {
			rawParam.MimeType = req.Header.Get("Content-Type")
			rawParam.ContentEncoding = req.Header.Get("Content-Encoding")
			rawParam.Data = req.Body
		} else {
			dec := json.NewDecoder(req.Body)
			err := dec.Decode(&param)
			if err != nil {
				return mskdata.Errorf(
					-32700,
					"could not read request body: %w", err,
				)
			}
		}

		return nil
	}

	h.finishHttpCall(handler, decoder, w)
}

func (h *HTTPBackend) pathValueCallHandler(wildcards []string, handler backends.CallHandler, w http.ResponseWriter, req *http.Request) {
	paramMap := make(map[string]string)
	for _, wildcard := range wildcards {
		paramMap[wildcard] = req.PathValue(wildcard)
	}

	decoder := func(param any) error {
		err := mskdata.ValsToStruct(paramMap, &param)
		if err != nil {
			return mskdata.Errorf(
				-32700,
				"could not decode stringmap from URL query: %w", err,
			)
		}
		return nil
	}

	h.finishHttpCall(handler, decoder, w)
}

func (h *HTTPBackend) finishHttpCall(handler backends.CallHandler, decoder backends.ParamDecoder, w http.ResponseWriter) {
	result, err := handler(decoder)
	if err != nil {
		h.writeError(w, mskdata.GetError(err))
		return
	}

	if raw, ok := any(result).(*mskdata.RawData); ok {
		if raw.MimeType != "" {
			w.Header().Set("Content-Type", raw.MimeType)
		}
		if raw.ContentEncoding != "" {
			w.Header().Set("Content-Encoding", raw.ContentEncoding)
		}
		_, err := io.Copy(w, raw.Data)
		if err != nil {
			h.errHandler(fmt.Errorf("could not write raw data response: %w", err))
			return
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		err := enc.Encode(result)
		if err != nil {
			h.errHandler(fmt.Errorf("could not write json response: %w", err))
			return
		}
	}
}

func (h *HTTPBackend) writeError(w http.ResponseWriter, merr *mskdata.Error) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	err := enc.Encode(merr)
	if err != nil {
		h.errHandler(fmt.Errorf("could not write error response: %w", err))
	}
}
