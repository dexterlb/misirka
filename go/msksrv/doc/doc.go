package doc

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/goccy/go-json"
)

type FullDoc struct {
	APIDescr string               `json:"description"`
	Calls    map[string]*CallDoc  `json:"calls"`
	Topics   map[string]*TopicDoc `json:"topics"`
}

type CallDoc struct {
	Description      string        `json:"description"`
	ExampleCalls     []ExampleCall `json:"example_calls"`
	PathValueAliases []string      `json:"path_value_aliases,omitempty"`
}

type TopicDoc struct {
	Description string        `json:"description"`
	Examples    []interface{} `json:"examples"`
}

type ExampleCall struct {
	Param  interface{} `json:"param"`
	Result interface{} `json:"result"`
}

func (f *FullDoc) Validate() {
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

//go:embed doc.html
var docHTMLTemplate []byte

func (doc *FullDoc) HTMLgz() ([]byte, error) {
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
