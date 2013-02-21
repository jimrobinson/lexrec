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
	"unicode"
	"unicode/utf8"
)

// eof indicates end of file for the input
const eof = -1

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
		return eof
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

// Accept consumes the next rune if it is c
func (l *Lexer) Accept(c rune) bool {
	if l.Next() == c {
		return true
	}
	l.Backup()
	return false
}

// AcceptRun consumes a run of rune c
func (l *Lexer) AcceptRun(c rune) {
	for {
		r := l.Next()
		if r != c || r == eof {
			break
		}
	}
	l.Backup()
}

// Except consumes the next rune if it's not rune c
func (l *Lexer) Except(c rune) bool {
	if c != l.Next() {
		return true
	}
	l.Backup()
	return false
}

// ExceptRun consumes a run of runes that are not rune c
func (l *Lexer) ExceptRun(c rune) {
	for {
		r := l.Next()
		if r == c || r == eof {
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

// run consumes input, emitting ItemType events until eof is reached.
func (l *Lexer) run() {
	defer close(l.items)
	for {
		for _, state := range l.rec.States {
			if !state.StateFn(l, state.ItemType, state.Emit) {
				l.rec.ErrorFn(l)
				break
			}
		}
		if l.Peek() == eof {
			l.Emit(ItemEOF)
			break
		}
	}
}

// SkipPast returns an ErrorFn that consumes a sequence of characters
// that are not c, and one or more instances of c.  This is the
// equivalent of calling ExceptRun(c) followed by AcceptRun(c).
func SkipPast(c rune) ErrorFn {
	return func(l *Lexer) {
		l.ExceptRun(c)
		l.AcceptRun(c)
	}
}

// Accept returns a StateFn that consumes one character c, emitting an
// error if it is not found.
func Accept(c rune) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.Accept(c) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected character %q, got %q", c, l.Peek())
		return false
	}
}

// AcceptRun returns a StateFn that consumes a run of character c from
// the input, emitting an error if
// it is are found.
func AcceptRun(c rune) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		l.AcceptRun(c)
		if l.pos > l.start {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected a run of character %q, got %q", c, l.Peek())
		return false
	}
}

// Except returns a StateFn that consumes one character from the input
// if it is not c, or emitting an error if the next character is c.
func Except(c rune) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		if l.Except(c) {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("expected a character other than %q", c)
		return false
	}
}

// ExceptRun returns a StateFn that consumes a run of characters that
// are not c.  If no characters are consumed before c is encountered,
// an error is emitted.
func ExceptRun(c rune) StateFn {
	return func(l *Lexer, t ItemType, emit bool) bool {
		l.ExceptRun(c)
		if l.pos > l.start {
			if emit {
				l.Emit(t)
			} else {
				l.Skip()
			}
			return true
		}
		l.Errorf("got unexpected character %q", c)
		return false
	}
}

// QuotedString consumes a sequence of characters until an
// unescaped quote or newline is encountered, and reports the sequence
// as ItemType t.
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
		case eof:
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

// Integer consumes digits 0-9
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

// NumericTz consumes a timezone field in the format [+-]HHMM or [+-]HH:MM
func NumericTz(l *Lexer, t ItemType, emit bool) (success bool) {
	r := l.Next()
	if r != '+' && r != '-' {
		l.Errorf("expected timezone sign [+-], got %q", r)
		l.Backup()
		return false
	}
	for i := 0; i < 2; i++ {
		r = l.Next()
		if !unicode.IsDigit(r) {
			l.Errorf("expected [0-9], got %q", r)
			l.Backup()
			return false
		}
	}
	if l.Peek() == ':' {
		l.Next()
	}
	for i := 0; i < 2; i++ {
		r = l.Next()
		if !unicode.IsDigit(r) {
			l.Errorf("expected [0-9], got %q", r)
			l.Backup()
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
