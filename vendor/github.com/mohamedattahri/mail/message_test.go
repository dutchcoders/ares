// Use of this source code is governed by a BSD-style licence.
// Copyright 2011 The Go Authors. All rights reserved.
// Author: Mohamed Attahri <mohamed@attahri.com>

package mail

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Example of a simple message with plain text.
func ExampleMessage() {
	msg := NewMessage()
	msg.SetFrom(&Address{"Al Bumin", "a.bumin@example.name"})
	msg.To().Add(&Address{"Polly Ester", "p.ester@example.com"})
	msg.SetSubject("Simple message with plain text")
	msg.SetContentType("text/plain")
	fmt.Fprintf(msg.Body, "Hello, World!")

	fmt.Println(msg)
}

// Example of a message with HTML and alternative plain text.
func ExampleMessage_alternative() {
	text := bytes.NewReader([]byte("Hello, World!"))
	html := bytes.NewReader([]byte("<html><body>Hello, World!</body></html>"))

	msg := NewMessage()
	msg.SetFrom(&Address{"Al Bumin", "a.bumin@example.name"})
	msg.To().Add(&Address{"Polly Ester", "p.ester@example.com"})
	msg.SetSubject("Message with HTML and alternative text")

	alternative := NewMultipart("multipart/alternative", msg)
	alternative.AddText("text/plain", text)
	alternative.AddText("text/html", html)
	alternative.Close()

	fmt.Println(msg)
}

// Example of a message with HTML content, alternative text, and an attachment.
func ExampleMessage_mixed() {
	text := bytes.NewReader([]byte("Hello, World!"))
	html := bytes.NewReader([]byte("<html><body>Hello, World!</body></html>"))
	data, _ := ioutil.ReadFile("path/of/photo.jpg")
	attachment := bytes.NewReader(data)

	msg := NewMessage()
	msg.SetFrom(&Address{"Al Bumin", "a.bumin@example.name"})
	msg.To().Add(&Address{"Polly Ester", "p.ester@example.com"})
	msg.SetSubject("Message with HTML, alternative text, and an attachment")

	mixed := NewMultipart("multipart/mixed", msg)
	alternative, _ := mixed.AddMultipart("multipart/alternative")
	alternative.AddText("text/plain", text)
	alternative.AddText("text/html", html)
	alternative.Close()
	mixed.AddAttachment(Attachment, "Photo", "image/jpeg", attachment)
	mixed.Close()

	fmt.Println(msg)
}

// This example shows how to use the cid URI Scheme to use an attachment as a
// data source for an HTML img tag.
func ExampleMessage_cid() {
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
}

var parseTests = []struct {
	in     string
	header Header
	body   string
}{
	{
		// RFC 5322, Appendix A.1.1
		in: `From: John Doe <jdoe@machine.example>
To: Mary Smith <mary@example.net>
Subject: Saying Hello
Date: Fri, 21 Nov 1997 09:55:06 -0600
Message-ID: <1234@local.machine.example>

This is a message just to say hello.
So, "Hello".
`,
		header: Header{
			"From":       []string{"John Doe <jdoe@machine.example>"},
			"To":         []string{"Mary Smith <mary@example.net>"},
			"Subject":    []string{"Saying Hello"},
			"Date":       []string{"Fri, 21 Nov 1997 09:55:06 -0600"},
			"Message-Id": []string{"<1234@local.machine.example>"},
		},
		body: "This is a message just to say hello.\nSo, \"Hello\".\n",
	},
}

