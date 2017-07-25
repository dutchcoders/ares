# Package mail

[![Build Status](https://travis-ci.org/mohamedattahri/mail.svg?branch=master)](https://travis-ci.org/mohamedattahri/mail)  [![GoDoc](https://godoc.org/github.com/mohamedattahri/mail?status.svg)](https://godoc.org/github.com/mohamedattahri/mail)

Package mail implements composing and parsing of mail messages.

Creating a message with multiple parts and attachments ends up being a surprisingly painful task in Go. The net/mail package in the standard library only offers tools to parse mail messages and addresses.

This package can replace the net/mail in your code without breaking it.

Feedback and contributions are more than welcome.

## Features

- multipart support
- attachments (base64)
- quoted-printable encoding of body text
- quoted-printable decoding of headers
- getters and setters common headers

## Known issues

- Quoted-printable encoding does not respect the 76 characters per line limitation imposed by RFC 2045 (https://github.com/golang/go/issues/4943).

## Installation

Alex Cesaro's [quotedprintable package](https://godoc.org/gopkg.in/alexcesaro/quotedprintable.v1) is the only external dependency. It's likely to be included in Go 1.5 in a new [mime/quotedprintable](https://codereview.appspot.com/132680044) package.
```go
go get godoc.org/gopkg.in/alexcesaro/quotedprintable.v1
go get github.com/mohamedattahri/mail
```

## Examples

### Plain text message
```go
msg := NewMessage()
msg.SetFrom(sender)
msg.To().Add(recipient)
msg.SetSubject("Plain text message")
msg.SetContentType("text/plain")
fmt.Fprintf(msg.Body, "Hello, World!")
fmt.Println(msg)
```

### HTML and alternative text
```go
msg := NewMessage()
msg.SetFrom(sender)
msg.To().Add(recipient)
msg.SetSubject("HTML and alternative text")
alternative := NewMultipart("multipart/alternative", msg)
alternative.AddText("text/plain", text)
alternative.AddText("text/html", html)
alternative.Close()
fmt.Println(msg)
```

### Simple message with an attachment
```go
msg := NewMessage()
msg.SetFrom(sender)
msg.To().Add(recipient)
msg.SetSubject("Simple message with an attachment")
mixed := NewMultipart("multipart/mixed", msg)
mixed.AddText("text/plain", text)
mixed.AddAttachment(Attachment, "Gopher.png", "", attachment)
mixed.Close()
fmt.Println(msg)
```

### HTML message, alternative text and an attachment
```go
msg := NewMessage()
msg.SetFrom(sender)
msg.To().Add(recipient)
msg.SetSubject("HTML message, alternative text and an attachment")

mixed := NewMultipart("multipart/mixed", msg)
alternative, _ := mixed.AddPart("multipart/alternative", nil)
alternative.AddText("text/plain", text)
alternative.AddText("text/html", html)
alternative.Close()
mixed.AddAttachment(Inline, "Photo", "image/jpeg", attachment)
mixed.Close()

fmt.Println(msg)
```

### Attached image and cid URI Scheme

This example shows how to use the cid URI Scheme to use an attachment as a data source for an HTML img tag.

```go
msg := NewMessage()
msg.SetFrom(&Address{"Al Bumin", "a.bumin@example.name"})
msg.To().Add(&Address{"Polly Ester", "p.ester@example.com"})
msg.SetSubject("Message with HTML, alternative text, and an attachment")
mixed := NewMultipart("multipart/mixed", msg)
// filename is the name that will be suggested to a user who would like to
// download the attachment, but also the ID with which you can refer to the
// attachment in a cid URI scheme.
filename := "gopher.jpg"
// The src of the image in this HTML is set to use the attachment with the
// Content-ID filename.
html := fmt.Sprintf("<html><body><img src=\"cid:%s\"/></body></html>", filename)
mixed.AddText("text/html", bytes.NewReader([]byte(html)))
// Load the photo and add the attachment with filename.
attachment, _ := ioutil.ReadFile("path/of/image.jpg")
mixed.AddAttachment(Attachment, filename, "image/jpeg", bytes.NewReader(attachment))
// Closing mixed, the parent part.
mixed.Close()

fmt.Println(msg)
```
