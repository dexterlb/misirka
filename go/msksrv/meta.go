package msksrv

import (
	"bytes"
	_ "embed"

	"github.com/dexterlb/misirka/go/mskdata"
	"github.com/dexterlb/misirka/go/msksrv/doc"
)

func (s *Server) HandleDoc() {
	s.HandleDocAt("doc", "doc.html")
}

func (s *Server) HandleDocAt(path string, htmlPath string) {
	s.assertNotBegun()
	s.docWanted = true

	exampleDoc := &doc.FullDoc{APIDescr: "<this documentation>"}

	AddCall(s, path, s.docHandler).
		Descr("get documentation for this API").
		Example(struct{}{}, exampleDoc)

	if htmlPath != "" {
		AddCall(s, htmlPath, s.docHTMLgzHandler).
			Descr("get documentation for this API in human-readeble HTML").
			Example(struct{}{}, s.htmlGzData([]byte("<gzipped HTML documentation>")))
	}
}

func (s *Server) docHandler(arg struct{}) (*doc.FullDoc, error) {
	return s.doc.Doc, nil
}

func (s *Server) docHTMLgzHandler(arg struct{}) (*mskdata.RawData, error) {
	return s.htmlGzData(s.doc.HTMLgz), nil
}
func (s *Server) htmlGzData(data []byte) *mskdata.RawData {
	return &mskdata.RawData{
		Data:            bytes.NewReader(data),
		MimeType:        "text/html",
		ContentEncoding: "gzip",
	}
}

func (s *Server) buildDocIfNeeded() error {
	if !s.docWanted {
		return nil
	}

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

	rd, err := fullDoc.Render()
	if err != nil {
		return err
	}

	s.doc = rd
	return nil
}

func (c *CallMeta[P, R]) Descr(descr string) *CallMeta[P, R] {
	c.s.assertNotBegun()
	c.info.Doc.Description = descr
	return c
}

func (c *CallMeta[P, R]) Example(param P, result R) *CallMeta[P, R] {
	c.s.assertNotBegun()
	c.info.Doc.ExampleCalls = append(c.info.Doc.ExampleCalls, doc.ExampleCall{
		Param:  param,
		Result: result,
	})
	return c
}

func (t *TopicMeta[T]) Descr(descr string) *TopicMeta[T] {
	t.s.assertNotBegun()
	t.info.Doc.Description = descr
	return t
}

func (t *TopicMeta[T]) Example(val any) *TopicMeta[T] {
	t.s.assertNotBegun()
	t.info.Doc.Examples = append(t.info.Doc.Examples, val)
	return t
}

func (s *Server) Descr(descr string) *Server {
	s.assertNotBegun()
	s.apiDescr = descr
	return s
}
