package goreadly

import (
	"fmt"
	"net/http"
	"testing"
)

func output(url string, doc *Document) {
	fmt.Println(url)
	fmt.Println(doc.Title)
	fmt.Println(doc.Body)
	fmt.Println("======================================")
}

func TestT(t *testing.T) {
}

func ExampleAll() {
	var urls = []string{
		"https://www.engadget.com/2018/02/27/asus-zenfone-5-hands-on-ai-ish/",
		"https://blogs.msdn.microsoft.com/dotnet/2018/02/27/announcing-entity-framework-core-2-1-preview-1/",
	}
	for _, url := range urls {
		res, err := http.Get(url)
		if err != nil {
			panic(err)
		}
		doc, err := ParseResponse(res)
		if err != nil {
			panic(err)
		}
		output(url, doc)
	}
	// Output:
}
