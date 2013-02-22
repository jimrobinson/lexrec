package main

// Example code for using lexrec to parse an NCSA common log format
// (see http://en.wikipedia.org/wiki/Common_Log_Format):
//
// 127.0.0.1 user-identifier frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
//

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jimrobinson/lexrec"
	"io"
	"log"
	"os"
)

const (
	ItemIgnore          lexrec.ItemType = lexrec.ItemEOF + 1 + iota
	ItemRemoteHost                      // remote client
	ItemRemoteLogname                   // remote user identd
	ItemRemoteUser                      // remote user login
	ItemRequestDay                      // numeric day of month (01 - 31)
	ItemRequestMonth                    // month name
	ItemRequestYear                     // numeric year
	ItemRequestHour                     // numeric  hour (00 - 23)
	ItemRequestMinute                   // numeric minute (00 - 59)
	ItemRequestSecond                   // numeric second (00 - 59)
	ItemRequestTz                       // numeric timezone [+-]HH:MM or [+-]HHMM
	ItemRequestMethod                   // HTTP method
	ItemRequestPath                     // HTTP path and parameters
	ItemRequestProtocol                 // HTTP protocol
	ItemResponseStatus                  // response status code
	ItemResponseBytes                   // response bytes
)

// accept a run of non-space characters
var acceptNotSpace = lexrec.ExceptRun(" ")

// accept a single space
var acceptSpace = lexrec.Accept(" ")

// accept a single open brace ('[')
var acceptOpenBrace = lexrec.Accept("[")

// accept a single slash ('/')
var acceptSlash = lexrec.Accept("/")

// accept a single colon (':')
var acceptColon = lexrec.Accept(":")

// accept a single close brace (']')
var acceptCloseBrace = lexrec.Accept("]")

// accept a single double-quote ('"')
var acceptQuote = lexrec.Accept(`"`)

// accept a run of non-double-quote characters
var acceptNotQuote = lexrec.ExceptRun(`"`)

// accept a single newline ('\n')
var acceptNewline = lexrec.Accept("\n")

// accept a run of non-newline characters
var acceptNotNewline = lexrec.ExceptRun("\n")

// ncsaRecord defines the NCSA Common Log Format
var ncsaRecord = lexrec.Record{
	Buflen:  8192,
	ErrorFn: lexrec.SkipPast("\n"),
	States: []lexrec.Binding{
		{ItemRemoteHost, acceptNotSpace, true},      // remote client address or hostname
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemRemoteLogname, acceptNotSpace, true},   // remote user identd
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemRemoteUser, acceptNotSpace, true},      // remote user login
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemIgnore, acceptOpenBrace, false},        // '['
		{ItemRequestDay, lexrec.Digits, true},       // 2 digit day of month
		{ItemIgnore, acceptSlash, false},            // '/'
		{ItemRequestMonth, lexrec.Letters, true},    // 3-character month
		{ItemIgnore, acceptSlash, false},            // '/'
		{ItemRequestYear, lexrec.Digits, true},      // year
		{ItemIgnore, acceptColon, false},            // ':'
		{ItemRequestHour, lexrec.Digits, true},      // 2-digit hour (00 - 23)
		{ItemIgnore, acceptColon, false},            // ':'
		{ItemRequestMinute, lexrec.Digits, true},    // 2-digit minute (00 - 59)
		{ItemIgnore, acceptColon, false},            // ':'
		{ItemRequestSecond, lexrec.Digits, true},    // 2-digit second (00 - 59)
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemRequestTz, numericTz, true},            // -0800
		{ItemIgnore, acceptCloseBrace, false},       // ]
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemIgnore, acceptQuote, false},            // '"'
		{ItemRequestMethod, acceptNotSpace, true},   // HTTP method
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemRequestPath, acceptNotSpace, true},     // HTTP path and parameters
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemRequestProtocol, acceptNotQuote, true}, // HTTP protocol
		{ItemIgnore, acceptQuote, false},            // "
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemResponseStatus, acceptNotSpace, true},  // response status code (a number, e.g., 200,  or '-')
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemResponseBytes, acceptNotNewline, true}, // response bytes (a number, e.g., 10, or '-')
		{ItemIgnore, acceptNewline, false},          // '\n'
	}}

const sign = "+-"
const digits = "0123456789"

// numericTz consumes a timezone field in the format [+-]HHMM or [+-]HH:MM
func numericTz(l *lexrec.Lexer, t lexrec.ItemType, emit bool) (success bool) {

	// leading + or -
	if !l.Accept(sign) {
		l.Errorf("expected plus or minus sign, got %q", l.Peek())
		l.Backup()
		return false
	}

	// Accept two 0-9 digits
	if !l.Accept(digits) || !l.Accept(digits) {
		l.Errorf("expected 2-digit hour, got %q", l.Peek())
		l.Backup()
		return false
	}

	// Accept optional ':'
	if ':' == l.Peek() {
		l.Next()
	}

	// Accept two 0-9 digits
	if !l.Accept(digits) || !l.Accept(digits) {
		l.Errorf("expected 2-digit minute, got %q", l.Peek())
		l.Backup()
		return false
	}

	// Note we aren't bothering to check the next character, if
	// it's a digit then we'll fail in the next StateFn.
	if emit {
		l.Emit(t)
	} else {
		l.Skip()
	}
	return true
}

func cat(path string, r io.Reader) {
	br := bufio.NewReader(r)
	l, err := lexrec.NewLexer("example", br, ncsaRecord)
	if err != nil {
		log.Fatal(err)
	}

	buf := new(bytes.Buffer)
	for {
		item := l.NextItem()
		if item.Type == lexrec.ItemEOF {
			break
		} else if item.Type == lexrec.ItemError {
			fmt.Printf("%s at %s:%d\n", item.Value, path, item.Pos)
			buf.Reset()
			continue
		}
		switch item.Type {
		default:
			buf.WriteByte(' ')
		case ItemRemoteHost:
			// start of a new record
		case ItemRequestDay:
			buf.WriteString(" [")
		case ItemRequestMethod:
			buf.WriteString(`] "`)
		case ItemResponseStatus:
			buf.WriteString(`" `)
		case ItemRequestMonth, ItemRequestYear:
			buf.WriteByte('/')
		case ItemRequestHour, ItemRequestMinute, ItemRequestSecond:
			buf.WriteByte(':')
		case lexrec.ItemEOR:
			fmt.Println(buf.String())
			buf.Reset()
		}
		buf.WriteString(item.Value)
	}
}

func main() {
	path := "example.txt"
	fh, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer fh.Close()
	cat(path, fh)
}
