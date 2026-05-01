package msksrv

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/doc"
)

func (s *Server) HandleDoc() {
	s.HandleDocAt("doc", "doc.html")
}

func (s *Server) HandleDocAt(path string, htmlPath string) {
	fullDoc := &doc.FullDoc{
		APIDescr: s.apiDescr,
		Topics:   make(map[string]*doc.TopicDoc),
		Calls:    make(map[string]*doc.CallDoc),
	}
	for tp := range s.topics {
		fullDoc.Topics[tp] = &s.topics[tp].Doc
	}
	for cp := range s.calls {
		fullDoc.Calls[cp] = &s.calls[cp].Doc
	}

	fullDoc.Validate()

	handleDoc := func(arg struct{}) (*doc.FullDoc, error) {
		return fullDoc, nil
	}

	htmlgz, err := fullDoc.HTMLgz()
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

	exampleDoc := &doc.FullDoc{APIDescr: "<this documentation>"}

	AddCall(s, path, handleDoc).
		Descr("get documentation for this API").
		Example(struct{}{}, exampleDoc)

	if htmlPath != "" {
		AddCall(s, htmlPath, handleDocHTMLgz).
			Descr("get documentation for this API in human-readeble HTML")
	}
}

func (c *CallMeta[P, R]) Descr(descr string) *CallMeta[P, R] {
	c.info.Doc.Description = descr
	return c
}

func (c *CallMeta[P, R]) Example(param P, result R) *CallMeta[P, R] {
	c.info.Doc.ExampleCalls = append(c.info.Doc.ExampleCalls, doc.ExampleCall{
		Param:  param,
		Result: result,
	})
	return c
}

func (t *TopicMeta[T]) Descr(descr string) *TopicMeta[T] {
	t.info.Doc.Description = descr
	return t
}

func (t *TopicMeta[T]) Example(val any) *TopicMeta[T] {
	t.info.Doc.Examples = append(t.info.Doc.Examples, val)
	return t
}

func (s *Server) Descr(descr string) *Server {
	s.apiDescr = descr
	return s
}
