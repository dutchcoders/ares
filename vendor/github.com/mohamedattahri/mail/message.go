// Use of this source code is governed by a BSD-style licence.
// Copyright 2011 The Go Authors. All rights reserved.
// Author: Mohamed Attahri <mohamed@attahri.com>

// Package mail implements composing and parsing of mail messages.
//
// Creating a message with multiple parts and attachments ends up being a
// surprisingly painful task in Go. That's because the mail package in the
// standard library only offers tools to parse mail messages and addresses.
//
// This package can replace the net/mail package of the standard library without
// breaking your existing code.
//
// Features
//
// - multipart support
//
// - attachments
//
// - quoted-printable encoding of body text
//
// - quoted-printable decoding of headers
//
// - getters and setters common headers
//
// Known issues
//
// - Quoted-printable encoding does not respect the 76 characters per line
// limitation imposed by RFC 2045 (https://github.com/golang/go/issues/4943).
//
// Installation
//
// Alex Cesaro's quotedprintable package (https://godoc.org/gopkg.in/alexcesaro/quotedprintable.v1)
// is the only external dependency. It's likely to be included in Go 1.5 in a new
// mime/quotedprintable package (https://codereview.appspot.com/132680044).
// 	go get godoc.org/gopkg.in/alexcesaro/quotedprintable.v1
// 	go get github.com/mohamedattahri/mail
package mail

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	qp "gopkg.in/alexcesaro/quotedprintable.v3"
)

var debug = debugT(false)

type debugT bool

func (d debugT) Printf(format string, args ...interface{}) {
	if d {
		log.Printf(format, args...)
	}
}

const (
	crlf            = "\r\n"
	boundaryLength  = 30
	messageIDLength = 12
	maxLineLen      = 76 // RFC 2045
)

// AttachmentType indicates to the mail user agent how an attachment should be
// treated.
type AttachmentType string

const (
	// Attachment indicates that the attachment should be offered as an optional
	// download.
	Attachment AttachmentType = "attachment"
	// Inline indicates that the attachment should be rendered in the body of
	// the message.
	Inline AttachmentType = "inline"
)

// A Message represents a mail message.
type Message struct {
	Header Header
	Body   io.ReadWriter
	root   *Multipart
}

func (m *Message) mimeVersion() string {
	return m.GetHeader("Mime-Version")
}

// GetHeader returns the undecoded value of header if found. To access the
// raw (potentially encoded) value of header, use the Message.Header.
func (m *Message) GetHeader(header string) string {
	encoded := textproto.MIMEHeader(m.Header).Get(header)
	if encoded == "" {
		return ""
	}
	dec := new(qp.WordDecoder)
	decoded, err := dec.DecodeHeader(encoded)
	if err != nil {
		return ""
	}

	return decoded
}

// SetHeader adds header to the list of headers and sets it to quoted-printable
// encoded value.
func (m *Message) SetHeader(header, value string) {
	textproto.MIMEHeader(m.Header).Set(header, value)
}

// Subject line of the message.
func (m *Message) Subject() string {
	return m.GetHeader("Subject")
}

// SetSubject sets the subject line of the message.
func (m *Message) SetSubject(subject string) {
	m.SetHeader("Subject", subject)
}

// From returns the address of the author.
func (m *Message) From() *Address {
	address, err := ParseAddress(m.GetHeader("From"))
	if err != nil {
		return nil
	}
	return address
}

// SetFrom sets the address of the message's author.
func (m *Message) SetFrom(address *Address) {
	m.SetHeader("From", address.String())
}

// Date returns the time and date when the message was written.
func (m *Message) Date() time.Time {
	date, _ := m.Header.Date()
	return date
}

// ContentType returns the MIME type of the message.
func (m *Message) ContentType() string {
	return m.GetHeader("Content-Type")
}

