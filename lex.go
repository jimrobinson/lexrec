// Much of this is inspired by and derived from Rob Pike's template
// parsing library (http://golang.org/pkg/text/template/parse/).
//
// Unlike the original code, this lexer is meant to handle reading
// staticly defined formats, so the StateFn won't necessarily know the
// proper order of the next token.  Instead we initialize the lexer
// with a Record that lists an array of Binding, defining the order of
// the StateFn for each record.
//
// The lexer is not expected hold the entire input in memory, instead
// the Record defines the size of the initial read buffer which will
// be used to read chunks of an io.Reader.  This buffer size will be
// increased as necessary.

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
	ErrorFn ErrorFn   // error function to apply if the lexer encounters a malformed record
	States  []Binding // lexer states that make up a record
}

// lexer holds the state of the scanner
type Lexer struct {
	name    string    // name of the input; used only for error reports
	r       io.Reader // input reader
	rec     Record    // log record definition
	items   chan Item // channel of lexed items
	eof     bool      // end of file reached?
	buf     []byte    // buffer of bytes read from r
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
		eof:   false,
	}
	go l.run()
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
		next := make([]byte, l.rec.Buflen)
		n, err := l.r.Read(next)
		if err == nil {
			l.buf = append(l.buf, next[0:n]...)
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

// Accept consumes the next rune if it in the valid set
func (l *Lexer) Accept(valid string) bool {
	if strings.IndexRune(valid, l.Next()) >= 0 {
		return true
	}
	l.Backup()
	return false
}

// AcceptRun consumes a run of runes from the valid set.
func (l *Lexer) AcceptRun(valid string) {
	for {
		r := l.Next()
		if r == EOF {
			break
		}
		if strings.IndexRune(valid, r) >= 0 {
			break
		}
	}
	l.Backup()
}

// Except consumes the next rune if it's not in the invalid set
func (l *Lexer) Except(invalid string) bool {
	if strings.IndexRune(invalid, l.Next()) < 0 {
		return true
	}
	l.Backup()
	return false
}

// ExceptRun consumes a run of runes that are not in the invalid set
func (l *Lexer) ExceptRun(invalid string) {
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
	if l.pos < l.rec.Buflen/2 {
		l.start = l.pos
	} else {
		l.buf, l.start, l.pos = append(l.buf[0:0], l.buf[l.pos:]...), 0, 0
	}
}

// SkipPast returns an ErrorFn that consumes a sequence of characters
// that are not in the set s, and one or more instances of the
// characters in the set s.  This is the equivalent of calling
// ExceptRun(s) followed by AcceptRun(s).
func SkipPast(s string) ErrorFn {
	return func(l *Lexer) {
		l.ExceptRun(s)
		l.AcceptRun(s)
	}
}

// Accept returns a StateFn that consumes one character from the valid
// set, emitting an error if one is not found.
func Accept(valid string) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.Accept(valid) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected character from the set [%q], got %q", valid, l.Peek())
		return false
	}
}

// AcceptRun returns a StateFn that consumes a run of runes from the
// input, emitting an error if none are found.
func AcceptRun(valid string) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		l.AcceptRun(valid)
		if l.pos > l.start {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected a run of characters from the set [%q], got %q", valid, l.Peek())
		return false
	}
}

// Except returns a StateFn that consumes one character from the input
// that are not in the invalid set, emitting an error if the first
// character read is in the invalid set.
func Except(invalid string) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.Except(invalid) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected a character outside the set [%q], got %q", invalid, l.Peek())
		return false
	}
}

// ExceptRun returns a StateFn that consumes a run of characters that
// are not in the invalid set.  If no characters are consumed before
// an invalid set rune is encountered, an error is emitted.
func ExceptRun(invalid string) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		l.ExceptRun(invalid)
		if l.pos > l.start {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected a character outside the set [%q], got %q", invalid, l.Peek())
		return false
	}
}

// QuotedString consumes a double-quote followed by a sequence of any
// non-double-quote characters, unescaped newline and double-quote
// characters are also consumed.
func QuotedString(l *Lexer, t ItemType, emit bool) (success bool) {
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

// Integer consumes unicode digits
func Integer(l *Lexer, t ItemType, emit bool) (success bool) {
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
