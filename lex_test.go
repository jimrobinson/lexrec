package lexrec

import (
	"strings"
	"testing"
)

const (
	ItemIgnore ItemType = ItemEOF + 1 + iota
	ItemEmit
)

var acceptRunA = AcceptRun("a", true)

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
		t.Errorf("expected 10 bytes, got %d\n", len(item.Value))
	}
}

func TestLexerSkipPast(t *testing.T) {
	r := strings.NewReader("b\n\n\n\n\na")
	l, err := NewLexer("TestLexerSkipPast", r, aRecord)
	if err != nil {
		t.Fatal(err)
	}

	item := l.NextItem()
	if item.Type != ItemError {
		t.Fatalf("expected ItemError on character b, got %q", item)
	}

	item = l.NextItem()
	if item.Type != ItemEmit {
		t.Fatalf("expected ItemEmit on character b, got %q", item)
	}
	if item.Value != "a" {
		t.Fatalf("expected ItemEmit of one character 'a', got %q", item.Value)
	}
}
