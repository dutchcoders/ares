package server

import (
	"golang.org/x/net/proxy"
	"net"
	"net/http"
	"time"

	"github.com/patrickmn/go-cache"

	"github.com/gorilla/mux"
	logging "github.com/op/go-logging"
	"net/url"

	"crypto/tls"
	"flag"
	"rsc.io/letsencrypt"
)

var format = logging.MustStringFormatter(
	"%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}",
)

var log = logging.MustGetLogger("ares:server")

var (
	cachePath = flag.String("cache", "letsencrypt.cache", "cache path (default: letsencrypt.cache)")
)

const (
	ApacheFormatPattern = "%s - - [%s] %s \"%s %d %d\" %f\n"
)

type Server struct {
	*config

	Cache *cache.Cache

	index chan Document

	// Director must be a function which modifies
	// the request into a new request to be sent
	// using Transport. Its response is then copied
	// back to the original client unmodified.
	Director func(*http.Request)

	// The transport used to perform proxy requests.
	// If nil, http.DefaultTransport is used.
	http.RoundTripper

	// FlushInterval specifies the flush interval
	// to flush to the client while copying the
	// response body.
	// If zero, no periodic flushing is done.
	FlushInterval time.Duration
}

func New(options ...func(*Server)) *Server {
	c := cache.New(5*time.Minute, 30*time.Second)

	p := &Server{
		config: &config{},
		index:  make(chan Document, 500),
		Cache:  c,
	}

	for _, optionFn := range options {
		optionFn(p)
	}

	d := net.Dial

	if p.Socks == "" {
	} else if u, err := url.Parse(p.Socks); err != nil {
		panic(err)
	} else if v, err := proxy.FromURL(u, proxy.Direct); err != nil {
		panic(err)
	} else {
		d = v.Dial
	}

	p.RoundTripper = &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return d(network, addr)
		},
		DialTLS: func(network, addr string) (net.Conn, error) {
			return tls.Dial(network, addr, &tls.Config{})
		},
	}

	return p
}

func (c *Server) Run() {
	log.Info("Ares started....")
	defer log.Info("Ares stopped....")

	if c.ElasticsearchURL != "" {
		go c.indexer()
	}

	var router = mux.NewRouter()
	router.NotFoundHandler = c

	handler := NewApacheLoggingHandler(router, log.Infof)

	if c.ListenerTLS == "" {
	} else {
		go func() {
			var m letsencrypt.Manager
			if err := m.CacheFile(*cachePath); err != nil {
				log.Fatal(err)
			}

			s := &http.Server{
				Addr:    c.ListenerTLS,
				Handler: handler,
				TLSConfig: &tls.Config{
					GetCertificate: m.GetCertificate,
				},
			}

			if err := s.ListenAndServeTLS("", ""); err != nil {
				panic(err)
			}
		}()
	}

	s := &http.Server{
		Addr:    c.Listener,
		Handler: handler,
	}

	if err := s.ListenAndServe(); err != nil {
		panic(err)
	}
}
