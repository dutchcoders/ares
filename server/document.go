package server

import (
	"time"
)

type Document struct {
	Date       time.Time              `json:"date"`
	RemoteAddr string                 `json:"remote_addr"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
	Request    *Request               `json:"request"`
	Response   *Response              `json:"response,omitempty"`
}

type Request struct {
	Method        string              `json:"method,omitempty"`
	URL           string              `json:"url,omitempty"`
	Proto         string              `json:"proto,omitempty"`
	Host          string              `json:"host,omitempty"`
	Cookies       map[string]string   `json:"cookies,omitempty"`
	ContentLength int64               `json:"content_length,omitempty"`
	Header        map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`
}

type Response struct {
	StatusCode    int                 `json:"status_code,omitempty"`
	ContentLength int64               `json:"content_length,omitempty"`
	Proto         string              `json:"proto,omitempty"`
	Header        map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`
	Hash          struct {
		SHA256 string `json:"sha256,omitempty"`
	} `json:"hashes,omitempty"`
}
