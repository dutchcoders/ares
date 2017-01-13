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

	"crypto/tls"
	"flag"
	"regexp"
	"rsc.io/letsencrypt"

	"github.com/PuerkitoBio/goquery"
	"path"
)

var log = logging.MustGetLogger("ares:proxy")

var (
	cachePath = flag.String("cache", "letsencrypt.cache", "cache path (default: letsencrypt.cache)")
)

type Proxy struct {
	listener net.Listener

	Hosts []Host `toml:"host"`

	Socks            string `toml:"socks"`
	ElasticsearchURL string `toml:"elasticsearch_url"`

	ListenerString    string `toml:"listener"`
	ListenerStringTLS string `toml:"tlslistener"`

	Data string `toml:"data"`

	Logging []struct {
		Output string `toml:"output"`
		Level  string `toml:"level"`
	} `toml:"logging"`

	Cache *cache.Cache

	router *mux.Router

	index chan *Pair
	p     *Proxy
}

type Host struct {
	Host    string   `toml:"host"`
	Target  string   `toml:"target"`
	Actions []Action `toml:"action"`
}

type Action struct {
	Path       string   `toml:"path"`
	Method     []string `toml:"method"`
	RemoteAddr []string `toml:"remote_addr"`
	Location   string   `toml:"location"`
	Action     string   `toml:"action"`
	StatusCode int      `toml:"statuscode"`
	Body       string   `toml:"body"`
	UserAgent  []string `toml:"user_agent"`
	Scripts    []string `toml:"scripts"`

	Regex   string `toml:"regex"`
	Replace string `toml:"replace"`
	File    string `toml:"file"`
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

func filter(action Action, req *http.Request) bool {
	if matched, _ := regexp.MatchString(action.Path, req.URL.Path); matched {
	} else {
		return false
	}

	CheckMethod := func(req *http.Request, methods []string) bool {
		if len(methods) == 0 {
			return true
		}

		for _, method := range methods {
			if method == req.Method {
				return true
			}
		}
		return false
	}

	if !CheckMethod(req, action.Method) {
		return false
	}

	CheckRemoteAddr := func(req *http.Request, addrs []string) bool {
		if len(addrs) == 0 {
			return true
		}

		remoteHost, _, _ := net.SplitHostPort(req.RemoteAddr)
		for _, remoteAddr := range addrs {
			if remoteAddr == remoteHost {
				return true
			}
		}
		return false
	}

	if !CheckRemoteAddr(req, action.RemoteAddr) {
		return false
	}

	CheckUserAgent := func(req *http.Request, agents []string) bool {
		if len(agents) == 0 {
			return true
		}

		for _, agent := range agents {
			if matched, _ := regexp.MatchString(agent, req.UserAgent()); matched {
				return true
			}
		}
		return false
	}

	if !CheckUserAgent(req, action.UserAgent) {
		return false
	}

	return true
}

func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	var host *Host
	var targetURL *url.URL

	for _, h := range t.Proxy.Hosts {
		u, err := url.Parse(h.Target)
		if err != nil {
			return nil, err
		}

		hst := req.Host
		if v, _, err := net.SplitHostPort(req.Host); err == nil {
			hst = v
		}

		if u.Host != hst {
			continue
		}

		targetURL = u
		host = &h
		break
	}

	if host == nil {
		r, w := io.Pipe()

		resp = &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       r,
			Request:    req,
			StatusCode: 404,
		}

		go func() {
			defer w.Close()
			w.Write([]byte("Host not configured."))
		}()
		return
	}

	dump, _ := httputil.DumpRequest(req, false)
	log.Debugf("Request: %s\n\n", string(dump))

	req.Header.Del("If-Modified-Since")
	req.Header.Del("Referer")

	defer req.Body.Close()

	for name, val := range req.Header {
		for i, _ := range val {
			val[i] = strings.Replace(val[i], host.Host, targetURL.Host, -1)
		}
		req.Header[name] = val
	}

	var body []byte
	if body, err = ioutil.ReadAll(req.Body); err != nil {
		log.Errorf("Error reading body: %s", err.Error())
		return
	}

	req.Body = ioutil.NopCloser(bytes.NewReader(body))

	for _, action := range host.Actions {
		if !filter(action, req) {
			continue
		}

		if action.Action == "redirect" {
			r, w := io.Pipe()

			resp = &http.Response{
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       r,
				Request:    req,
				StatusCode: action.StatusCode,
			}

			resp.Header.Add("Content-Type", "text/html")
			resp.Header.Add("Location", action.Location)

			go func() {
				defer w.Close()
				// w.Write([]byte("☄ HTTP status code returned!"))
			}()
		} else if action.Action == "serve" {
			r, w := io.Pipe()

			resp = &http.Response{
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       r,
				Request:    req,
				StatusCode: action.StatusCode,
			}

			resp.Header.Add("Content-Type", "text/html")

			go func() {
				defer w.Close()

				w.Write([]byte(action.Body))
			}()
		} else if action.Action == "file" {
			r, w := io.Pipe()

			resp := &http.Response{
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Body:       r,
				Request:    req,
				StatusCode: action.StatusCode,
			}

			// ready := make(chan struct{})
			// prw := &pipeResponseWriter{r, w, resp, ready}

			resp.Header.Add("Content-Type", "text/html")

			go func() {
				defer w.Close()

				if f, err := os.Open(action.File); err == nil {
					io.Copy(w, f)
				} else {
					log.Errorf("Error opening file: %s: %s", action.File, err.Error())
				}
			}()
		}
	}

	if resp != nil {
	} else if resp, err = t.RoundTripper.RoundTrip(req); err != nil {
		return nil, err
	}

	contentType := ""
	if val := resp.Header.Get("Content-Type"); val != "" {
		contentType = val
	}

	mt, _, _ := mime.ParseMediaType(contentType)

	cookies := map[string]string{}
	for _, cookie := range req.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	remoteHost, _, _ := net.SplitHostPort(req.RemoteAddr)
	pair := &Pair{
		Date:       time.Now(),
		RemoteAddr: remoteHost,
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

	defer func() {
		dump, _ = httputil.DumpResponse(resp, false)
		log.Debugf("Response: %s\n", string(dump))

		t.Proxy.index <- pair
	}()

	// calculate hash
	extension := ""
	if v, err := mime.ExtensionsByType(resp.Header.Get("Content-Type")); err != nil {
	} else if len(v) == 0 {
	} else {
		extension = v[0]
	}

	Save := func() {
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

		path := path.Join(t.Proxy.Data, fmt.Sprintf("/%s/%s/%s", req.URL.Host, string(hash[0]), string(hash[1])))

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

		t.Proxy.Cache.Set(req.URL.String(), hash, cache.DefaultExpiration)

		resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	}

	if t.Proxy.Data != "" {
		Save()
	}

	if strings.HasPrefix(mt, "text/") {
		rdr := resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			rdr, _ = gzip.NewReader(rdr)
		}

		b, _ := ioutil.ReadAll(rdr)
		pair.Response.Body = string(b)

		bs := string(b)
		for _, h := range t.Proxy.Hosts {
			u, err := url.Parse(h.Target)
			if err != nil {
				continue
			}

			bs = strings.Replace(bs, u.Host, h.Host, -1)
		}

		if strings.HasPrefix(mt, "text/html") {
			resp.Body = ioutil.NopCloser(strings.NewReader(bs))

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(bs))
			if err == io.EOF {
			} else if err != nil {
				log.Error("Error parsing document: %s", err.Error())
			}

			body := doc.Find("body")
			for _, action := range host.Actions {
				if !filter(action, req) {
					continue
				}

				if action.Action == "inject" {
					for _, script := range action.Scripts {
						log.Infof("Injecting script %s.", script)
						if b, err := ioutil.ReadFile(script); err != nil {
							log.Errorf("Error injecting: %s", err.Error())
						} else {
							body.AppendHtml(string(b))
						}
					}
				}
			}

			html, _ := doc.Html()

			for _, action := range host.Actions {
				if !filter(action, req) {
					continue
				}

				if action.Action == "replace" {
					re := regexp.MustCompile(action.Regex)
					html = re.ReplaceAllString(html, action.Replace)
				}
			}

			resp.Body = ioutil.NopCloser(strings.NewReader(html))
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

		for name, val := range resp.Header {
			for i, _ := range val {
				val[i] = strings.Replace(val[i], targetURL.Host, host.Host, -1)
			}
			resp.Header[name] = val
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
	resp.Header.Del("Content-Length")

	return
}

func New() *Proxy {
	c := cache.New(5*time.Minute, 30*time.Second)
	return &Proxy{
		index: make(chan *Pair, 500),
		Cache: c,
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

func (p *Proxy) startIndexer() {
	if p.ElasticsearchURL == "" {
		return
	}

	go func() {
		es, err := elastic.NewClient(elastic.SetURL(p.ElasticsearchURL), elastic.SetSniff(false))
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

				if bulk.NumberOfActions() < 10 {
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
	log.Info("Ares started....")
	defer log.Info("Ares stopped....")

	c.startIndexer()

	var router = mux.NewRouter()

	d := net.Dial

	if c.Socks == "" {
	} else if u, err := url.Parse(c.Socks); err != nil {
		panic(err)
	} else if v, err := proxy.FromURL(u, proxy.Direct); err != nil {
		panic(err)
	} else {
		d = v.Dial
	}

	director := func(req *http.Request) {
		for _, h := range c.Hosts {
			hst := req.Host
			if v, _, err := net.SplitHostPort(req.Host); err == nil {
				hst = v
			}

			if h.Host != hst {
				continue
			}

			u, err := url.Parse(h.Target)
			if err != nil {
				return
			}

			req.Host = u.Host
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
		}

		log.Debugf("Using backend: %s", req.URL.String())
	}

	ph := NewSingleHostReverseProxy(director)
	ph.Transport = &Transport{
		RoundTripper: &http.Transport{
			Dial: d,
		},
		Proxy: c,
	}

	router.NotFoundHandler = ph

	handler := NewApacheLoggingHandler(router, log.Infof)

	if c.ListenerStringTLS == "" {
	} else {
		go func() {
			var m letsencrypt.Manager
			if err := m.CacheFile(*cachePath); err != nil {
				log.Fatal(err)
			}

			s := &http.Server{
				Addr:    c.ListenerStringTLS,
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

	server := &http.Server{
		Addr:    c.ListenerString,
		Handler: handler,
	}

	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
