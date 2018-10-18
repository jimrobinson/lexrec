package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/jimrobinson/lexrec"
	"io"
	"log"
	"os"
)

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
