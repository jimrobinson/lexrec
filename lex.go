/*
Package lexrec implements a library for parsing fixed format records.

The caller defines a Record that consists of

 - Buflen, indicating the expected size the average record, in bytes.
   This is used as a hint to manage the size of the read-ahead buffer.
   The buffer will be expanded to at least this size on the first
   read, and it will be increased as needed if a token crosses
   multiple read boundaries.

 - States, a slice of Binding.  Each Binding consists of an
   ItemType, a StateFn, and a boolean indicating whether or not the
   token should be emitted on success.

 - ErrorFn, a function to call if one of the StateFn returns false,
   indicating an error state.  ErrorFn shoould recover the Lexer,
   typically this would be accomplished by skipping the remainder of
   the record.

The Lexer will iterate over States, calling each StateFn in turn. On
success the StateFn will emit the ItemType or not, depending on the
value of the emit boolean.

The caller can iterate over NextItem(), looking for the ItemType
events that interest them.

Once the end of States is reached, an ItemEOR will be emitted.  Once
the end of the input has been reached an ItemEOF will be emitted.

Much of this library was inspired by and derived from by Rob Pike's
template parsing libary (http://golang.org/pkg/text/template/parse/).
Any elegant bits in this library are from his original library.
*/
package lexrec

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// eof indicates end of file for the input
const EOF = -1

// RunFn is a function that can drive a Lexer
type RunFn func(l *Lexer)

// StateFn is a function that can consume characters from the input
// and emit an lexed token item as ItemType t.  If emit is false the
// item will be consumed but not transmitted.
type StateFn func(l *Lexer, t ItemType, emit bool) (success bool)

// ErrorFn is a function run when StateFn returns false (indicating a
// record was not parsed successfully).  Typically errorFn take steps
// to recover the state of the lexer, e.g., skipping to the end of the
// record.
type ErrorFn func(l *Lexer)

// ItemType represents the type of a lexical token
type ItemType int

const (
	ItemError ItemType = iota // error detected
	ItemEOR                   // end of record
	ItemEOF                   // end of file
)

// Item represents a lexed token item
type Item struct {
	Type  ItemType // the type of this item
	Pos   int64    // the starting position, in bytes, of this item
	Value string   //  the value of this item
}

// Binding maps a lexer ItemType to a lexer StateFn. The boolean emit
// controls whether or not the item is communicated to the parser.
type Binding struct {
	ItemType ItemType // the type of this item
	StateFn  StateFn  // the lexer function to call
	Emit     bool     // emit the item type or skip over it
}

// Record represents a log record
type Record struct {
	Buflen  int       // size of initial buffer, this will be grown as necessary
	States  []Binding // lexer states that make up a record
	ErrorFn ErrorFn   // error function to apply if the lexer encounters a malformed record
}

func NewRecord(n int, states []Binding, errorFn ErrorFn) Record {
	return Record{
		Buflen:  n,
		States:  states,
		ErrorFn: errorFn,
	}
}

// lexer holds the state of the scanner
type Lexer struct {
	name    string    // name of the input; used only for error reports
	r       io.Reader // input reader
	rec     Record    // log record definition
	items   chan Item // channel of lexed items
	eof     bool      // end of file reached?
	next    []byte    // buffer of bytes to read from r and append to buf
	buf     []byte    // buffer of bytes to hold a complete token
	rpos    int64     // current position in input
	pos     int       // current position in buf
	start   int       // start position of item in buf
	width   int       // width of most recent rune read from buf
	lastPos int64     // position of most recent item returned by nextItem
}

// NewLexer returns a lexer for rec records from the UTF-8 reader r.
// The name is only used for debugging messages.
func NewLexer(name string, r io.Reader, rec Record) (l *Lexer, err error) {
	if len(rec.States) == 0 {
		err = fmt.Errorf("rec.states must not be empty.")
		return
	}
	if rec.Buflen < 1 {
		err = fmt.Errorf("rec.Buflen must be > 0: %d", rec.Buflen)
		return
	}
	if rec.ErrorFn == nil {
		err = fmt.Errorf("rec.ErrorFn must not be nil")
		return
	}
	l = &Lexer{
		name:  name,
		r:     r,
		rec:   rec,
		items: make(chan Item),
		next:  make([]byte, rec.Buflen),
		eof:   false,
	}
	go l.run()
	return
}

// NewLexerRun returns a lexer for rec records from the UTF-8 reader
// r, and driving the lexer using RunFn instead of iterating over
// rec.States.  The name is only used for debugging messages.
func NewLexerRun(name string, r io.Reader, rec Record, runFn RunFn) (l *Lexer, err error) {
	if rec.Buflen < 1 {
		err = fmt.Errorf("rec.Buflen must be > 0: %d", rec.Buflen)
		return
	}
	if rec.ErrorFn == nil {
		err = fmt.Errorf("rec.ErrorFn must not be nil")
		return
	}
	l = &Lexer{
		name:  name,
		r:     r,
		rec:   rec,
		items: make(chan Item),
		next:  make([]byte, rec.Buflen),
		eof:   false,
	}
	go func(l *Lexer, runFn RunFn) {
		defer close(l.items)
		runFn(l)
	}(l, runFn)

	return
}