// mediaType returns the media MIME type of the message, minus the params.
func (m *Message) mediaType() string {
	mediaType, _, _ := mime.ParseMediaType(m.GetHeader("Content-Type"))
	return mediaType
}

// SetContentType returns the MIME type of the message.
func (m *Message) SetContentType(mediaType string) {
	_, params, _ := mime.ParseMediaType(m.GetHeader("Content-Type"))
	if m.root != nil {
		params["boundary"] = m.root.Boundary()
	}

	m.SetHeader("Content-Type", mime.FormatMediaType(mediaType, params))
}

// contentLength returns the length of the message.
func (m *Message) contentLength() int64 {
	value := m.GetHeader("Content-Length")
	if parsed, err := strconv.ParseInt(value, 10, 64); err != nil {
		return parsed
	}
	return 0
}

// To gives access to the list of recipients of the message.
func (m *Message) To() *AddressList {
	if _, exists := m.Header["To"]; !exists {
		m.Header["To"] = []string{""}
	}
	return &AddressList{raw: &m.Header["To"][0]}
}

// Cc gives access to the list of recipients Cced in the message.
func (m *Message) Cc() *AddressList {
	if _, exists := m.Header["Cc"]; !exists {
		m.Header["Cc"] = []string{""}
	}
	return &AddressList{raw: &m.Header["Cc"][0]}
}

// Bcc gives access to the list of recipients Bcced in the message.
func (m *Message) Bcc() *AddressList {
	if _, exists := m.Header["Bcc"]; !exists {
		m.Header["Bcc"] = []string{""}
	}
	return &AddressList{raw: &m.Header["Bcc"][0]}
}

// ReplyTo returns the address that should be used to reply to the message.
func (m *Message) ReplyTo() string {
	return m.GetHeader("Reply-To")
}

// SetReplyTo sets the address that should be used to reply to the message.
func (m *Message) SetReplyTo(address string) {
	m.SetHeader("Reply-To", address)
}

// Sender returns the address of the actual sender acting on behalf of the
// author listed in From.
func (m *Message) Sender() string {
	return m.GetHeader("Sender")
}

// SetSender sets the address of the actual sender acting on behalf of the
// author listed in From.
func (m *Message) SetSender(address string) {
	m.SetHeader("Sender", address)
}

// MessageID returns the unique identifier of this message.
func (m *Message) MessageID() string {
	return m.GetHeader("Message-ID")
}

// SetMessageID sets the unique identifier of this message.
func (m *Message) SetMessageID(id string) {
	m.SetHeader("Message-ID", id)
}

// InReplyTo returns the Message-Id of the message that this message is a reply
// to.
func (m *Message) InReplyTo() string {
	return m.GetHeader("In-Reply-To")
}

// SetInReplyTo sets the Message-Id of the message that this message is a reply
// to.
func (m *Message) SetInReplyTo(id string) {
	m.SetHeader("In-Reply-To", id)
}

// boudary string found in the Content-Type header.
func (m *Message) boundary() string {
	_, params, _ := mime.ParseMediaType(m.ContentType())
	if value, exists := params["boundary"]; exists {
		return value
	}
	return ""
}

// Bytes assembles the message in an array of bytes.
func (m *Message) Bytes() []byte {
	output := &bytes.Buffer{}

	//
	// Header
	//
	for key, items := range m.Header {
		for _, item := range items {
			if item != "" {
				fmt.Fprintf(output, "%s: %s%s", key, qp.QEncoding.Encode("utf-8", item), crlf)
			}
		}
	}
	// Subject; because an empty suject would have been skipped in the previous loop.
	if m.Subject() == "" {
		output.WriteString("Subject: ")
		output.WriteString(crlf)
	}
	// Date; automatically inserted if not found in the Header map.
	if _, err := m.Header.Date(); err != nil {
		output.WriteString("Date: " + time.Now().Format(time.RFC1123Z))
		output.WriteString(crlf)
	}
	// Mime-Version; set to 1.0 if missing in the Header map.
	if m.mimeVersion() == "" {
		output.WriteString("Mime-Version: 1.0")
		output.WriteString(crlf)
	}
	// Message-ID; randomly generated value if missing in the Header map.
	if m.MessageID() == "" {
		fmt.Fprintf(output, "Message-ID: <%s.%s>", randomString(messageIDLength), m.From().Address)
		output.WriteString(crlf)
	}
	output.WriteString(crlf)
	//
	// Body
	//
	output.ReadFrom(m.Body)

	return output.Bytes()
}

