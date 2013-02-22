package lexrec
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