// run consumes input, emitting ItemType events until EOF is reached.
func (l *Lexer) run() {
	defer close(l.items)
	eor := len(l.rec.States) - 1
	for {
		for i, state := range l.rec.States {
			if !state.StateFn(l, state.ItemType, state.Emit) {
				l.rec.ErrorFn(l)
				break
			}
			if i == eor || l.eof {
				l.Emit(ItemEOR)
			}
		}
		if l.Peek() == EOF {
			l.Emit(ItemEOF)
			break
		}
	}
}

// NextItem returns the next Item from the input.
func (l *Lexer) NextItem() Item {
	item := <-l.items
	l.lastPos = item.Pos
	return item
}

// LastPos returns the position of the most recent Item read from the input
func (l *Lexer) LastPos() int64 {
	return l.lastPos
}

// Errorf returns an error token
func (l *Lexer) Errorf(format string, args ...interface{}) {
	l.items <- Item{ItemError, l.rpos, fmt.Sprintf(format, args...)}
}

// Next consumes the next rune in the input.
func (l *Lexer) Next() rune {
	// read more of the input if if we've reached the end of the
	// buffer or if we might be on a character boundry.
	if (len(l.buf) - l.pos) < utf8.UTFMax {
		n, err := l.r.Read(l.next)
		if err == nil {
			l.buf = append(l.buf, l.next[0:n]...)
		} else if err != io.EOF {
			l.Errorf("%s: %v", l.name, err)
		}
	}
	if l.pos == len(l.buf) {
		l.eof = true
		return EOF
	}
	r, w := utf8.DecodeRune(l.buf[l.pos:])
	l.width = w
	l.pos += w
	l.rpos += int64(w)
	return r
}

// Peek returns but does not consume the next rune in the input.
func (l *Lexer) Peek() rune {
	r := l.Next()
	l.Backup()
	return r
}

// Size returns the number of bytes in the current run of token characters
func (l *Lexer) Size() int {
	return l.pos - l.start
}

// Bytes returns the bytes in the current run of token characters
func (l *Lexer) Bytes() []byte {
	return l.buf[l.start:l.pos]
}

// Accept consumes the next rune if it in the valid set, returning true on success.
func (l *Lexer) Accept(valid string) bool {
	if strings.IndexRune(valid, l.Next()) >= 0 {
		return true
	}
	l.Backup()
	return false
}

// Except consumes the next rune if it's not in the invalid set, returning true on success.
func (l *Lexer) Except(invalid string) bool {
	if strings.IndexRune(invalid, l.Next()) < 0 {
		return true
	}
	l.Backup()
	return false
}

// AcceptRun consumes a run of runes from the valid set, returning true on success.
func (l *Lexer) AcceptRun(valid string) bool {
	for {
		r := l.Next()
		if r == EOF {
			break
		}
		if strings.IndexRune(valid, r) < 0 {
			break
		}
	}
	l.Backup()
	return l.pos > l.start
}

// ExceptRun consumes a run of runes that are not in the invalid set, returning true on success.
func (l *Lexer) ExceptRun(invalid string) bool {
	for {
		r := l.Next()
		if r == EOF {
			break
		}
		if strings.IndexRune(invalid, r) >= 0 {
			break
		}
	}
	l.Backup()
	return l.pos > l.start
}

// Backup steps back one rune.  Can only be called once per call of Next.
func (l *Lexer) Backup() {
	if !l.eof {
		l.pos -= l.width
		l.rpos -= int64(l.width)
	}
}

// Emit reports the current item to the client
func (l *Lexer) Emit(t ItemType) {
	l.items <- Item{t, l.rpos - int64(l.pos-l.start), string(l.buf[l.start:l.pos])}
	l.Skip()
}

// Skip advances over the current item without reporting it
func (l *Lexer) Skip() {
	// We're at a point where we know we have completely read a
	// token.  If we've read 90% of an l.buf's capacity, shift the
	// unread content to the start of the buffer.  Otherwise just
	// move l.start to the current position.
	n := cap(l.buf)
	r := n - l.pos
	if n/10 >= r {
		l.buf, l.start, l.pos = append(l.buf[0:0], l.buf[l.pos:]...), 0, 0
	} else {
		l.start = l.pos
	}
}

// SkipPast returns an ErrorFn that consumes a sequence of characters
// that are not in the set s, and one or more instances of the
// characters in the set s.
func SkipPast(s string) ErrorFn {
	return func(l *Lexer) {
		if l.ExceptRun(s) {
			l.Skip()
		}
		if l.AcceptRun(s) {
			l.Skip()
		}
	}
}

