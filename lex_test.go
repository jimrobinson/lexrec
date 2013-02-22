package lexrec

import (
	"strings"
	"testing"
)

const (
	ItemIgnore ItemType = ItemEOF + 1 + iota
	ItemEmit
)

var acceptRunA = AcceptRun("a")

var aRecord = Record{
	Buflen:  1,
	ErrorFn: SkipPast("\n"),
	States: []Binding{
		{ItemEmit, acceptRunA, true}}}

func TestLexerAcceptRunA(t *testing.T) {
	r := strings.NewReader("aaaaaaaaaa")
	l, err := NewLexer("TestLexerAccceptRunA", r, aRecord)
	if err != nil {
		t.Fatal(err)
	}

	item := l.NextItem()
	if len(item.Value) != 10 {
		t.Error("expected 10 bytes, got %d\n", len(item.Value))
	}
}
