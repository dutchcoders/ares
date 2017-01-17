FROM golang:1.7-alpine

ADD . /go/src/github.com/dutchcoders/ares

RUN go build -o /go/bin/ares github.com/dutchcoders/ares

ENTRYPOINT /go/bin/ares -p 0.0.0.0:8080

EXPOSE 8080
