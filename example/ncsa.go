package main

// Example code for using lexrec to parse an NCSA common log format
// (see http://en.wikipedia.org/wiki/Common_Log_Format):
//
// 127.0.0.1 user-identifier frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
//

import (
	"github.com/jimrobinson/lexrec"
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
var acceptNotSpace = lexrec.ExceptRun(" ", true)

// accept a single space
var acceptSpace = lexrec.Accept(" ", true)

// accept a single open brace ('[')
var acceptOpenBrace = lexrec.Accept("[", true)

// accept a single slash ('/')
var acceptSlash = lexrec.Accept("/", true)

// accept a single colon (':')
var acceptColon = lexrec.Accept(":", true)

// accept a single close brace (']')
var acceptCloseBrace = lexrec.Accept("]", true)

// accept a single double-quote ('"')
var acceptQuote = lexrec.Accept(`"`, true)

// accept a run of non-double-quote characters
var acceptNotQuote = lexrec.ExceptRun(`"`, true)

// accept a single newline ('\n')
var acceptNewline = lexrec.Accept("\n", true)

// accept a run of non-newline characters
var acceptNotNewline = lexrec.ExceptRun("\n", true)

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
		{ItemResponseStatus, digitsOrMinus, true},   // response status code (a number, e.g., 200,  or '-')
		{ItemIgnore, acceptSpace, false},            // ' '
		{ItemResponseBytes, digitsOrMinus, true},    // response bytes (a number, e.g., 10, or '-')
		{ItemIgnore, acceptNewline, false},          // '\n'
	}}

const sign = "+-"
const digits = "0123456789"

// digitsOrMinus consumes either a sequence of digits or the single
// char '-' followed by a space.
func digitsOrMinus(l *lexrec.Lexer, t lexrec.ItemType, emit bool) (success bool) {
	if l.AcceptRun(digits) || (l.Accept("-") && l.Peek() == ' ') {
		if emit {
			l.Emit(t)
		} else {
			l.Skip()
		}
		return true
	}
	l.Errorf("expected a '-' or a sequence of %q, got %q", digits, l.Peek())
	return false
}

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