// String returns a text representation of the message.
func (m *Message) String() string {
	return string(m.Bytes())
}

// NewMessage returns a new empty message.
func NewMessage() *Message {
	return &Message{
		Header: make(Header),
		Body:   &bytes.Buffer{},
	}
}

// ReadMessage reads a message from r.
// The headers are parsed, and the body of the message will be available
// for reading from r.
func ReadMessage(r io.Reader) (msg *Message, err error) {
	tp := textproto.NewReader(bufio.NewReader(r))

	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	return &Message{
		Header: Header(hdr),
		Body:   bufio.NewReadWriter(bufio.NewReader(tp.R), nil),
	}, nil
}

// Layouts suitable for passing to time.Parse.
// These are tried in order.
var dateLayouts []string

func init() {
	// Generate layouts based on RFC 5322, section 3.3.

	dows := [...]string{"", "Mon, "}   // day-of-week
	days := [...]string{"2", "02"}     // day = 1*2DIGIT
	years := [...]string{"2006", "06"} // year = 4*DIGIT / 2*DIGIT
	seconds := [...]string{":05", ""}  // second
	// "-0700 (MST)" is not in RFC 5322, but is common.
	zones := [...]string{"-0700", "MST", "-0700 (MST)"} // zone = (("+" / "-") 4DIGIT) / "GMT" / ...

	for _, dow := range dows {
		for _, day := range days {
			for _, year := range years {
				for _, second := range seconds {
					for _, zone := range zones {
						s := dow + day + " Jan " + year + " 15:04" + second + " " + zone
						dateLayouts = append(dateLayouts, s)
					}
				}
			}
		}
	}
}

func parseDate(date string) (time.Time, error) {
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, date)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("mail: header could not be parsed")
}

// A Header represents the key-value pairs in a mail message header.
type Header map[string][]string

// Get gets the first value associated with the given key.
// If there are no values associated with the key, Get returns "".
func (h Header) Get(key string) string {
	return textproto.MIMEHeader(h).Get(key)
}

var ErrHeaderNotPresent = errors.New("mail: header not in message")

// Date parses the Date header field.
func (h Header) Date() (time.Time, error) {
	hdr := h.Get("Date")
	if hdr == "" {
		return time.Time{}, ErrHeaderNotPresent
	}
	return parseDate(hdr)
}

// AddressList parses the named header field as a list of addresses.
func (h Header) AddressList(key string) ([]*Address, error) {
	hdr := h.Get(key)
	if hdr == "" {
		return nil, ErrHeaderNotPresent
	}
	return ParseAddressList(hdr)
}

// Address represents a single mail address.
// An address such as "Barry Gibbs <bg@example.com>" is represented
// as Address{Name: "Barry Gibbs", Address: "bg@example.com"}.
type Address struct {
	Name    string // Proper name; may be empty.
	Address string // user@domain
}

// ParseAddress parses a single RFC 5322 address, e.g. "Barry Gibbs <bg@example.com>"
func ParseAddress(address string) (*Address, error) {
	return newAddrParser(address).parseAddress()
}

// ParseAddressList parses the given string as a list of addresses.
func ParseAddressList(list string) ([]*Address, error) {
	return newAddrParser(list).parseAddressList()
}

