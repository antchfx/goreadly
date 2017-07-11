readability
===
readability is Go package for makes Web pages more readable.

[![GoDoc](https://godoc.org/github.com/antchfx/readability?status.svg)](https://godoc.org/github.com/antchfx/readability)

Install
===
    go get github.com/antchfx/readability

Example
===
```go
package main

import (
	"fmt"
	"net/http"

	"github.com/antchfx/readability"
)

func main() {
	resp, _ := http.Get("https://www.engadget.com/2017/07/10/google-highlights-pirate-sites/")
	doc, err := readability.NewDocument(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.Title())
	fmt.Println(doc.Content())
}
```