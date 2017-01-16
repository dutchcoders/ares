package server

import (
	"github.com/PuerkitoBio/goquery"

	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"regexp"
	"strings"
)

type ActionRequester interface {
	OnRequest(*http.Request) (*http.Request, *http.Response, error)
}

type ActionRequestRedirect struct {
	*Action
}

func (a *ActionRequestRedirect) OnRequest(req *http.Request) (*http.Request, *http.Response, error) {
	r, w := io.Pipe()

	resp := &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       r,
		Request:    req,
		StatusCode: a.StatusCode,
	}

	resp.Header.Add("Content-Type", "text/html")
	resp.Header.Add("Location", a.Location)

	go func() {
		defer w.Close()
	}()

	return req, resp, nil
}

type ActionRequestServe struct {
	*Action
}

func (a *ActionRequestServe) OnRequest(req *http.Request) (*http.Request, *http.Response, error) {
	r, w := io.Pipe()

	resp := &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       r,
		Request:    req,
		StatusCode: 200,
	}

	resp.Header.Add("Content-Type", a.ContentType)

	go func() {
		defer w.Close()

		fmt.Println(a.Body)

		w.Write([]byte(a.Body))
	}()

	return req, resp, nil
}

type ActionRequestFile struct {
	*Action
}

func (a *ActionRequestFile) OnRequest(req *http.Request) (*http.Request, *http.Response, error) {
	r, w := io.Pipe()

	resp := &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       r,
		Request:    req,
		StatusCode: a.StatusCode,
	}

	resp.Header.Add("Content-Type", a.ContentType)
	go func() {
		defer w.Close()

		if tmpl, err := template.ParseFiles(a.File); err != nil {
			log.Errorf("Error opening file: %s: %s", a.File, err.Error())
		} else if err = tmpl.Execute(w, req); err != nil {
			log.Errorf("Error opening file: %s: %s", a.File, err.Error())
		} else {
		}
	}()

	return req, resp, nil
}

type ActionResponserer interface {
	OnResponse(*http.Request, *http.Response) (*http.Response, error)
}

type ActionResponseReplace struct {
	*Action
}

func (a *ActionResponseReplace) OnResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
	if resp.StatusCode < 200 {
		return resp, nil
	}

	if resp.StatusCode >= 300 {
		return resp, nil
	}

	contentType := ""
	if val := resp.Header.Get("Content-Type"); val != "" {
		contentType = val
	}

	mt, _, _ := mime.ParseMediaType(contentType)
	if !strings.HasPrefix(mt, "text/html") {
		return resp, nil
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err == io.EOF {
		return resp, nil
	} else if err != nil {
		log.Errorf("Error reading response body: %s", err.Error())
		return resp, err
	}

	html := string(b)

	re := regexp.MustCompile(a.Regex)
	html = re.ReplaceAllString(html, a.Replace)

	resp.Body = ioutil.NopCloser(strings.NewReader(html))
	return resp, nil
}

type ActionResponseInject struct {
	*Action
}

func (a *ActionResponseInject) OnResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
	if resp.StatusCode < 200 {
		return resp, nil
	}

	if resp.StatusCode >= 300 {
		return resp, nil
	}

	contentType := ""
	if val := resp.Header.Get("Content-Type"); val != "" {
		contentType = val
	}

	mt, _, _ := mime.ParseMediaType(contentType)
	if !strings.HasPrefix(mt, "text/html") {
		return resp, nil
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err == io.EOF {
		return resp, nil
	} else if err != nil {
		log.Error("Error parsing document: %s", err.Error())
		return resp, err
	}

	body := doc.Find("body")
	for _, script := range a.Scripts {
		log.Infof("Injecting script %s.", script)
		if b, err := ioutil.ReadFile(script); err != nil {
			log.Errorf("Error injecting: %s", err.Error())
		} else {
			body.AppendHtml(string(b))
		}
	}

	html, _ := doc.Html()

	resp.Body = ioutil.NopCloser(strings.NewReader(html))
	return resp, nil
}