// String formats the address as a valid RFC 5322 address.
// If the address's name contains non-ASCII characters
// the name will be rendered according to RFC 2047.
func (a *Address) String() string {
	s := "<" + a.Address + ">"
	if a.Name == "" {
		return s
	}
	// If every character is printable ASCII, quoting is simple.
	allPrintable := true
	for i := 0; i < len(a.Name); i++ {
		// isWSP here should actually be isFWS,
		// but we don't support folding yet.
		if !isVchar(a.Name[i]) && !isWSP(a.Name[i]) {
			allPrintable = false
			break
		}
	}
	if allPrintable {
		b := bytes.NewBufferString(`"`)
		for i := 0; i < len(a.Name); i++ {
			if !isQtext(a.Name[i]) && !isWSP(a.Name[i]) {
				b.WriteByte('\\')
			}
			b.WriteByte(a.Name[i])
		}
		b.WriteString(`" `)
		b.WriteString(s)
		return b.String()
	}

	// UTF-8 "Q" encoding
	b := bytes.NewBufferString("=?utf-8?q?")
	for i := 0; i < len(a.Name); i++ {
		switch c := a.Name[i]; {
		case c == ' ':
			b.WriteByte('_')
		case isVchar(c) && c != '=' && c != '?' && c != '_':
			b.WriteByte(c)
		default:
			fmt.Fprintf(b, "=%02X", c)
		}
	}
	b.WriteString("?= ")
	b.WriteString(s)
	return b.String()
}

type addrParser []byte

func newAddrParser(s string) *addrParser {
	p := addrParser(s)
	return &p
}

func (p *addrParser) parseAddressList() ([]*Address, error) {
	var list []*Address
	for {
		p.skipSpace()
		addr, err := p.parseAddress()
		if err != nil {
			return nil, err
		}
		list = append(list, addr)

		p.skipSpace()
		if p.empty() {
			break
		}
		if !p.consume(',') {
			return nil, errors.New("mail: expected comma")
		}
	}
	return list, nil
}

// parseAddress parses a single RFC 5322 address at the start of p.
func (p *addrParser) parseAddress() (addr *Address, err error) {
	debug.Printf("parseAddress: %q", *p)
	p.skipSpace()
	if p.empty() {
		return nil, errors.New("mail: no address")
	}

	// address = name-addr / addr-spec
	// TODO(dsymonds): Support parsing group address.

	// addr-spec has a more restricted grammar than name-addr,
	// so try parsing it first, and fallback to name-addr.
	// TODO(dsymonds): Is this really correct?
	spec, err := p.consumeAddrSpec()
	if err == nil {
		return &Address{
			Address: spec,
		}, err
	}
	debug.Printf("parseAddress: not an addr-spec: %v", err)
	debug.Printf("parseAddress: state is now %q", *p)

	// display-name
	var displayName string
	if p.peek() != '<' {
		displayName, err = p.consumePhrase()
		if err != nil {
			return nil, err
		}
	}
	debug.Printf("parseAddress: displayName=%q", displayName)

	// angle-addr = "<" addr-spec ">"
	p.skipSpace()
	if !p.consume('<') {
		return nil, errors.New("mail: no angle-addr")
	}
	spec, err = p.consumeAddrSpec()
	if err != nil {
		return nil, err
	}
	if !p.consume('>') {
		return nil, errors.New("mail: unclosed angle-addr")
	}
	debug.Printf("parseAddress: spec=%q", spec)

	return &Address{
		Name:    displayName,
		Address: spec,
	}, nil
}

