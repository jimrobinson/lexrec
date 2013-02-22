	PACKAGE
	
	package lexrec
	    import "/Users/jimr/proj/github/src/jimrobinson/lexrec"
	
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
	
	    The caller can iterate over NextItem(), looking for the ItemType events
	    that interest them.
	
	    Once the end of States is reached, an ItemEOR will be emitted. Once the
	    end of the input has been reached an ItemEOF will be emitted.
	
	    Much of this library was inspired by and derived from by Rob Pike's
	    template parsing libary (http://golang.org/pkg/text/template/parse/).
	    Any elegant bits in this library are from his original library.
	
	CONSTANTS
	
	const EOF = -1
	    eof indicates end of file for the input
	
	
	FUNCTIONS
	
	func Digits(l *Lexer, t ItemType, emit bool) (success bool)
	    Digits consumes unicode digits
	
	func Letters(l *Lexer, t ItemType, emit bool) (success bool)
	    Letters consumes unicode letters
	
	func Number(l *Lexer, t ItemType, emit bool) bool
	    Number scans a number: decimal, octal, hex, float, or imaginary. This
	    method is taken from the go text/templates/parser package, and has the
	    same limitations.
	
	func Quote(l *Lexer, t ItemType, emit bool) (success bool)
	    Quote consumes a double-quote followed by a sequence of any
	    non-double-quote characters, unescaped newline and double-quote
	    characters are also consumed.
	
	func Spaces(l *Lexer, t ItemType, emit bool) (success bool)
	    Spaces consumes unicode spaces
	
	
	TYPES
	
	type Binding struct {
	    ItemType ItemType // the type of this item
	    StateFn  StateFn  // the lexer function to call
	    Emit     bool     // emit the item type or skip over it
	}
	    Binding maps a lexer ItemType to a lexer StateFn. The boolean emit
	    controls whether or not the item is communicated to the parser.
	
	type ErrorFn func(l *Lexer)
	    ErrorFn is a function run when StateFn returns false (indicating a
	    record was not parsed successfully). Typically errorFn take steps to
	    recover the state of the lexer, e.g., skipping to the end of the record.
	
	func SkipPast(s string) ErrorFn
	    SkipPast returns an ErrorFn that consumes a sequence of characters that
	    are not in the set s, and one or more instances of the characters in the
	    set s. This is the equivalent of calling ExceptRun(s) followed by
	    AcceptRun(s).
	
	type Item struct {
	    Type  ItemType // the type of this item
	    Pos   int64    // the starting position, in bytes, of this item
	    Value string   //  the value of this item
	}
	    Item represents a lexed token item
	
	type ItemType int
	    ItemType represents the type of a lexical token
	
	const (
	    ItemError ItemType = iota // error detected
	    ItemEOR                   // end of record
	    ItemEOF                   // end of file
	)
	
	type Lexer struct {
	    // contains filtered or unexported fields
	}
	    lexer holds the state of the scanner
	
	func NewLexer(name string, r io.Reader, rec Record) (l *Lexer, err error)
	    NewLexer returns a lexer for rec records from the UTF-8 reader r. The
	    name is only used for debugging messages.
	
	func (l *Lexer) Accept(valid string) bool
	    Accept consumes the next rune if it in the valid set
	
	func (l *Lexer) AcceptRun(valid string)
	    AcceptRun consumes a run of runes from the valid set.
	
	func (l *Lexer) Backup()
	    Backup steps back one rune. Can only be called once per call of Next.
	
	func (l *Lexer) Bytes() []byte
	    Bytes returns the bytes in the current run of token characters
	
	func (l *Lexer) Emit(t ItemType)
	    Emit reports the current item to the client
	
	func (l *Lexer) Errorf(format string, args ...interface{})
	    Errorf returns an error token
	
	func (l *Lexer) Except(invalid string) bool
	    Except consumes the next rune if it's not in the invalid set
	
	func (l *Lexer) ExceptRun(invalid string)
	    ExceptRun consumes a run of runes that are not in the invalid set
	
	func (l *Lexer) LastPos() int64
	    LastPos returns the position of the most recent Item read from the input
	
	func (l *Lexer) Next() rune
	    Next consumes the next rune in the input.
	
	func (l *Lexer) NextItem() Item
	    NextItem returns the next Item from the input.
	
	func (l *Lexer) Peek() rune
	    Peek returns but does not consume the next rune in the input.
	
	func (l *Lexer) Size() int
	    Size returns the number of bytes in the current run of token characters
	
	func (l *Lexer) Skip()
	    Skip advances over the current item without reporting it
	
	type Record struct {
	    Buflen  int       // size of initial buffer, this will be grown as necessary
	    States  []Binding // lexer states that make up a record
	    ErrorFn ErrorFn   // error function to apply if the lexer encounters a malformed record
	}
	    Record represents a log record
	
	type StateFn func(l *Lexer, t ItemType, emit bool) (success bool)
	    StateFn is a function that can consume characters from the input and
	    emit an lexed token item as ItemType t. If emit is false the item will
	    be consumed but not transmitted.
	
	func Accept(valid string) StateFn
	    Accept returns a StateFn that consumes one character from the valid set,
	    emitting an error if one is not found.
	
	func AcceptRun(valid string) StateFn
	    AcceptRun returns a StateFn that consumes a run of runes from the input,
	    emitting an error if none are found.
	
	func Except(invalid string) StateFn
	    Except returns a StateFn that consumes one character from the input that
	    are not in the invalid set, emitting an error if the first character
	    read is in the invalid set.
	
	func ExceptRun(invalid string) StateFn
	    ExceptRun returns a StateFn that consumes a run of characters that are
	    not in the invalid set. If no characters are consumed before an invalid
	    set rune is encountered, an error is emitted.
	
	
