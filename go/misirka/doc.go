package misirka

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"text/template"

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
		panic("Don't be lazy! Set a description on the Misirka object with .Descr(<text>)!")
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

func (t *TopicMeta) Descr(descr string) *TopicMeta {
	t.info.doc.Description = descr
	return t
}

func (t *TopicMeta) Example(val any) *TopicMeta {
	t.info.doc.Examples = append(t.info.doc.Examples, val)
	return t
}

func (m *Misirka) Descr(descr string) *Misirka {
	m.apiDescr = descr
	return m
}

//go:embed doc.html
var docHTMLTemplate []byte

func (m *Misirka) docHTMLgz(doc *fullDoc) ([]byte, error) {
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