// consumeAddrSpec parses a single RFC 5322 addr-spec at the start of p.
func (p *addrParser) consumeAddrSpec() (spec string, err error) {
	debug.Printf("consumeAddrSpec: %q", *p)

	orig := *p
	defer func() {
		if err != nil {
			*p = orig
		}
	}()

	// local-part = dot-atom / quoted-string
	var localPart string
	p.skipSpace()
	if p.empty() {
		return "", errors.New("mail: no addr-spec")
	}
	if p.peek() == '"' {
		// quoted-string
		debug.Printf("consumeAddrSpec: parsing quoted-string")
		localPart, err = p.consumeQuotedString()
	} else {
		// dot-atom
		debug.Printf("consumeAddrSpec: parsing dot-atom")
		localPart, err = p.consumeAtom(true)
	}
	if err != nil {
		debug.Printf("consumeAddrSpec: failed: %v", err)
		return "", err
	}

	if !p.consume('@') {
		return "", errors.New("mail: missing @ in addr-spec")
	}

	// domain = dot-atom / domain-literal
	var domain string
	p.skipSpace()
	if p.empty() {
		return "", errors.New("mail: no domain in addr-spec")
	}
	// TODO(dsymonds): Handle domain-literal
	domain, err = p.consumeAtom(true)
	if err != nil {
		return "", err
	}

	return localPart + "@" + domain, nil
}

// consumePhrase parses the RFC 5322 phrase at the start of p.
func (p *addrParser) consumePhrase() (phrase string, err error) {
	debug.Printf("consumePhrase: [%s]", *p)
	// phrase = 1*word
	var words []string
	for {
		// word = atom / quoted-string
		var word string
		p.skipSpace()
		if p.empty() {
			return "", errors.New("mail: missing phrase")
		}
		if p.peek() == '"' {
			// quoted-string
			word, err = p.consumeQuotedString()
		} else {
			// atom
			// We actually parse dot-atom here to be more permissive
			// than what RFC 5322 specifies.
			word, err = p.consumeAtom(true)
		}

		// RFC 2047 encoded-word starts with =?, ends with ?=, and has two other ?s.
		if err == nil && strings.HasPrefix(word, "=?") && strings.HasSuffix(word, "?=") && strings.Count(word, "?") == 4 {
			word, err = decodeRFC2047Word(word)
		}

		if err != nil {
			break
		}
		debug.Printf("consumePhrase: consumed %q", word)
		words = append(words, word)
	}
	// Ignore any error if we got at least one word.
	if err != nil && len(words) == 0 {
		debug.Printf("consumePhrase: hit err: %v", err)
		return "", fmt.Errorf("mail: missing word in phrase: %v", err)
	}
	phrase = strings.Join(words, " ")
	return phrase, nil
}

// consumeQuotedString parses the quoted string at the start of p.
func (p *addrParser) consumeQuotedString() (qs string, err error) {
	// Assume first byte is '"'.
	i := 1
	qsb := make([]byte, 0, 10)
Loop:
	for {
		if i >= p.len() {
			return "", errors.New("mail: unclosed quoted-string")
		}
		switch c := (*p)[i]; {
		case c == '"':
			break Loop
		case c == '\\':
			if i+1 == p.len() {
				return "", errors.New("mail: unclosed quoted-string")
			}
			qsb = append(qsb, (*p)[i+1])
			i += 2
		case isQtext(c), c == ' ' || c == '\t':
			// qtext (printable US-ASCII excluding " and \), or
			// FWS (almost; we're ignoring CRLF)
			qsb = append(qsb, c)
			i++
		default:
			return "", fmt.Errorf("mail: bad character in quoted-string: %q", c)
		}
	}
	*p = (*p)[i+1:]
	return string(qsb), nil
}

// consumeAtom parses an RFC 5322 atom at the start of p.
// If dot is true, consumeAtom parses an RFC 5322 dot-atom instead.
func (p *addrParser) consumeAtom(dot bool) (atom string, err error) {
	if !isAtext(p.peek(), false) {
		return "", errors.New("mail: invalid string")
	}
	i := 1
	for ; i < p.len() && isAtext((*p)[i], dot); i++ {
	}
	atom, *p = string((*p)[:i]), (*p)[i:]
	return atom, nil
}

