package ares

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ApacheLogRecord struct {
	http.ResponseWriter

	ip                          string
	time                        time.Time
	host, method, uri, protocol string
	status                      int
	responseBytes               int64
	elapsedTime                 time.Duration
}

func (r *ApacheLogRecord) Log(printFunc printFn) {
	timeFormatted := r.time.Format("02/Jan/2006 03:04:05")
	requestLine := fmt.Sprintf("%s %s %s", r.method, r.uri, r.protocol)
	printFunc(ApacheFormatPattern, r.ip, timeFormatted, r.host, requestLine, r.status, r.responseBytes,
		r.elapsedTime.Seconds())
}

func (r *ApacheLogRecord) Write(p []byte) (int, error) {
	written, err := r.ResponseWriter.Write(p)
	r.responseBytes += int64(written)
	return written, err
}

func (r *ApacheLogRecord) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

type ApacheLoggingHandler struct {
	handler   http.Handler
	printFunc printFn
}

type printFn func(string, ...interface{})

func NewApacheLoggingHandler(handler http.Handler, printFunc printFn) http.Handler {
	return &ApacheLoggingHandler{
		handler:   handler,
		printFunc: printFunc,
	}
}

func (h *ApacheLoggingHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}

	record := &ApacheLogRecord{
		ResponseWriter: rw,
		ip:             clientIP,
		time:           time.Time{},
		method:         r.Method,
		host:           r.Host,
		uri:            r.RequestURI,
		protocol:       r.Proto,
		status:         http.StatusOK,
		elapsedTime:    time.Duration(0),
	}

	startTime := time.Now()
	h.handler.ServeHTTP(record, r)
	finishTime := time.Now()

	record.time = finishTime.UTC()
	record.elapsedTime = finishTime.Sub(startTime)

	record.Log(h.printFunc)
}
