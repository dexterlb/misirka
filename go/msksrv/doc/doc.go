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
	APIName  string               `json:"name"`
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

type docCache struct {
	HTMLgz []byte
}

func (f *FullDoc) Validate() error {
	if f.APIName == "" {
		return fmt.Errorf("Don't be lazy! Set a name on the Server object with .Name(<text>)!")
	}

	if f.APIDescr == "" {
		return fmt.Errorf("Don't be lazy! Set a description on the Server object with .Descr(<text>)!")
	}

	for ct, c := range f.Calls {
		if c.Description == "" {
			return (fmt.Errorf("Don't be lazy! Set a description on %s with .Descr(<text>)!", ct))
		}
		if len(c.ExampleCalls) == 0 {
			return (fmt.Errorf("Don't be lazy! Set some examples on %s with .Example(<param>, <result>)", ct))
		}
	}

	for tt, t := range f.Topics {
		if t.Description == "" {
			return (fmt.Errorf("Don't be lazy! Set a description on %s with .Descr(<text>)!", tt))
		}
		if len(t.Examples) == 0 {
			return (fmt.Errorf("Don't be lazy! Set some examples on %s with .Example(<value>)!", tt))
		}
	}

	return nil
}

type RenderedDoc struct {
	Doc    *FullDoc
	HTMLgz []byte
}

func (f *FullDoc) Render() (*RenderedDoc, error) {
	err := f.Validate()
	if err != nil {
		return nil, err
	}
	htmlgz, err := f.buildHTMLgz()
	if err != nil {
		return nil, fmt.Errorf("could not build HTML documentation: %w", err)
	}
	return &RenderedDoc{
		Doc:    f,
		HTMLgz: htmlgz,
	}, nil
}

//go:embed doc.html
var docHTMLTemplate []byte

func (doc *FullDoc) buildHTMLgz() ([]byte, error) {
	var templErrors struct{ errors []error }
	funcs := template.FuncMap{
		"jsonify": func(x interface{}) string {
			data, err := json.Marshal(x)
			if err != nil {
				templErrors.errors = append(
					templErrors.errors,
					fmt.Errorf("could not jsonify item: %w", err),
				)
			}
			return string(data)
		},
	}

	if len(templErrors.errors) != 0 {
		return nil, templErrors.errors[0]
	}

	tmpl, err := template.New("doc").Funcs(funcs).Parse(string(docHTMLTemplate))
	if err != nil {
		return nil, (fmt.Errorf("parsing doc template: %w", err))
	}

	var gzbuf bytes.Buffer
	w := gzip.NewWriter(&gzbuf)
	if err := tmpl.Execute(w, doc); err != nil {
		return nil, (fmt.Errorf("executing doc template: %w", err))
	}
	w.Close()

	return gzbuf.Bytes(), nil
}
