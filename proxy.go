package ares

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"golang.org/x/net/proxy"
	"io"
	"io/ioutil"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pborman/uuid"

	"context"
	"crypto/sha256"
	"github.com/gorilla/mux"
	logging "github.com/op/go-logging"
	"gopkg.in/olivere/elastic.v5"
	"net/url"
)

var log = logging.MustGetLogger("proxy")

type Proxy struct {
	listener net.Listener

	Hosts map[string]string `toml:"hosts"`

	Socks string `toml:"socks"`

	ListenerString    string `toml:"listener"`
	TLSListenerString string `toml:"tlslistener"`

	CACertificateFile     string `toml:"ca_cert"`
	ServerCertificateFile string `toml:"server_cert"`
	ServerKeyFile         string `toml:"server_key"`
	AuthType              string `toml:"authenticationtype"`

	Logging []struct {
		Output string `toml:"output"`
		Level  string `toml:"level"`
	} `toml:"logging"`

	Servers []Server `toml:"servers"`

	router *mux.Router

	index chan Pair
	p     *Proxy
}

type Server struct {
	Host    string `toml:"host"`
	Backend string `toml:"backend"`
}

func StaticHandler(path string) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctype := mime.TypeByExtension(filepath.Ext(path))
		rw.Header().Set("Content-Type", ctype)

		rw.WriteHeader(http.StatusOK)

		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()

		if r.Method != "HEAD" {
			io.Copy(rw, f)
		}

		return
	}
}

type Transport struct {
	http.RoundTripper
	Proxy *Proxy
}

func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	dump, _ := httputil.DumpRequest(req, false)
	log.Debugf("Request: %s\n\n", string(dump))

	req.Header.Del("If-Modified-Since")
	req.Header.Del("Referer")

	defer req.Body.Close()

	var body []byte
	if body, err = ioutil.ReadAll(req.Body); err != nil {
		log.Errorf("Error reading body: %s", err.Error())
		return
	}

	req.Body = ioutil.NopCloser(bytes.NewReader(body))
	if resp, err = t.RoundTripper.RoundTrip(req); err != nil {
		return nil, err
	}

	dump, _ = httputil.DumpResponse(resp, false)
	log.Debugf("Response: %s\n", string(dump))

	contentType := ""
	if val := resp.Header.Get("Content-Type"); val != "" {
		contentType = val
	}

	mt, _, _ := mime.ParseMediaType(contentType)

	switch mt {
	case "text/html":
		// change cookie
	case "text/javascript":
	}

	cookies := map[string]string{}
	for _, cookie := range req.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	host, _, _ := net.SplitHostPort(req.RemoteAddr)

	pair := Pair{
		Date:       time.Now(),
		RemoteAddr: host,
		Meta: map[string]interface {
		}{},
		Request: &Request{
			Method:        req.Method,
			URL:           req.URL.String(),
			Proto:         req.Proto,
			Header:        req.Header,
			ContentLength: req.ContentLength,
			Host:          req.Host,
			Cookies:       cookies,
			Body:          string(body),
		},
		Response: &Response{
			StatusCode:    resp.StatusCode,
			Proto:         resp.Proto,
			Header:        resp.Header,
			ContentLength: resp.ContentLength,
			Body:          "",
		},
	}

	// calculate hash
	extension := ""
	if v, err := mime.ExtensionsByType(resp.Header.Get("Content-Type")); err != nil {
	} else if len(v) == 0 {
	} else {
		extension = v[0]
	}

	func() {
		if resp.StatusCode >= 300 {
			return
		}

		var rdr io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			rdr, _ = gzip.NewReader(rdr)
		}

		resp.Header.Del("Content-Encoding")

		hasher := sha256.New()

		rdr = io.TeeReader(rdr, hasher)

		var body []byte
		if body, err = ioutil.ReadAll(rdr); err != nil {
			log.Errorf("Error reading body: %s", err.Error())
			return
		}

		hash := fmt.Sprintf("%x", hasher.Sum(nil))
		pair.Response.Hash.SHA256 = hash

		path := fmt.Sprintf("/data/%s/%s/%s", req.URL.Host, string(hash[0]), string(hash[1]))

		for {
			if _, err := os.Stat(fmt.Sprintf("%s/%s%s", path, hash, extension)); os.IsNotExist(err) {
			} else if err != nil {
				log.Errorf("Error stat path: %s", err.Error())
				break
			}

			if err := os.MkdirAll(path, 0750); err != nil {
				log.Errorf("Error creating directory: %s", err.Error())
			} else if err := ioutil.WriteFile(fmt.Sprintf("%s/%s%s", path, hash, extension), body, 0640); err != nil {
				log.Errorf("Error writing to file %s", err.Error())
			}

			break
		}

		// we should see if we can .Write to body conn
		c := cache.New(5*time.Minute, 30*time.Second)
		c.Set(req.URL.String(), hash, cache.DefaultExpiration)

		resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	}()

	if strings.HasPrefix(mt, "text/") {
		rdr := resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			rdr, _ = gzip.NewReader(rdr)
		}

		b, _ := ioutil.ReadAll(rdr)

		bs := string(b)
		pair.Response.Body = string(b)

		for h, v := range t.Proxy.Hosts {
			bs = strings.Replace(bs, v, h, -1)
		}

		// todo(): replace headers

		if strings.HasPrefix(mt, "text/html") {
			resp.Body = ioutil.NopCloser(strings.NewReader(bs))
		} else {
			resp.Body = ioutil.NopCloser(strings.NewReader(bs))
		}

		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")

		if err := req.ParseForm(); err == nil {
			form := map[string][]string{}
			for k, v := range req.Form {
				form[k] = v
			}

			pair.Meta["form"] = form
		} else {
			log.Errorf("Error parsing form: %s", err.Error())
		}

		query := map[string][]string{}
		for k, v := range req.URL.Query() {
			query[k] = v
		}
		pair.Meta["query"] = query

		if username, password, ok := req.BasicAuth(); ok {
			pair.Meta["authorization"] = struct {
				Type     string `json:"type"`
				Username string `json:"username"`
				Password string `json:"password"`
			}{
				Type:     "basic",
				Username: username,
				Password: password,
			}
		}

		// gzip?
	}

	t.Proxy.index <- pair

	return
}

