package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"

	"github.com/patrickmn/go-cache"

	"net/url"

	"github.com/gorilla/mux"
	logging "github.com/op/go-logging"

	"crypto/tls"
	"flag"

	"golang.org/x/crypto/acme/autocert"

	"github.com/dutchcoders/ares/api"
	"github.com/dutchcoders/ares/database"
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

	index chan interface{}

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

	db *database.Database
}

func New(options ...func(*Server)) *Server {
	c := cache.New(5*time.Minute, 30*time.Second)

	p := &Server{
		config: &config{},
		index:  make(chan interface{}, 500),
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

	if db, err := database.Open(p.config.MongoURL); err != nil {
		panic(err)
	} else {
		p.db = db
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

	go func() {
		a := api.New(c.db)
		a.Serve()
	}()

	if c.ListenerTLS == "" {
	} else {
		cacheDir := "./cache/"

		/*
			if c.LetsEncryptCache != "" {
				cacheDir = c.LetsEncryptCache
			}
		*/

		m := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Cache:  autocert.DirCache(cacheDir),
			HostPolicy: func(_ context.Context, host string) error {
				/*
					if !strings.HasSuffix(host, "tor.onl") {
						return errors.New("acme/autocert: host not configured")
					}
					return nil
				*/
				return nil
			},
		}

		go func() {
			/*
				certificate, err := tls.LoadX509KeyPair(c.ServerCertificateFile, c.ServerKeyFile)
				if err != nil {
					log.Errorf("Error loading certificate: %s", err.Error())
					return
				}

				certFn := func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
					if len(certificate.Certificate) == 0 {
						return nil, fmt.Errorf("No certificate configured.")
					} else if x509Cert, err := x509.ParseCertificate(certificate.Certificate[0]); err != nil {
						return nil, err
					} else if err := x509Cert.VerifyHostname(clientHello.ServerName); err != nil {
						return nil, err
					}

					return &certificate, nil
				}

			*/
			s := &http.Server{
				Addr:    ":https",
				Handler: handler,
				TLSConfig: &tls.Config{
					GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
						/*
							if c, err := certFn(clientHello); err == nil {
								return c, err
							}
						*/

						return m.GetCertificate(clientHello)
					},
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
