package msksrv

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/goccy/go-json"
)

type fullDoc struct {
	APIDescr string               `json:"description"`
	Calls    map[string]*callDoc  `json:"calls"`
	Topics   map[string]*topicDoc `json:"topics"`
}

type callDoc struct {
	Description      string        `json:"description"`
	ExampleCalls     []exampleCall `json:"example_calls"`
	PathValueAliases []string      `json:"path_value_aliases,omitempty"`
}

type topicDoc struct {
	Description string        `json:"description"`
	Examples    []interface{} `json:"examples"`
}

type exampleCall struct {
	Param  interface{} `json:"param"`
	Result interface{} `json:"result"`
}

func (f *fullDoc) Validate() {
	if f.APIDescr == "" {
		panic("Don't be lazy! Set a description on the Server object with .Descr(<text>)!")
	}

	for ct, c := range f.Calls {
		if c.Description == "" {
			panic(fmt.Sprintf("Don't be lazy! Set a description on %s with .Descr(<text>)!", ct))
		}
		if len(c.ExampleCalls) == 0 {
			panic(fmt.Sprintf("Don't be lazy! Set some examples on %s with .Example(<param>, <result>)", ct))
		}
	}

	for tt, t := range f.Topics {
		if t.Description == "" {
			panic(fmt.Sprintf("Don't be lazy! Set a description on %s with .Descr(<text>)!", tt))
		}
		if len(t.Examples) == 0 {
			panic(fmt.Sprintf("Don't be lazy! Set some examples on %s with .Example(<value>)!", tt))
		}
	}
}

func (s *Server) HandleDoc() {
	s.HandleDocAt("doc", "doc.html")
}

func (s *Server) HandleDocAt(path string, htmlPath string) {
	doc := &fullDoc{
		APIDescr: s.apiDescr,
		Topics:   make(map[string]*topicDoc),
		Calls:    make(map[string]*callDoc),
	}
	for tp := range s.topics {
		doc.Topics[tp] = &s.topics[tp].doc
	}
	for cp := range s.calls {
		doc.Calls[cp] = &s.calls[cp].doc
	}

	doc.Validate()

	handleDoc := func(arg struct{}) (*fullDoc, error) {
		return doc, nil
	}

	htmlgz, err := s.docHTMLgz(doc)
	if err != nil {
		panic(fmt.Sprintf("documentation doesn't render, %s", err))
	}

	handleDocHTMLgz := func(arg struct{}) (*mskdata.RawData, error) {
		return &mskdata.RawData{
			Data:            bytes.NewReader(htmlgz),
			MimeType:        "text/html",
			ContentEncoding: "gzip",
		}, nil
	}

	exampleDoc := &fullDoc{APIDescr: "<this documentation>"}

	AddCall(s, path, handleDoc).
		Descr("get documentation for this API").
		Example(struct{}{}, exampleDoc)

	if htmlPath != "" {
		AddCall(s, htmlPath, handleDocHTMLgz).
			Descr("get documentation for this API in human-readeble HTML")
	}
}

func (c *CallMeta[P, R]) Descr(descr string) *CallMeta[P, R] {
	c.info.doc.Description = descr
	return c
}

func (c *CallMeta[P, R]) Example(param P, result R) *CallMeta[P, R] {
	c.info.doc.ExampleCalls = append(c.info.doc.ExampleCalls, exampleCall{
		Param:  param,
		Result: result,
	})
	return c
}

func (t *TopicMeta[T]) Descr(descr string) *TopicMeta[T] {
	t.info.doc.Description = descr
	return t
}

func (t *TopicMeta[T]) Example(val any) *TopicMeta[T] {
	t.info.doc.Examples = append(t.info.doc.Examples, val)
	return t
}

func (s *Server) Descr(descr string) *Server {
	s.apiDescr = descr
	return s
}

//go:embed doc.html
var docHTMLTemplate []byte

func (s *Server) docHTMLgz(doc *fullDoc) ([]byte, error) {
	funcs := template.FuncMap{
		"jsonify": func(x interface{}) string {
			data, err := json.Marshal(x)
			if err != nil {
				panic(fmt.Sprintf("cannot jsonify value inside documentation! %s", err))
			}
			return string(data)
		},
	}
	tmpl, err := template.New("doc").Funcs(funcs).Parse(string(docHTMLTemplate))
	if err != nil {
		return nil, fmt.Errorf("parsing doc template: %w", err)
	}

	var gzbuf bytes.Buffer
	w := gzip.NewWriter(&gzbuf)
	if err := tmpl.Execute(w, doc); err != nil {
		return nil, fmt.Errorf("executing doc template: %w", err)
	}
	w.Close()

	return gzbuf.Bytes(), nil
}
