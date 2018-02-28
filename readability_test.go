package readability

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func loadURL(url string) (io.ReadCloser, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

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
		rc, err := loadURL(url)
		if err != nil {
			panic(err)
		}
		defer rc.Close()
		doc, err := Read(rc)
		if err != nil {
			panic(err)
		}
		output(url, doc)
	}
	// Output:
}