func (p *addrParser) consume(c byte) bool {
	if p.empty() || p.peek() != c {
		return false
	}
	*p = (*p)[1:]
	return true
}

// skipSpace skips the leading space and tab characters.
func (p *addrParser) skipSpace() {
	*p = bytes.TrimLeft(*p, " \t")
}

func (p *addrParser) peek() byte {
	return (*p)[0]
}

func (p *addrParser) empty() bool {
	return p.len() == 0
}

func (p *addrParser) len() int {
	return len(*p)
}

func decodeRFC2047Word(s string) (string, error) {
	fields := strings.Split(s, "?")
	if len(fields) != 5 || fields[0] != "=" || fields[4] != "=" {
		return "", errors.New("address not RFC 2047 encoded")
	}
	charset, enc := strings.ToLower(fields[1]), strings.ToLower(fields[2])
	if charset != "us-ascii" && charset != "iso-8859-1" && charset != "utf-8" {
		return "", fmt.Errorf("charset not supported: %q", charset)
	}

	in := bytes.NewBufferString(fields[3])
	var r io.Reader
	switch enc {
	case "b":
		r = base64.NewDecoder(base64.StdEncoding, in)
	case "q":
		r = qDecoder{r: in}
	default:
		return "", fmt.Errorf("RFC 2047 encoding not supported: %q", enc)
	}

	dec, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}

	switch charset {
	case "us-ascii":
		b := new(bytes.Buffer)
		for _, c := range dec {
			if c >= 0x80 {
				b.WriteRune(unicode.ReplacementChar)
			} else {
				b.WriteRune(rune(c))
			}
		}
		return b.String(), nil
	case "iso-8859-1":
		b := new(bytes.Buffer)
		for _, c := range dec {
			b.WriteRune(rune(c))
		}
		return b.String(), nil
	case "utf-8":
		return string(dec), nil
	}
	panic("unreachable")
}

type qDecoder struct {
	r       io.Reader
	scratch [2]byte
}

func (qd qDecoder) Read(p []byte) (n int, err error) {
	// This method writes at most one byte into p.
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := qd.r.Read(qd.scratch[:1]); err != nil {
		return 0, err
	}
	switch c := qd.scratch[0]; {
	case c == '=':
		if _, err := io.ReadFull(qd.r, qd.scratch[:2]); err != nil {
			return 0, err
		}
		x, err := strconv.ParseInt(string(qd.scratch[:2]), 16, 64)
		if err != nil {
			return 0, fmt.Errorf("mail: invalid RFC 2047 encoding: %q", qd.scratch[:2])
		}
		p[0] = byte(x)
	case c == '_':
		p[0] = ' '
	default:
		p[0] = c
	}
	return 1, nil
}

var atextChars = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	"abcdefghijklmnopqrstuvwxyz" +
	"0123456789" +
	"!#$%&'*+-/=?^_`{|}~")

// isAtext returns true if c is an RFC 5322 atext character.
// If dot is true, period is included.
func isAtext(c byte, dot bool) bool {
	if dot && c == '.' {
		return true
	}
	return bytes.IndexByte(atextChars, c) >= 0
}

// isQtext returns true if c is an RFC 5322 qtext character.
func isQtext(c byte) bool {
	// Printable US-ASCII, excluding backslash or quote.
	if c == '\\' || c == '"' {
		return false
	}
	return '!' <= c && c <= '~'
}

// isVchar returns true if c is an RFC 5322 VCHAR character.
func isVchar(c byte) bool {
	// Visible (printing) characters.
	return '!' <= c && c <= '~'
}

// isWSP returns true if c is a WSP (white space).
// WSP is a space or horizontal tab (RFC5234 Appendix B).
func isWSP(c byte) bool {
	return c == ' ' || c == '\t'
}

