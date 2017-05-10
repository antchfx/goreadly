package main

import (
	"fmt"
	"os"

	"github.com/antchfx/readability"
)

func main() {
	doc, err := readability.FromURL(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.Title())
	fmt.Println(doc.Content())
}
