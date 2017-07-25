package server

import (
	"io"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/op/go-logging"
)

type config struct {
	Hosts []Host `toml:"host"`

	Socks            string `toml:"socks"`
	ElasticsearchURL string `toml:"elasticsearch_url"`

	MongoURL string `toml:"mongodb_uri"`

	Listener    string `toml:"listener"`
	ListenerTLS string `toml:"tlslistener"`

	Data string `toml:"data"`

	Logging []struct {
		Output string `toml:"output"`
		Level  string `toml:"level"`
	} `toml:"logging"`
}

type Host struct {
	Host    string   `toml:"host"`
	Target  string   `toml:"target"`
	Actions []Action `toml:"action"`
}

type Action struct {
	Path        string   `toml:"path"`
	Method      []string `toml:"method"`
	RemoteAddr  []string `toml:"remote_addr"`
	Location    string   `toml:"location"`
	Action      string   `toml:"action"`
	StatusCode  int      `toml:"statuscode"`
	ContentType string   `toml:"content_type"`
	Body        string   `toml:"body"`
	UserAgent   []string `toml:"user_agent"`
	Scripts     []string `toml:"scripts"`

	Regex   string `toml:"regex"`
	Replace string `toml:"replace"`
	File    string `toml:"file"`
}

func Config(val string) func(*Server) {
	return func(server *Server) {
		if _, err := toml.DecodeFile(val, &server); err != nil {
			panic(err)
		}

		logBackends := []logging.Backend{}
		for _, log := range server.Logging {
			var err error

			var output io.Writer = os.Stdout

			switch log.Output {
			case "stdout":
				output = os.Stdout
			case "stderr":
				output = os.Stderr
			default:
				output, err = os.OpenFile(os.ExpandEnv(log.Output), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
			}

			if err != nil {
				panic(err)
			}

			backend := logging.NewLogBackend(output, "", 0)
			backendFormatter := logging.NewBackendFormatter(backend, format)
			backendLeveled := logging.AddModuleLevel(backendFormatter)

			level, err := logging.LogLevel(log.Level)
			if err != nil {
				panic(err)
			}

			backendLeveled.SetLevel(level, "")

			logBackends = append(logBackends, backendLeveled)
		}

		logging.SetBackend(logBackends...)
	}
}

func Address(addr string) func(*Server) {
	return func(s *Server) {
		s.Listener = addr
	}
}

func TLSAddress(addr string) func(*Server) {
	return func(server *Server) {
		server.ListenerTLS = addr
	}
}
