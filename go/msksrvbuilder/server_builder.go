package msksrvbuilder

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/dexterlb/misirka/go/msksrv"
	"github.com/dexterlb/misirka/go/msksrv/backends"
	"github.com/dexterlb/misirka/go/msksrv/backends/httpbackend"
	"github.com/dexterlb/misirka/go/msksrv/backends/mqttbackend"
	"github.com/dexterlb/misirka/go/msksrv/backends/wsbackend"
)

type ServerBuildConfig struct {
	HTTPBackend HTTPBackendBuildConfig `yaml:"http"`
	WSBackend   WSBackendBuildConfig   `yaml:"ws"`
	MQTTBackend MQTTBackendBuildConfig `yaml:"mqtt"`
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

type MQTTBackendBuildConfig struct {
	mqttbackend.Cfg `yaml:",inline"`
	Enable          bool `yaml:"enable"`
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

func BuildServerFromYaml(evtHandlers backends.EventHandlers, filename string) (*msksrv.Server, *MainLoop, error) {
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

	srv, ml := BuildServer(evtHandlers, &cfg)
	return srv, ml, nil
}

func BuildServer(evtHandlers backends.EventHandlers, cfg *ServerBuildConfig) (*msksrv.Server, *MainLoop) {
	ml := &MainLoop{
		cfg:          *cfg,
		evtHandlers:  evtHandlers,
		srv:          msksrv.New(),
		httpBindAddr: cfg.HTTPBackend.BindAddress,
	}

	if cfg.HTTPBackend.Enable {
		ml.addHTTPBackend(&cfg.HTTPBackend)
	}

	if cfg.WSBackend.Enable {
		ml.addWSBackend(&cfg.WSBackend)
	}

	if cfg.MQTTBackend.Enable {
		ml.addMQTTBackend(&cfg.MQTTBackend)
	}

	return ml.srv, ml
}

type MainLoop struct {
	srv          *msksrv.Server
	evtHandlers  backends.EventHandlers
	httpBindAddr string
	httpMux      *http.ServeMux
	mqttBackend  *mqttbackend.MQTTBackend
	cfg          ServerBuildConfig
}

func (m *MainLoop) Run() error {
	if m.cfg.Doc.Enable {
		m.addDocs(&m.cfg.Doc)
	}

	m.srv.Begin()

	if m.mqttBackend != nil {
		err := m.mqttBackend.Start(context.TODO())
		if err != nil {
			return fmt.Errorf("could not start MQTT backend: %w", err)
		}
	}

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
	m.evtHandlers.ForBackend("http").Info(
		"Starting HTTP server",
		map[string]interface{}{
			"bind_addr": srv.Addr,
		},
	)
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
	hb := httpbackend.New(m.evtHandlers.ForBackend("http"))

	prefix, _ := strings.CutSuffix(cfg.Prefix, "/")
	prefixSlash := fmt.Sprintf("%s/", prefix)
	m.httpMux.Handle(prefixSlash, http.StripPrefix(prefix, hb.Handler()))

	m.srv.AddBackend(hb)
}

func (m *MainLoop) addWSBackend(cfg *WSBackendBuildConfig) {
	m.wantHTTP()
	wb := wsbackend.New(m.evtHandlers.ForBackend("ws"))

	m.httpMux.Handle(cfg.URL, wb.WSHTTPHandler())

	m.srv.AddBackend(wb)
}

func (m *MainLoop) addMQTTBackend(cfg *MQTTBackendBuildConfig) {
	mb := mqttbackend.New(&cfg.Cfg, m.evtHandlers.ForBackend("mqtt"))

	m.srv.AddBackend(mb)
	m.mqttBackend = mb
}

func (m *MainLoop) addDocs(cfg *DocBuildConfig) {
	m.srv.HandleDocAt(cfg.Path, cfg.HTMLPath)
}

func (m *MainLoop) wantHTTP() {
	if m.httpMux == nil {
		m.httpMux = http.NewServeMux()
	}
}
