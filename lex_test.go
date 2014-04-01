package lexrec

import (
	//"fmt"
	"strings"
	"testing"
)

const (
	ItemIgnore ItemType = ItemEOF + 1 + iota
	ItemAorB
	ItemTab
	ItemA
	ItemColon
	ItemB
	ItemEmit
)

type lexTest struct {
	name   string
	input  string
	record Record
	expect []Item
}

/*
var (
	tItemEOF = Item{ItemEOF, 0, ""}
)

var parseA = Record{Buflen:1, ErrorFn:SkipPast("\n"), States:[]Binding{{ItemA, AcceptRun("a", true),true}}}

var lexTests = []lexTest{
	{"empty", "", parseA, []Item[tItemEOF]},
}

func collect(l *Lexer) []Item {
	items := []Item{}
	for {
		item := l.NextItem()
		items = append(items, items)
		if item.Type == ItemEOF {
			break
		}
	}
	return items
}

func TestLex(t *testing.T) {
	for _, test := range lexTests {
		l := NewLexer(test.name, test.record)
		items := collect(l)

		n := len(test.expect)
		if len(items) < n {
			t.Errorf("missing trailing items: %v", test.expect[len(items):])
		} else if len(items) > n {
			t.Errorf("unexpected extra items: %v", test.items[n:])
		}

		for i := range items {
			if items[i].Type != test.expect[i].Type {
				t.Errorf("got Type %q expected Type %q\n", items[i].Type, test.expect[i].Type)
			}
			if items[i].Pos != test.expect[i].Pos {
				t.Errorf("got Pos %q expected Pos %q\n", items[i].Pos, test.expect[i].Pos)
			}
			if items[i].Value != test.expect[i].Value {
				t.Errorf("got Value %q expected Value %q\n", items[i].Value, test.expect[i].Value)
			}
		}
	}
}
*/

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
	r := strings.NewReader("bbb\n\n\n\n\na")
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
