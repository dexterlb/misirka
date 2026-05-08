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

func (doc *FullDoc) Validate() error {
	if doc.APIName == "" {
		return fmt.Errorf("don't be lazy! Set a name on the Server object with .Name(<text>)")
	}

	if doc.APIDescr == "" {
		return fmt.Errorf("don't be lazy! Set a description on the Server object with .Descr(<text>)")
	}

	for ct, c := range doc.Calls {
		if c.Description == "" {
			return (fmt.Errorf("don't be lazy! Set a description on %s with .Descr(<text>)", ct))
		}
		if len(c.ExampleCalls) == 0 {
			return (fmt.Errorf("don't be lazy! Set some examples on %s with .Example(<param>, <result>)", ct))
		}
	}

	for tt, t := range doc.Topics {
		if t.Description == "" {
			return (fmt.Errorf("don't be lazy! Set a description on %s with .Descr(<text>)", tt))
		}
		if len(t.Examples) == 0 {
			return (fmt.Errorf("don't be lazy! Set some examples on %s with .Example(<value>)", tt))
		}
	}

	return nil
}

type RenderedDoc struct {
	Doc    *FullDoc
	HTMLgz []byte
}

func (doc *FullDoc) Render() (*RenderedDoc, error) {
	err := doc.Validate()
	if err != nil {
		return nil, err
	}
	htmlgz, err := doc.buildHTMLgz()
	if err != nil {
		return nil, fmt.Errorf("could not build HTML documentation: %w", err)
	}
	return &RenderedDoc{
		Doc:    doc,
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