func New() *Proxy {
	return &Proxy{
		index: make(chan Pair, 500),
	}
}

type Pair struct {
	Date       time.Time              `json:"date"`
	RemoteAddr string                 `json:"remote_addr"`
	Meta       map[string]interface{} `json:"meta"`
	Request    *Request               `json:"request"`
	Response   *Response              `json:"response"`
}

type Request struct {
	Method        string              `json:"method"`
	URL           string              `json:"url"`
	Proto         string              `json:"proto"`
	Host          string              `json:"host"`
	Cookies       map[string]string   `json:"cookies"`
	ContentLength int64               `json:"content_length"`
	Header        map[string][]string `json:"headers"`
	Body          string              `json:"body"`
}

type Response struct {
	StatusCode    int                 `json:"status_code"`
	ContentLength int64               `json:"content_length"`
	Proto         string              `json:"proto"`
	Header        map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	Hash          struct {
		SHA256 string `json:"sha256"`
	} `json:"hashes"`
}

func (p *Proxy) StartIndexer() {
	go func() {
		es, err := elastic.NewClient(elastic.SetURL("http://127.0.0.1:9200/"), elastic.SetSniff(false))
		if err != nil {
			panic(err)
		}

		bulk := es.Bulk()

		count := 0
		for {
			select {
			case pair := <-p.index:
				pairId := uuid.NewUUID()
				bulk = bulk.Add(elastic.NewBulkIndexRequest().
					Index("ares").
					Type("pairs").
					Id(pairId.String()).
					Doc(pair),
				)

				log.Debugf("Indexed message with id %s", pairId.String())

				if bulk.NumberOfActions() < 100 {
					continue
				}
			case <-time.After(time.Second * 10):
			}

			if bulk.NumberOfActions() == 0 {
			} else if response, err := bulk.Do(context.Background()); err != nil {
				log.Errorf("Error indexing: %s", err.Error())
			} else {
				indexed := response.Indexed()
				count += len(indexed)

				log.Infof("Bulk indexing: %d total %d.\n", len(indexed), count)
			}
		}
	}()
}

type redirectHandler struct {
}

func (rh *redirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u := *r.URL
	u.Scheme = "https"
	u.Host = r.Host
	log.Debug("Redirecting to: %s %s", r.URL.String(), u.String())
	http.Redirect(w, r, u.String(), 301)
}

func RedirectHandler() http.Handler {
	return &redirectHandler{}
}

const (
	ApacheFormatPattern = "%s - - [%s] %s \"%s %d %d\" %f\n"
)

func (c *Proxy) ListenAndServe() {
	log.Info("Starting Ares proxy....")

	var router = mux.NewRouter()

	u, err := url.Parse(c.Socks)
	if err != nil {
		panic(err)
	}

	d, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		panic(err)
	}

	director := func(req *http.Request) {
		if h, ok := c.Hosts[req.Host]; ok {
			req.Host = h
		}

		req.URL.Scheme = "http" //target.Scheme
		req.URL.Host = req.Host

		log.Debugf("Using backend: %s", req.URL.String())
	}

	ph := NewSingleHostReverseProxy(director)
	ph.Transport = &Transport{
		RoundTripper: &http.Transport{
			Dial: d.Dial,
		},
		Proxy: c,
	}

	router.NotFoundHandler = ph

	for _, l := range c.Logging {

		backend1 := logging.NewLogBackend(os.Stderr, "", 0)
		backend1Leveled := logging.AddModuleLevel(backend1)
		backend1Leveled.SetLevel(logging.ERROR, "")
		logging.SetBackend(backend1Leveled)

		level, err := logging.LogLevel(l.Level)
		if err != nil {
			panic(err)
		}

		backend1Leveled.SetLevel(level, "")
	}

	server := &http.Server{
		Addr:    c.ListenerString,
		Handler: NewApacheLoggingHandler(router, log.Infof),
	}

	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