// Accept returns a StateFn that consumes one character from the valid
// set.  If needed is true and if no character is consumed, an error
// is emitted.
func Accept(valid string, needed bool) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.Accept(valid) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		if needed {
			l.Errorf("expected character from the set %q, got %q", valid, l.Peek())
		}
		return false
	}
}

// AcceptRun returns a StateFn that consumes a run of runes from the
// input.  If needed is true and if no characters are consumed, an
// error is emitted.
func AcceptRun(valid string, needed bool) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.AcceptRun(valid) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		if needed {
			l.Errorf("expected a run of characters from the set %q, got %q", valid, l.Peek())
		}
		return false
	}
}

// Except returns a StateFn that consumes one character from the input
// that are not in the invalid set. If needed is true and no
// characters are consumed, an error is emitted.
func Except(invalid string, needed bool) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.Except(invalid) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		if needed {
			l.Errorf("expected a character outside the set %q, got %q", invalid, l.Peek())
		}
		return false
	}
}

// ExceptRun returns a StateFn that consumes a run of characters that
// are not in the invalid set.  If needed is true and if no characters
// are consumed, an error is emitted.
func ExceptRun(invalid string, needed bool) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.ExceptRun(invalid) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		if needed {
			l.Errorf("expected a character outside the set %q, got %q", invalid, l.Peek())
		}
		return false
	}
}

// Quote consumes a double-quote followed by a sequence of any
// non-double-quote characters, unescaped newline and double-quote
// characters are also consumed.  An error is emitted if an unescaped
// terminating quote is not found.  Escaped newlines are allowed
// within the quoted text.
func Quote(l *Lexer, t ItemType, emit bool) (success bool) {
	r := l.Next()
	if r != '"' {
		l.Errorf("expected '\"', got %q", r)
		l.Backup()
		return false
	}
	for {
		switch l.Next() {
		case '\\':
			l.Next()
		case '\n':
			l.Errorf("unterminated quote")
			l.Backup()
			return false
		case EOF:
			l.Errorf("unterminated quote")
			return false
		case '"':
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
	}
	return false
}

// Digits consumes unicode digits
func Digits(l *Lexer, t ItemType, emit bool) (success bool) {
	for {
		r := l.Next()
		if !unicode.IsDigit(r) {
			l.Backup()
			if l.pos > l.start {
				if emit {
					l.Emit(t)
				} else {
					l.Skip()
				}
				return true
			}
			l.Errorf("expected [0-9], got %q", r)
			return false
		}
	}
	return false
}

// Letters consumes unicode letters
func Letters(l *Lexer, t ItemType, emit bool) (success bool) {
	for {
		r := l.Next()
		if !unicode.IsLetter(r) {
			l.Backup()
			if l.pos > l.start {
				if emit {
					l.Emit(t)
				} else {
					l.Skip()
				}
				return true
			}
			l.Errorf("expected letter, got %q", r)
			return false
		}
	}
	return false
}

// Spaces consumes unicode spaces
func Spaces(l *Lexer, t ItemType, emit bool) (success bool) {
	for {
		r := l.Next()
		if !unicode.IsSpace(r) {
			l.Backup()
			if l.pos > l.start {
				if emit {
					l.Emit(t)
				} else {
					l.Skip()
				}
				return true
			}
			l.Errorf("expected whitespace, got %q", r)
			return false
		}
	}
	return false
}

// Number scans a number: decimal, octal, hex, float, or imaginary.
// This method is taken from the go text/templates/parser package, and
// has the same limitations.
func Number(l *Lexer, t ItemType, emit bool) bool {
	if !l.scanNumber() {
		l.Errorf("bad number syntax: %q", l.buf[l.start:l.pos])
		return false
	}
	if sign := l.Peek(); sign == '+' || sign == '-' {
		// Complex: 1+2i. No spaces, must end in 'i'.
		if !l.scanNumber() || l.buf[l.pos-1] != 'i' {
			l.Errorf("bad number syntax: %q", l.buf[l.start:l.pos])
			return false
		}
	}
	if emit {
		l.Emit(t)
	} else {
		l.Skip()
	}
	return true
}

func (l *Lexer) scanNumber() bool {
	// Optional leading sign.
	l.Accept("+-")
	// Is it hex?
	digits := "0123456789"
	if l.Accept("0") && l.Accept("xX") {
		digits = "0123456789abcdefABCDEF"
	}
	l.AcceptRun(digits)
	if l.Accept(".") {
		l.AcceptRun(digits)
	}
	if l.Accept("eE") {
		l.Accept("+-")
		l.AcceptRun("0123456789")
	}
	// Is it imaginary?
	l.Accept("i")
	// Next thing mustn't be alphanumeric.
	if isAlphaNumeric(l.Peek()) {
		l.Next()
		return false
	}
	return true
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
