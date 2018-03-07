package main

import (
	"fmt"
	"net/http"

	"github.com/antchfx/goreadly"
)

func main() {
	resp, err := http.Get("https://cn.engadget.com/2018/02/27/asus-zenfone-5-hands-on/")
	if err != nil {
		panic(err)
	}
	doc, err := goreadly.ParseResponse(resp)
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.Title)
	fmt.Println(doc.Body)
}