func TestParsing(t *testing.T) {
	for i, test := range parseTests {
		msg, err := ReadMessage(bytes.NewBuffer([]byte(test.in)))
		if err != nil {
			t.Errorf("test #%d: Failed parsing message: %v", i, err)
			continue
		}
		if !headerEq(msg.Header, test.header) {
			t.Errorf("test #%d: Incorrectly parsed message header.\nGot:\n%+v\nWant:\n%+v",
				i, msg.Header, test.header)
		}
		body, err := ioutil.ReadAll(msg.Body)
		if err != nil {
			t.Errorf("test #%d: Failed reading body: %v", i, err)
			continue
		}
		bodyStr := string(body)
		if bodyStr != test.body {
			t.Errorf("test #%d: Incorrectly parsed message body.\nGot:\n%+v\nWant:\n%+v",
				i, bodyStr, test.body)
		}
	}
}

func headerEq(a, b Header) bool {
	if len(a) != len(b) {
		return false
	}
	for k, as := range a {
		bs, ok := b[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(as, bs) {
			return false
		}
	}
	return true
}

func TestDateParsing(t *testing.T) {
	tests := []struct {
		dateStr string
		exp     time.Time
	}{
		// RFC 5322, Appendix A.1.1
		{
			"Fri, 21 Nov 1997 09:55:06 -0600",
			time.Date(1997, 11, 21, 9, 55, 6, 0, time.FixedZone("", -6*60*60)),
		},
		// RFC5322, Appendix A.6.2
		// Obsolete date.
		{
			"21 Nov 97 09:55:06 GMT",
			time.Date(1997, 11, 21, 9, 55, 6, 0, time.FixedZone("GMT", 0)),
		},
		// Commonly found format not specified by RFC 5322.
		{
			"Fri, 21 Nov 1997 09:55:06 -0600 (MDT)",
			time.Date(1997, 11, 21, 9, 55, 6, 0, time.FixedZone("", -6*60*60)),
		},
	}
	for _, test := range tests {
		hdr := Header{
			"Date": []string{test.dateStr},
		}
		date, err := hdr.Date()
		if err != nil {
			t.Errorf("Failed parsing %q: %v", test.dateStr, err)
			continue
		}
		if !date.Equal(test.exp) {
			t.Errorf("Parse of %q: got %+v, want %+v", test.dateStr, date, test.exp)
		}
	}
}

func TestAddressParsingError(t *testing.T) {
	const txt = "=?iso-8859-2?Q?Bogl=E1rka_Tak=E1cs?= <unknown@gmail.com>"
	_, err := ParseAddress(txt)
	if err == nil || !strings.Contains(err.Error(), "charset not supported") {
		t.Errorf(`mail.ParseAddress(%q) err: %q, want ".*charset not supported.*"`, txt, err)
	}
}

func TestAddressParsing(t *testing.T) {
	tests := []struct {
		addrsStr string
		exp      []*Address
	}{
		// Bare address
		{
			`jdoe@machine.example`,
			[]*Address{{
				Address: "jdoe@machine.example",
			}},
		},
		// RFC 5322, Appendix A.1.1
		{
			`John Doe <jdoe@machine.example>`,
			[]*Address{{
				Name:    "John Doe",
				Address: "jdoe@machine.example",
			}},
		},
		// RFC 5322, Appendix A.1.2
		{
			`"Joe Q. Public" <john.q.public@example.com>`,
			[]*Address{{
				Name:    "Joe Q. Public",
				Address: "john.q.public@example.com",
			}},
		},
		{
			`Mary Smith <mary@x.test>, jdoe@example.org, Who? <one@y.test>`,
			[]*Address{
				{
					Name:    "Mary Smith",
					Address: "mary@x.test",
				},
				{
					Address: "jdoe@example.org",
				},
				{
					Name:    "Who?",
					Address: "one@y.test",
				},
			},
		},
		{
			`<boss@nil.test>, "Giant; \"Big\" Box" <sysservices@example.net>`,
			[]*Address{
				{
					Address: "boss@nil.test",
				},
				{
					Name:    `Giant; "Big" Box`,
					Address: "sysservices@example.net",
				},
			},
		},
		// RFC 5322, Appendix A.1.3
		// TODO(dsymonds): Group addresses.

		// RFC 2047 "Q"-encoded ISO-8859-1 address.
		{
			`=?iso-8859-1?q?J=F6rg_Doe?= <joerg@example.com>`,
			[]*Address{
				{
					Name:    `Jörg Doe`,
					Address: "joerg@example.com",
				},
			},
		},
		// RFC 2047 "Q"-encoded US-ASCII address. Dumb but legal.
		{
			`=?us-ascii?q?J=6Frg_Doe?= <joerg@example.com>`,
			[]*Address{
				{
					Name:    `Jorg Doe`,
					Address: "joerg@example.com",
				},
			},
		},
		// RFC 2047 "Q"-encoded UTF-8 address.
		{
			`=?utf-8?q?J=C3=B6rg_Doe?= <joerg@example.com>`,
			[]*Address{
				{
					Name:    `Jörg Doe`,
					Address: "joerg@example.com",
				},
			},
		},
		// RFC 2047, Section 8.
		{
			`=?ISO-8859-1?Q?Andr=E9?= Pirard <PIRARD@vm1.ulg.ac.be>`,
			[]*Address{
				{
					Name:    `André Pirard`,
					Address: "PIRARD@vm1.ulg.ac.be",
				},
			},
		},
		// Custom example of RFC 2047 "B"-encoded ISO-8859-1 address.
		{
			`=?ISO-8859-1?B?SvZyZw==?= <joerg@example.com>`,
			[]*Address{
				{
					Name:    `Jörg`,
					Address: "joerg@example.com",
				},
			},
		},
		// Custom example of RFC 2047 "B"-encoded UTF-8 address.
		{
			`=?UTF-8?B?SsO2cmc=?= <joerg@example.com>`,
			[]*Address{
				{
					Name:    `Jörg`,
					Address: "joerg@example.com",
				},
			},
		},
		// Custom example with "." in name. For issue 4938
		{
			`Asem H. <noreply@example.com>`,
			[]*Address{
				{
					Name:    `Asem H.`,
					Address: "noreply@example.com",
				},
			},
		},
	}
	for _, test := range tests {
		if len(test.exp) == 1 {
			addr, err := ParseAddress(test.addrsStr)
			if err != nil {
				t.Errorf("Failed parsing (single) %q: %v", test.addrsStr, err)
				continue
			}
			if !reflect.DeepEqual([]*Address{addr}, test.exp) {
				t.Errorf("Parse (single) of %q: got %+v, want %+v", test.addrsStr, addr, test.exp)
			}
		}

		addrs, err := ParseAddressList(test.addrsStr)
		if err != nil {
			t.Errorf("Failed parsing (list) %q: %v", test.addrsStr, err)
			continue
		}
		if !reflect.DeepEqual(addrs, test.exp) {
			t.Errorf("Parse (list) of %q: got %+v, want %+v", test.addrsStr, addrs, test.exp)
		}
	}
}

func TestAddressFormatting(t *testing.T) {
	tests := []struct {
		addr *Address
		exp  string
	}{
		{
			&Address{Address: "bob@example.com"},
			"<bob@example.com>",
		},
		{
			&Address{Name: "Bob", Address: "bob@example.com"},
			`"Bob" <bob@example.com>`,
		},
		{
			// note the ö (o with an umlaut)
			&Address{Name: "Böb", Address: "bob@example.com"},
			`=?utf-8?q?B=C3=B6b?= <bob@example.com>`,
		},
		{
			&Address{Name: "Bob Jane", Address: "bob@example.com"},
			`"Bob Jane" <bob@example.com>`,
		},
		{
			&Address{Name: "Böb Jacöb", Address: "bob@example.com"},
			`=?utf-8?q?B=C3=B6b_Jac=C3=B6b?= <bob@example.com>`,
		},
	}
	for _, test := range tests {
		s := test.addr.String()
		if s != test.exp {
			t.Errorf("Address%+v.String() = %v, want %v", *test.addr, s, test.exp)
		}
	}
}

var (
	hendrixMail = &Address{Name: "Jimi Hendrix", Address: "jimi.hendrix@heaven.com"}
	vaughanMail = &Address{Name: "Stevie Ray Vaughan", Address: "stevie-ray.vaughan@heaven.com"}
)

func TestAddressListContain(t *testing.T) {
	raw := hendrixMail.String()
	list := AddressList{raw: &raw}
	if !list.Contain(hendrixMail) {
		t.Error("AddressList: expected to find", hendrixMail.String())
	}
}

func TestAddressListString(t *testing.T) {
	raw := ""
	list := AddressList{raw: &raw}
	list.Add(hendrixMail)
	if list.String() != hendrixMail.String() {
		t.Error("AddressList: returned", list.String(), "expecting:", hendrixMail.String())
	}
}

func TestAddressListAdd(t *testing.T) {
	raw := ""

	list := AddressList{raw: &raw}
	list.Add(hendrixMail)

	if !list.Contain(hendrixMail) {
		t.Error("AddressList: expected to find", hendrixMail.String())
	}
}

func TestAddressListRemove(t *testing.T) {
	raw := ""

	list := AddressList{raw: &raw}
	list.Add(hendrixMail)
	list.Remove(hendrixMail)

	if list.Contain(hendrixMail) {
		t.Error("AddressList: not expecting to find", hendrixMail.String())
	}
}

func TestMessageSubjectASCII(t *testing.T) {
	subject := "ASCII subject"
	msg := NewMessage()
	msg.SetSubject(subject)

	if msg.Subject() != subject {
		t.Error("Subject: do not match. found:", msg.Subject(), "expected:", subject)
	}
}

func TestMessageSubjectComplex(t *testing.T) {
	subject := "é è ê ç à â î"
	msg := NewMessage()
	msg.SetSubject(subject)

	if msg.Subject() != subject {
		t.Error("Subject: do not match. found:", msg.Subject(), "expected:", subject)
	}
}

func TestMessageMessageID(t *testing.T) {
	id := "9876-message-id"
	msg := NewMessage()
	msg.SetMessageID(id)

	if msg.MessageID() != id {
		t.Error("MessageID: do not match. found:", msg.MessageID(), "expected:", id)
	}
}

func TestMessageTo(t *testing.T) {
	msg := NewMessage()
	msg.To().Add(hendrixMail)

	if !msg.To().Contain(hendrixMail) {
		t.Error("To: do not match. found:", msg.To(), "expected:", hendrixMail)
	}
}

func TestMessageCc(t *testing.T) {
	msg := NewMessage()
	msg.Cc().Add(hendrixMail)

	if !msg.Cc().Contain(hendrixMail) {
		t.Error("Cc: do not match. found:", msg.Cc(), "expected:", hendrixMail)
	}
}

func TestMessageBcc(t *testing.T) {
	msg := NewMessage()
	msg.Bcc().Add(hendrixMail)

	if !msg.Bcc().Contain(hendrixMail) {
		t.Error("Bcc: do not match. found:", msg.Bcc(), "expected:", hendrixMail)
	}
}

func TestMessageBodyWriter(t *testing.T) {
	data := []byte("hello, world!")
	msg := NewMessage()

	// Writing
	if _, err := fmt.Fprintf(msg.Body, string(data)); err != nil {
		t.Error("failed to write data in Body:", err)
	}

	// Reading
	read, err := ioutil.ReadAll(msg.Body)
	if err != nil {
		t.Error("failed reading data from message body:", err)
	}

	// Comparing
	if !bytes.Equal(data, read) {
		t.Errorf("Body writer: found: \"%s\", expected: \"%s\"", read, data)
	}
}

func TestReadMessage(t *testing.T) {
	sender := &Address{Name: "Al Bumin", Address: "a.bumin@example.name"}
	recipient := &Address{Name: "Polly Ester", Address: "p.ester@example.com"}
	cc := []*Address{
		&Address{Name: "Jim North", Address: "j.north@example.com"},
		&Address{Name: "John Doe", Address: "j.doe@example.com"},
	}
	bcc := &Address{Name: "Harry Jones", Address: "h.jones@example.com"}

	text := bytes.NewReader([]byte("Simple mail message with an attachment."))
	data, _ := ioutil.ReadFile("tests/gopherbw.png")
	attachment := bytes.NewReader(data)

	sent := NewMessage()
	sent.SetFrom(sender)
	sent.SetMessageID("12345-message-id")
	sent.To().Add(recipient)
	sent.Cc().Add(cc[0])
	sent.Cc().Add(cc[1])
	sent.Bcc().Add(bcc)
	sent.SetSubject("Gopher image envoyée par email")
	mixed := NewMultipart("multipart/mixed", sent)
	mixed.AddText("text/plain", text)
	mixed.AddAttachment(Attachment, "Gopher.png", "", attachment)
	mixed.Close()

	read, err := ReadMessage(bytes.NewBuffer(sent.Bytes()))
	if err != nil {
		t.Error("reading composed message failed:", err)
	}

	if read.From().String() != sent.From().String() && read.From().String() == sender.String() {
		t.Error("From: do not match.", "expected:", sent.From(), ", found:", read.From())
	}

	if !read.To().Contain(recipient) {
		t.Error("To: do not match.", "expected:", sent.To(), ", found:", read.To())
	}

	if !read.Cc().Contain(cc[0]) || !read.Cc().Contain(cc[1]) {
		t.Error("To: do not match.", "expected:", sent.Cc(), ", found:", read.Cc())
	}

	if !read.Bcc().Contain(bcc) {
		t.Error("To: do not match.", "expected:", sent.Bcc(), ", found:", read.Bcc())
	}

	if read.Subject() != sent.Subject() {
		t.Error("Subject: do not match.", "expected:", sent.Subject(), ", found:", read.Subject())
	}

	if read.ContentType() != sent.ContentType() {
		t.Error("ContentType: do not match.", "expected:", sent.ContentType(), ", found:", read.ContentType())
	}

	if read.MessageID() != sent.MessageID() {
		t.Error("MessageID: do not match.", "expected:", sent.MessageID(), ", found:", read.MessageID())
	}
}

// Example of a message with plain text, HTML and an attachment.
func TestAddAttachment(t *testing.T) {
	sender := &Address{Name: "Al Bumin", Address: "a.bumin@example.name"}
	recipient := &Address{Name: "Polly Ester", Address: "p.ester@example.com"}

	text := bytes.NewReader([]byte(`Package mail implements composing and parsing of mail messages.

Il est utile pour créer des messages électroniques qui peuvent être envoyés via un serveur SMTP.

إذ الجديدة، الإحتلال لها. تمهيد الستار إتفاقية أن قام. وتنصيب المؤلّفة من الى, هو ضرب لإعادة بعتادهم والمعدات, أم وهزيمة النازية فعل. حين تم قائمة للإمبراطورية, الشهيرة المعارك التحالف تلك لم, مع أضف عليها لإعلان. عرض واستمرت ايطاليا، بالولايات و. لم الامم ألمانيا للأسطول شبح.

This package was designed with the idea of eventually replacing the one in the standard package without breaking any existing code. It is offered in an independant package so that it can be tested in the wild before it's submitted as a contribution.`))
	data, _ := ioutil.ReadFile("tests/gopherbw.png")
	attachment := bytes.NewReader(data)

	msg := NewMessage()
	msg.SetFrom(sender)
	msg.To().Add(recipient)
	msg.SetSubject("Gopher image envoyée par email")
	mixed := NewMultipart("multipart/mixed", msg)
	mixed.AddText("text/plain", text)
	mixed.AddAttachment(Attachment, "Gopher.png", "", attachment)
	mixed.Close()

	ioutil.WriteFile("tests/gopher.eml", msg.Bytes(), os.ModePerm)
}