// adapted from mime.randomBoundary
func randomString(length int) string {
	buf := make([]byte, length)
	_, err := io.ReadFull(rand.Reader, buf[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buf[:])
}

// AddressList implements methods to manipulate a comma-separated list of mail
// addresses.
type AddressList struct {
	raw *string
}

// Add address to the list.
func (a *AddressList) Add(address *Address) {
	if *a.raw != "" {
		*a.raw += ","
	}
	*a.raw += address.String()
}

// Remove address from the list.
func (a *AddressList) Remove(address *Address) {
	list, err := ParseAddressList(*a.raw)
	if err != nil {
		return
	}

	var addresses []string
	for _, item := range list {
		if current := item.String(); current != address.String() {
			addresses = append(addresses, current)
		}
	}
	*a.raw = strings.Join(addresses, ",")
}

// Contain returns a value indicating whether address is in the list.
func (a *AddressList) Contain(address *Address) bool {
	return strings.Contains(*a.raw, address.String())
}

// Addresses contained in the list as an array or an error if the underlying
// string is malformed.
func (a *AddressList) Addresses() ([]*Address, error) {
	return ParseAddressList(*a.raw)
}

// String returns the addresses in the list in a comma-separated string.
func (a *AddressList) String() string {
	return *a.raw
}

// Multipart repreents a multipart message body. It can other nest multiparts,
// texts, and attachments.
type Multipart struct {
	writer    *multipart.Writer
	mediaType string
	isClosed  bool
	header    textproto.MIMEHeader
}

var ErrPartClosed = errors.New("mail: part has been closed")

// AddMultipart creates a nested part with mediaType and a randomly generated
// boundary. The returned nested part can then be used to add a text or
// an attachment.
//
// Example:
// 	alt, _ := part.AddMultipart("multipart/mixed")
// 	alt.AddText("text/plain", text)
// 	alt.AddAttachment("gopher.png", "", image)
// 	alt.Close()
func (p *Multipart) AddMultipart(mediaType string) (nested *Multipart, err error) {
	if !strings.HasPrefix(mediaType, "multipart") {
		return nil, errors.New("mail: mediaType must start with the word \"multipart\" as in multipart/mixed or multipart/alter")
	}

	if p.isClosed {
		return nil, ErrPartClosed
	}

	boundary := randomString(boundaryLength)

	// Mutlipart management
	var mimeType string
	if strings.HasPrefix(mediaType, "multipart") {
		mimeType = mime.FormatMediaType(
			mediaType,
			map[string]string{"boundary": boundary},
		)
	} else {
		mimeType = mediaType
	}

	// Header
	p.header = make(textproto.MIMEHeader)
	p.header["Content-Type"] = []string{mimeType}

	w, err := p.writer.CreatePart(p.header)
	if err != nil {
		return nil, err
	}

	nested = createPart(w, p.header, mediaType, boundary)
	return nested, nil
}

// AddText applies quoted-printable encoding to the content of r before writing
// the encoded result in a new sub-part with media MIME type set to mediaType.
//
// Specifying the charset in the mediaType string is recommended
// ("plain/text; charset=utf-8").
func (p *Multipart) AddText(mediaType string, r io.Reader) error {
	if p.isClosed {
		return ErrPartClosed
	}

	p.header = textproto.MIMEHeader(map[string][]string{
		"Content-Type":              {mediaType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})

	w, err := p.writer.CreatePart(p.header)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(r)
	encoder := qp.NewWriter(w)
	buffer := make([]byte, maxLineLen)
	for {
		read, err := reader.Read(buffer[:])
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		encoder.Write(buffer[:read])
	}
	fmt.Fprintf(w, crlf)
	fmt.Fprintf(w, crlf)
	return nil
}

// AddAttachment encodes the content of r in base64 and writes it as an
// attachment of type attachType in this part.
//
// filename is the file name that will be suggested by the mail user agent to a
// user who would like to download the attachment. It's also the value to which
// the Content-ID header will be set. A name with an extension such as
// "report.docx" or "photo.jpg" is recommended. RFC 5987 is not supported, so
// the charset is restricted to ASCII characters.
//
// mediaType indicates the content type of the attachment. If an empty string is
// passed, mime.TypeByExtension will first be called to deduce a value from the
// extension of filemame before defaulting to "application/octet-stream".
//
// In the following example, the media MIME type will be set to "image/png"
// based on the ".png" extension of the filename "gopher.png":
// 	part.AddAttachment(Inline, "gopher.png", "", image)
func (p *Multipart) AddAttachment(attachType AttachmentType, filename, mediaType string, r io.Reader) (err error) {
	if p.isClosed {
		return ErrPartClosed
	}

	// Default Content-Type value
	if mediaType == "" && filename != "" {
		mediaType = mime.TypeByExtension(filepath.Ext(filename))
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	header := textproto.MIMEHeader(map[string][]string{
		"Content-Type":              {mediaType},
		"Content-ID":                {fmt.Sprintf("<%s>", filename)},
		"Content-Location":          {fmt.Sprintf("%s", filename)},
		"Content-Transfer-Encoding": {"base64"},
		"Content-Disposition":       {fmt.Sprintf("%s;\r\n\tfilename=%s;", attachType, filename)},
	})

	w, err := p.writer.CreatePart(header)
	if err != nil {
		return err
	}

	encoder := base64.NewEncoder(base64.StdEncoding, w)
	data := bufio.NewReader(r)

	buffer := make([]byte, int(math.Ceil(maxLineLen/4)*3))
	for {
		read, err := io.ReadAtLeast(data, buffer[:], len(buffer))
		if err != nil {
			if err == io.EOF {
				break
			} else if err != io.ErrUnexpectedEOF {
				return err
			}
		}

		if _, err := encoder.Write(buffer[:read]); err != nil {
			return err
		}

		if read == len(buffer) {
			fmt.Fprintf(w, crlf)
		}
	}
	encoder.Close()
	fmt.Fprintf(w, crlf)

	return nil
}

// Header map of the part.
func (p *Multipart) Header() textproto.MIMEHeader {
	return p.header
}

// Boundary separating the children of this part.
func (p *Multipart) Boundary() string {
	return p.writer.Boundary()
}

// MediaType returns the media MIME type of this part.
func (p *Multipart) MediaType() string {
	return p.mediaType
}

// Close adds a closing boundary to the part.
//
// Calling AddText, AddAttachment or AddMultipart on a closed part will return
// ErrPartClosed.
func (p *Multipart) Close() error {
	if p.isClosed {
		return ErrPartClosed
	}
	p.isClosed = true
	return p.writer.Close()
}

// Closed returns true if the part has been closed.
func (p *Multipart) Closed() bool {
	return p.isClosed
}

func createPart(w io.Writer, header textproto.MIMEHeader, mediaType string, boundary string) *Multipart {
	m := &Multipart{
		writer:    multipart.NewWriter(w),
		header:    header,
		mediaType: mediaType,
	}
	m.writer.SetBoundary(boundary)
	return m
}

// NewMultipart modifies msg to become a multipart message and returns the root
// part inside which other parts, texts and attachments can be nested.
//
// Example:
// 	multipart := NewMultipart("multipart/alternative", msg)
// 	multipart.AddPart("text/plain", text)
// 	multipart.AddPart("text/html", html)
// 	multipart.Close()
func NewMultipart(mediaType string, msg *Message) (root *Multipart) {
	boundary := randomString(boundaryLength)
	msg.root = createPart(msg.Body, make(textproto.MIMEHeader), mediaType, boundary)
	_, params, _ := mime.ParseMediaType(msg.GetHeader("Content-Type"))
	if params == nil {
		params = make(map[string]string)
	}
	params["boundary"] = boundary
	msg.SetHeader("Content-Type", mime.FormatMediaType(mediaType, params))
	return msg.root
}
