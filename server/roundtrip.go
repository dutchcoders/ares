package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"crypto/sha256"
	"net/url"

	"regexp"

	"github.com/PuerkitoBio/goquery"
	"path"
	"strconv"
)

func filter(action Action, req *http.Request) bool {
	if matched, _ := regexp.MatchString(action.Path, req.URL.RequestURI()); matched {
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

func HostNotConfigured(req *http.Request) (*http.Response, error) {
	r, w := io.Pipe()

	go func() {
		defer w.Close()

		w.Write([]byte("Host not configured."))
	}()

	return &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       r,
		Request:    req,
		StatusCode: 404,
	}, nil
}

func IsMediaType(contentType string, val string) bool {
	mt, _, _ := mime.ParseMediaType(contentType)
	return strings.HasPrefix(mt, val)
}

func (p *Server) GetHost(hst string) *Host {
	for _, h := range p.Hosts {
		if v, _, err := net.SplitHostPort(hst); err == nil {
			hst = v
		}

		if hst != h.Host {
			continue
		}

		return &h
	}

	return nil
}

/*
func hash(req *http.Request, resp *http.Response) (*http.Response, error) {
	if resp.StatusCode >= 300 {
		return resp, nil
	}

	hasher := sha256.New()

	rdr := io.TeeReader(resp.Body, hasher)

	var body []byte
	if body, err = ioutil.ReadAll(rdr); err != nil {
		return nil, err
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
_ = hash

	resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return resp, nil
}
*/

func (t *Server) saveToDisk(req *http.Request, resp *http.Response) (*http.Response, error) {
	if resp.StatusCode >= 300 {
		return resp, nil
	}

	hasher := sha256.New()

	rdr := io.TeeReader(resp.Body, hasher)

	var body []byte
	if v, err := ioutil.ReadAll(rdr); err != nil {
		return nil, err
	} else {
		body = v
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	extension := ""
	if v, err := mime.ExtensionsByType(resp.Header.Get("Content-Type")); err != nil {
	} else if len(v) == 0 {
	} else {
		extension = v[0]
	}

	path := path.Join(t.Data, fmt.Sprintf("/%s/%s/%s", req.URL.Host, string(hash[0]), string(hash[1])))

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

	resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return resp, nil
}

func (t *Server) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	remoteHost, _, _ := net.SplitHostPort(req.RemoteAddr)

	doc := &Document{
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
			Cookies:       nil,
			Body:          "",
		},
		Response: nil,
	}

	defer func(doc *Document) {
		t.index <- *doc
	}(doc)

	host := t.GetHost(req.Host)
	if host == nil {
		return HostNotConfigured(req)
	}

	var targetURL *url.URL
	if u, err := url.Parse(host.Target); err != nil {
		return nil, err
	} else {
		targetURL = u
	}

	req.Host = targetURL.Host
	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host

	dump, _ := httputil.DumpRequest(req, false)
	log.Debugf("Request: %s\n\n", string(dump))

	defer req.Body.Close()

	// update referer to target url
	if val := req.Header.Get("Referer"); val == "" {
	} else if u, err := url.Parse(val); err != nil {
	} else if targetURL.Host == u.Host {
		// replace url and scheme
		u.Scheme = targetURL.Scheme
		u.Host = targetURL.Host

		req.Header.Set("Referer", u.String())
	}

	// read body
	var body []byte
	if body, err = ioutil.ReadAll(req.Body); err == io.EOF {
		return
	} else if err != nil {
		log.Errorf("Error reading body: %s", err.Error())
		return
	}

	// don't like this
	doc.Request.Body = string(body)

	req.Body = ioutil.NopCloser(bytes.NewReader(body))

	for _, action := range host.Actions {
		if !filter(action, req) {
			continue
		}

		var a interface{} = nil

		if action.Action == "redirect" {
			a = &ActionRequestRedirect{
				Action: &action,
			}
		} else if action.Action == "serve" {
			a = &ActionRequestServe{
				Action: &action,
			}
		} else if action.Action == "file" {
			a = &ActionRequestFile{
				Action: &action,
			}
		}

		if a, ok := a.(ActionRequester); !ok {
		} else if req, resp, err = a.OnRequest(req); err != nil {
			log.Errorf("Error executing action: %s: %s", err.Error())
		} else if resp == nil {
		} else {
			// or do we want to have the injector and such run?
			return resp, err
		}
	}

	if resp != nil {
	} else if resp, err = t.RoundTripper.RoundTrip(req); err != nil {
		return nil, err
	}

	defer func() {
		// todo(nl5887): gzip response ?
		resp.Header.Del("Content-Length")

		resp.Header.Set("Server", "Ares (github.com/dutchcoders/server/)")

		dump, _ = httputil.DumpResponse(resp, false)
		log.Debugf("Response: %s\n", string(dump))
	}()

	// remove gzip encoding
	if resp.Header.Get("Content-Encoding") != "gzip" {
	} else if r, err := gzip.NewReader(resp.Body); err == io.EOF {
	} else if err != nil {
		log.Error("Error decoding gzip body: %s", err)
		return resp, err
	} else {
		resp.Body = r

		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
	}

	doc.Response = &Response{
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		Header:        resp.Header,
		ContentLength: resp.ContentLength,
		Body:          "",
	}

	type EnrichFunc func(req *http.Request, doc *Document) (*Document, error)

	funcs := []EnrichFunc{
		func(req *http.Request, doc *Document) (*Document, error) {
			cookies := map[string]string{}
			for _, cookie := range req.Cookies() {
				cookies[cookie.Name] = cookie.Value
			}
			doc.Request.Cookies = cookies
			return doc, nil
		},
		func(req *http.Request, doc *Document) (*Document, error) {
			// extraction of form
			if err := req.ParseForm(); err != nil {
				return nil, err
			}

			form := map[string][]string{}
			for k, v := range req.Form {
				form[k] = v
			}

			doc.Meta["form"] = form
			return doc, nil
		},
		func(req *http.Request, doc *Document) (*Document, error) {
			// extraction of query
			query := map[string][]string{}
			for k, v := range req.URL.Query() {
				query[k] = v
			}
			doc.Meta["query"] = query
			return doc, nil
		},
		func(req *http.Request, doc *Document) (*Document, error) {
			// extraction of basic authentication
			if username, password, ok := req.BasicAuth(); ok {
				doc.Meta["authorization"] = struct {
					Type     string `json:"type"`
					Username string `json:"username"`
					Password string `json:"password"`
				}{
					Type:     "basic",
					Username: username,
					Password: password,
				}
			}
			return doc, nil
		},
	}

	for _, fn := range funcs {
		if d, err := fn(req, doc); err != nil {
			log.Errorf("Error: %s", err.Error())
		} else {
			doc = d
		}
	}

	// todo(nl5887): calculate hash
	if t.Data == "" {
	} else if resp, err = t.saveToDisk(req, resp); err != nil {
		log.Error("Error saving response: %s", err.Error())
	}

	for _, action := range host.Actions {
		if !filter(action, req) {
			continue
		}

		var a interface{} = nil

		if action.Action == "inject" {
			a = &ActionResponseInject{
				Action: &action,
			}
		} else if action.Action == "replace" {
			a = &ActionResponseReplace{
				Action: &action,
			}
		}

		if a, ok := a.(ActionResponserer); !ok {
		} else if resp, err = a.OnResponse(req, resp); err != nil {
			log.Errorf("Error executing action: %s: %s", err.Error())
		} else {
			log.Debugf("Executed action: %s", action.Action)
		}
	}

	// we'll only store bodies for html documents
	if _, ok := resp.Header["Content-Length"]; !ok {
	} else if v, err := strconv.Atoi(resp.Header.Get("Content-Length")); err != nil {
	} else if v == 0 {
	} else if !IsMediaType(resp.Header.Get("Content-Type"), "text/html") {
	} else if d, err := goquery.NewDocumentFromReader(resp.Body); err == io.EOF {
		return resp, nil
	} else if err != nil {
		log.Error("Error parsing document: %s", err.Error())
		return resp, err
	} else {
		doc.Response.Body = d.Text()

		html, _ := d.Html()
		resp.Body = ioutil.NopCloser(strings.NewReader(html))
	}

	// rewrite location
	if val := resp.Header.Get("Location"); val == "" {
	} else if u, err := url.Parse(val); err != nil {
	} else if targetURL.Host == u.Host {
		// replace url and scheme
		u.Scheme = req.URL.Scheme
		u.Host = req.URL.Host

		resp.Header.Set("Location", u.String())
	}

	// rewrite cookie domains
	for i, line := range resp.Header["Set-Cookie"] {
		c := parseCookie(line)

		// do we want to remove secure and http only flags?
		c.Domain = host.Host

		resp.Header["Set-Cookie"][i] = c.String()
	}

	return
}
