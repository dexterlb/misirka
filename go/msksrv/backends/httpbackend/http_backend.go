package httpbackend

import (
	"fmt"
	"io"
	"net/http"

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

func (h *HTTPBackend) AddTopic(path string, tinfo *backends.TopicInfo) {
	getter := func(args *getterArgs, rw func(interface{})) error {
		tinfo.Bus.UseT(rw)
		return nil
	}
	h.addCallHandler(path, backends.MkCallHandler(getter))

	// TODO: implement long polling when a specific header is set
}

type getterArgs struct {
	// TODO: do we want getters to be able to want specific stuff?
}

func (h *HTTPBackend) AddCall(path string, call *backends.CallInfo) {
	h.addCallHandler(path, call.Handler)
	for _, pva := range call.PathValueAliases {
		h.addPathValueCallHandler(pva, call.Handler)
	}
}

func (h *HTTPBackend) addCallHandler(path string, handler backends.CallHandler) {
	fullPath := fmt.Sprintf("/%s", path)
	h.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		h.httpCallHandler(handler, w, req)
	})
}

func (h *HTTPBackend) addPathValueCallHandler(pathWithWildcards string, handler backends.CallHandler) {
	fullPath := fmt.Sprintf("/%s", pathWithWildcards)
	wildcards := extractWildcards(pathWithWildcards)
	h.mux.HandleFunc(fullPath, func(w http.ResponseWriter, req *http.Request) {
		h.pathValueCallHandler(wildcards, handler, w, req)
	})
}

func (h *HTTPBackend) Handler() http.Handler {
	return h.mux
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
				err := mskdata.ValsToStruct(paramMap, param)
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
		// TODO: instead of just looking at path-values, also look at the request body
		// like we do with "regular" requests. Merge values from both sources.
		// Also, in the case when one of the fields of the handler's argument (which must be a struct)
		// is tagged as "catchall", decode the body into that field instead of into
		// the whole struct. This functionality will also be quite useful in the MQTT backend.
		err := mskdata.ValsToStruct(paramMap, param)
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
	respond := func(result interface{}) {
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
				h.writeError(w, mskdata.Errorf(-32700, "could not write json response: %w", err))
				return
			}
		}
	}

	err := handler(decoder, respond)
	if err != nil {
		h.writeError(w, mskdata.GetError(err))
		return
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
