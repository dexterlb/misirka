package msksrvbuilder

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/dexterlb/misirka/go/msksrv"
	"github.com/dexterlb/misirka/go/msksrv/backends/httpbackend"
	"github.com/dexterlb/misirka/go/msksrv/backends/wsbackend"
)

type ServerBuildConfig struct {
	HTTPBackend HTTPBackendBuildConfig `yaml:"http"`
	WSBackend   WSBackendBuildConfig   `yaml:"ws"`
	Doc         DocBuildConfig         `yaml:"doc"`
}

type HTTPBackendBuildConfig struct {
	Enable      bool   `yaml:"enable"`
	BindAddress string `yaml:"bind"`
	Prefix      string `yaml:"prefix"`
}

type WSBackendBuildConfig struct {
	Enable bool   `yaml:"enable"`
	URL    string `yaml:"url"`
}

type DocBuildConfig struct {
	Enable   bool   `yaml:"enable"`
	Path     string `yaml:"path"`
	HTMLPath string `yaml:"html_path"`
}

var DefaultServerBuildConfig = ServerBuildConfig{
	HTTPBackend: HTTPBackendBuildConfig{
		Enable: true,
		Prefix: "",
	},
	WSBackend: WSBackendBuildConfig{
		Enable: true,
		URL:    "/ws",
	},
	Doc: DocBuildConfig{
		Enable:   true,
		Path:     "doc",
		HTMLPath: "doc.html",
	},
}

func BuildServerFromYaml(errHandler func(error), filename string) (*msksrv.Server, *MainLoop, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open %s: %s", filename, err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			_ = fmt.Errorf("could not close %s: %s", filename, err)
		}
	}(f)

	m := yaml.NewDecoder(f, yaml.Strict())

	var cfg = DefaultServerBuildConfig

	err = m.Decode(&cfg)
	if err != nil {
		return nil, nil, err
	}

	srv, ml := BuildServer(errHandler, &cfg)
	return srv, ml, nil
}

func BuildServer(errHandler func(error), cfg *ServerBuildConfig) (*msksrv.Server, *MainLoop) {
	ml := &MainLoop{
		cfg:          *cfg,
		errHandler:   errHandler,
		srv:          msksrv.New(errHandler),
		httpBindAddr: cfg.HTTPBackend.BindAddress,
	}

	if cfg.HTTPBackend.Enable {
		ml.addHTTPBackend(&cfg.HTTPBackend)
	}

	if cfg.WSBackend.Enable {
		ml.addWSBackend(&cfg.WSBackend)
	}

	return ml.srv, ml
}

type MainLoop struct {
	srv          *msksrv.Server
	errHandler   func(error)
	httpBindAddr string
	httpMux      *http.ServeMux
	cfg          ServerBuildConfig
}

func (m *MainLoop) Run() error {
	if m.cfg.Doc.Enable {
		m.addDocs(&m.cfg.Doc)
	}

	m.srv.Begin()

	if m.httpMux != nil {
		return m.listenHTTP()
	}

	infiniteWait := make(chan struct{})
	<-infiniteWait

	return nil
}

func (m *MainLoop) listenHTTP() error {
	srv := &http.Server{}
	srv.Handler = m.httpMux
	srv.Addr = m.httpBindAddr
	return srv.ListenAndServe()
}

func (m *MainLoop) Server() *msksrv.Server {
	return m.srv
}

func (m *MainLoop) AddRawHttpHandler(url string, handler http.Handler) {
	m.wantHTTP()
	m.httpMux.Handle(url, handler)
}

func (m *MainLoop) addHTTPBackend(cfg *HTTPBackendBuildConfig) {
	m.wantHTTP()
	hb := httpbackend.New(m.errHandlerFor("http backend"))

	prefix, _ := strings.CutSuffix(cfg.Prefix, "/")
	prefixSlash := fmt.Sprintf("%s/", prefix)
	m.httpMux.Handle(prefixSlash, http.StripPrefix(prefix, hb.Handler()))

	m.srv.AddBackend(hb)
}

func (m *MainLoop) addWSBackend(cfg *WSBackendBuildConfig) {
	m.wantHTTP()
	wb := wsbackend.New(m.errHandlerFor("ws backend"))

	m.httpMux.Handle(cfg.URL, wb.WSHTTPHandler())

	m.srv.AddBackend(wb)
}

func (m *MainLoop) addDocs(cfg *DocBuildConfig) {
	m.srv.HandleDocAt(cfg.Path, cfg.HTMLPath)
}

func (m *MainLoop) errHandlerFor(sub string) func(error) {
	return func(err error) { fmt.Errorf("[%s] %w", sub, err) }
}

func (m *MainLoop) wantHTTP() {
	if m.httpMux == nil {
		m.httpMux = http.NewServeMux()
	}
}
