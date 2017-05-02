readability
===
readability is Go package for makes Web pages more readable, inspire by [node-readability](https://github.com/luin/readability) and [go-readability](https://github.com/mauidude/go-readability)

[![GoDoc](https://godoc.org/github.com/antchfx/readability?status.svg)](https://godoc.org/github.com/antchfx/readability)

Feature
===
+ Supporting HTML5 tags(article, section).
+ Supporting encodings such as GBK and GB2312.
+ Converting relative urls to absolute for images and links automatically
+ Customized allowed tag and with attrs.

Install
===
    go get github.com/antchfx/readability

Example
===
```go
import (
    "github.com/antchfx/readability"
)

func main() {
	doc, err := readability.LoadURL("https://www.engadget.com/2017/05/02/switch-zelda-nintendo-dlc-hard-mode-tingle/")
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.Title())
	fmt.Println(doc.Content())
}
```