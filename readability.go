package readability

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/antchfx/xquery/html"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

var (
	// Logger is logging debug output.
	Logger = log.New(ioutil.Discard, "[readability] ", log.LstdFlags)
	// MinTextLength specified the minimum length of content.
	MinTextLength = 25

	// AllowedHtmlTagAttrs defines the tag attrs list that can allowed appear on content.
	AllowedHtmlTagAttrs = map[string]bool{
		"src":  true,
		"href": true,
	}

	blacklistCandidatesRegexp  = regexp.MustCompile(`(?i)popupbody`)
	okMaybeItsACandidateRegexp = regexp.MustCompile(`(?i)and|article|body|column|main|shadow`)
	unlikelyCandidatesRegexp   = regexp.MustCompile(`(?i)combx|comment|community|hidden|disqus|modal|extra|foot|header|menu|remark|rss|shoutbox|sidebar|sponsor|ad-break|agegate|pagination|pager|popup`)
	divToPElementsRegexp       = regexp.MustCompile(`(?i)<(a|blockquote|dl|div|img|ol|p|pre|table|ul)`)

	negativeRegexp = regexp.MustCompile(`(?i)combx|comment|com-|foot|footer|footnote|masthead|media|meta|outbrain|promo|related|scroll|shoutbox|sidebar|sponsor|shopping|tags|tool|widget`)
	positiveRegexp = regexp.MustCompile(`(?i)article|body|content|entry|hentry|main|page|pagination|post|text|blog|story`)

	sentenceRegexp = regexp.MustCompile(`\.( |$)`)

	normalizeWhitespaceRegexp = regexp.MustCompile(`[\r\n\f]+`)
)

// A Document represents an article document object.
type Document struct {
	// OriginalURL is an original request URL.
	OriginalURL string

	root        *html.Node
	respURL     *url.URL
	content     string
	contentOnce sync.Once
	title       string
	titleOnce   sync.Once
}

type candidate struct {
	node  *html.Node
	score float32
}

// Title returns article title of HTML page.
func (d *Document) Title() string {
	d.titleOnce.Do(func() {
		d.title = d.parseTitle()
	})
	return d.title
}

// Content returns article content of HTML page.
func (d *Document) Content() string {
	d.contentOnce.Do(func() {
		d.content = d.parseContent()
	})
	return d.content
}

func (d *Document) parseTitle() string {
	var title, betterTitle string
	if n := htmlquery.FindOne(d.root, "//meta[@property='og:title'or @name='twitter:title']"); n != nil {
		title = htmlquery.SelectAttr(n, "content")
	} else if n := htmlquery.FindOne(d.root, "//title"); n != nil {
		title = htmlquery.InnerText(n)
	}
	var seps = []string{" | ", " _ ", " - ", "«", "»", "—"}
	for _, sep := range seps {
		if array := strings.Split(title, sep); len(array) > 1 {
			if len(betterTitle) > 0 {
				// conflict with separate character
				betterTitle = title
				break
			}
			betterTitle = strings.TrimSpace(array[0])
		}
	}
	if len(betterTitle) > 10 {
		return betterTitle
	}
	return title
}

func (d *Document) parseContent() string {
	// remove unlikely candidates
	htmlquery.FindEach(d.root, "//*", func(_ int, n *html.Node) {
		switch n.Data {
		case "script", "style", "noscript":
			removeNodes(n)
			return
		case "html", "body":
			return
		}
		str := htmlquery.SelectAttr(n, "class") + htmlquery.SelectAttr(n, "id")
		if blacklistCandidatesRegexp.MatchString(str) || (unlikelyCandidatesRegexp.MatchString(str) && !okMaybeItsACandidateRegexp.MatchString(str)) {
			removeNodes(n)
		}
	})
	// turn all divs that don't have children block level elements into p's
	htmlquery.FindEach(d.root, "//div", func(_ int, n *html.Node) {
		htmlStr := htmlquery.OutputHTML(n, false)
		if !divToPElementsRegexp.MatchString(htmlStr) {
			n.Data = "p"
		}
	})
	// loop through all paragraphs, and assign a score to them based on how content-y they look.
	candidates := make(map[*html.Node]*candidate)
	htmlquery.FindEach(d.root, "//p|//td", func(_ int, n *html.Node) {
		text := htmlquery.InnerText(n)
		count := utf8.RuneCountInString(text)
		// if this paragraph is less than x chars, don't count it
		if count < MinTextLength {
			return
		}

		parent := n.Parent
		grandparent := parent.Parent
		if _, ok := candidates[parent]; !ok {
			candidates[parent] = d.scoreNode(parent)
		}
		if grandparent != nil {
			if _, ok := candidates[grandparent]; !ok {
				candidates[grandparent] = d.scoreNode(grandparent)
			}
		}
		contentScore := float32(1.0)
		// for any commas within this paragraph
		contentScore += float32(strings.Count(text, ","))
		contentScore += float32(strings.Count(text, "，")) // gb2312 character
		contentScore += float32(math.Min(float64(int(count/100.0)), 3))

		candidates[parent].score += contentScore
		if grandparent != nil {
			candidates[grandparent].score += contentScore / 2.0
		}
	})

	// scale the final candidates score based on link density. Good content
	// should have a relatively small link density (5% or less) and be mostly
	// unaffected by this operation
	var best *candidate
	for _, candidate := range candidates {
		candidate.score = candidate.score * (1 - d.getLinkDensity(candidate.node))
		if best == nil || best.score < candidate.score {
			best = candidate
		}
	}
	// if still have no top candidate, just use the body as a last resort.
	if best == nil {
		best = &candidate{htmlquery.FindOne(d.root, "//body"), 0}
	}

	// now that we have the top candidate, look through its siblings for content that might also be related.
	// like preambles, content split by ads that we removed, etc.
	var buf bytes.Buffer
	siblingScoreThreshold := float32(math.Max(10, float64(best.score*.2)))
	for n := best.node.Parent.FirstChild; n != nil; n = n.NextSibling {
		append := false
		if n == best.node {
			append = true
		} else if c, ok := candidates[n]; ok && c.score >= siblingScoreThreshold {
			append = true
		}

		if n.Data == "p" {
			linkDensity := d.getLinkDensity(n)
			content := htmlquery.InnerText(n)
			contentLength := utf8.RuneCountInString(content)
			if contentLength >= 80 && linkDensity < .25 {
				append = true
			} else if contentLength < 80 && linkDensity == 0 {
				append = sentenceRegexp.MatchString(content)
			}
		}
		if append {
			html.Render(&buf, n)
		}
	}
	// we have all of the content that we need.
	// now we clean it up for presentation.
	return d.sanitize(buf.String())
}

func (d *Document) sanitize(content string) string {
	doc, err := htmlquery.Parse(strings.NewReader(content))
	if err != nil {
		Logger.Println("Unable to create document", err)
		return ""
	}
	// clean out spurious headers from an element.
	htmlquery.FindEach(doc, "//*", func(_ int, n *html.Node) {
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6", "h7":
			if d.classWeight(n) < 0 || d.getLinkDensity(n) > 0.33 {
				removeNodes(n)
			}
		case "input", "select", "textarea", "button", "object", "iframe", "embed":
			removeNodes(n)
		}
	})

	d.cleanConditionally(doc, "table", "ul", "div")
	node := htmlquery.FindOne(doc, "//body")
	getAbsoluteUrl := func(path string) string {
		if d.respURL == nil {
			return path
		}
		if strings.HasPrefix(path, "http://") ||
			strings.HasPrefix(path, "https://") ||
			strings.HasPrefix(path, "ftp://") {
			return path
		}
		u, err := d.respURL.Parse(path)
		if err != nil {
			return path
		}
		return u.String()
	}
	var fn func(*bytes.Buffer, *html.Node)
	fn = func(buf *bytes.Buffer, n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			buf.WriteString(n.Data)
			return
		case n.Type == html.CommentNode:
			return
		}

		buf.WriteString("<" + n.Data)
		for _, attr := range n.Attr {
			if !AllowedHtmlTagAttrs[attr.Key] {
				continue
			}
			if (n.Data == "img" && attr.Key == "src") ||
				(n.Data == "a" && attr.Key == "href") ||
				(n.Data == "embed " && attr.Key == "src") {
				attr.Val = getAbsoluteUrl(attr.Val)
			}
			buf.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
		}
		if selfClosingHtmlTags[n.Data] {
			buf.WriteString("/>")
		} else {
			buf.WriteString(">")
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			fn(buf, child)
		}
		if !selfClosingHtmlTags[n.Data] {
			buf.WriteString("</" + n.Data + ">")
		}
	}

	var buf bytes.Buffer
	for node = node.FirstChild; node != nil; node = node.NextSibling {
		fn(&buf, node)
	}
	text := buf.String()
	if text == "" {
		text = htmlquery.OutputHTML(node, false)
	}
	return normalizeWhitespaceRegexp.ReplaceAllString(text, "\n")
}

func (d *Document) cleanConditionally(n *html.Node, tags ...string) {
	for i, tag := range tags {
		tags[i] = "//" + tag
	}
	selector := strings.Join(tags, "|")
	htmlquery.FindEach(n, selector, func(_ int, n *html.Node) {
		weight := float32(d.classWeight(n))
		if weight < 0 {
			removeNodes(n)
			return
		}
		text := htmlquery.InnerText(n)
		if strings.Count(text, ",")+strings.Count(text, "，") < 10 {
			// if there are not very many commas, and the number of
			// non-paragraph elements is more than paragraphs or other ominous signs, remove the element.
			var (
				p     = len(htmlquery.Find(n, "//p|//br"))
				img   = len(htmlquery.Find(n, "//img"))
				li    = len(htmlquery.Find(n, "//li")) - 100
				embed = len(htmlquery.Find(n, "//embed[@src]"))
				input = len(htmlquery.Find(n, "//input"))
			)

			contentLength := len(strings.TrimSpace(text))
			linkDensity := d.getLinkDensity(n)
			remove := false
			reason := ""
			if img > p && img > 1 {
				reason = "too many images"
				remove = true
			} else if li > p && n.Data != "ul" && n.Data != "ol" {
				reason = "less than 3x <p>s than <input>s"
				remove = true
			} else if input > (p / 3.0) {
				remove = true
			} else if contentLength < MinTextLength && (img == 0 || img > 2) {
				reason = "too short content length without a single image"
				remove = true
			} else if weight < 25 && linkDensity > 0.2 {
				reason = fmt.Sprintf("too many links for its weight (%f)", weight)
				remove = true
			} else if weight >= 25 && linkDensity > 0.5 {
				reason = fmt.Sprintf("too many links for its weight (%f)", weight)
				remove = true
			} else if (embed == 1 && contentLength < 75) || embed > 1 {
				reason = "<embed>s with too short a content length, or too many <embed>s"
				remove = true
			}

			if remove {
				Logger.Printf("Conditionally cleaned %s#%s.%s with weight %f and content score %f because it has %s\n", n.Data, htmlquery.SelectAttr(n, "id"), htmlquery.SelectAttr(n, "class"), weight, weight, reason)
				removeNodes(n)
			}
		}
	})
}

func (d *Document) scoreNode(n *html.Node) *candidate {
	contentScore := d.classWeight(n)
	switch n.Data {
	case "article":
		contentScore += 10
	case "section":
		contentScore += 8
	case "div":
		contentScore += 5
	case "pre", "td", "blockquote":
		contentScore += 3
	case "address", "ol", "ul", "dl", "dd", "dt", "li", "form":
		contentScore -= 3
	case "h1", "h2", "h3", "h4", "h5", "h6", "th":
		contentScore -= 5
	}
	// checking node has itemscope??
	for _, attr := range n.Attr {
		if attr.Key == "itemscope" {
			contentScore += 5
		}
		if attr.Key == "itemtype" {
			contentScore += 30
		}
	}
	return &candidate{n, float32(contentScore)}
}

func (d *Document) classWeight(n *html.Node) int {
	weight := 0
	if v := htmlquery.SelectAttr(n, "class"); v != "" {
		if negativeRegexp.MatchString(v) {
			weight -= 25
		}

		if positiveRegexp.MatchString(v) {
			weight += 25
		}
	}
	if v := htmlquery.SelectAttr(n, "id"); v != "" {
		if negativeRegexp.MatchString(v) {
			weight -= 25
		}

		if positiveRegexp.MatchString(v) {
			weight += 25
		}
	}
	return weight
}

func (d *Document) getLinkDensity(n *html.Node) float32 {
	textLength := utf8.RuneCountInString(htmlquery.InnerText(n))
	if textLength == 0 {
		return 0
	}
	linkLength := 0
	for _, n := range htmlquery.Find(n, "//a") {
		if v := htmlquery.SelectAttr(n, "href"); v == "" || v == "#" {
			continue
		}
		linkLength += utf8.RuneCountInString(htmlquery.InnerText(n))
	}
	return float32(linkLength) / float32(textLength)
}

func removeNodes(n *html.Node) {
	if n.Parent == nil {
		return
	}
	if n.NextSibling != nil {
		n.NextSibling.PrevSibling = n.PrevSibling
	}
	if n.PrevSibling != nil {
		n.PrevSibling.NextSibling = n.NextSibling
	}
	if n.Parent.FirstChild == n {
		n.Parent.FirstChild = n.NextSibling
	}
}

var (
	selfClosingHtmlTags = map[string]bool{
		"area":   true,
		"base":   true,
		"embed":  true,
		"iframe": true,
		"input":  true,
		"link":   true,
		"meta":   true,
		"param":  true,
		"source": true,
		"track":  true,
		"hr":     true,
		"img":    true,
		"br":     true,
	}
)

// LoadURL loads the HTML document from the specified URL.
func LoadURL(url string) (*Document, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	r, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	node, err := htmlquery.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML error: %s", err)
	}
	return &Document{
		OriginalURL: url,
		respURL:     resp.Request.URL,
		root:        node,
	}, nil
}

// Parse parses the HTML document from the given Reader.
func Parse(r io.Reader) (*Document, error) {
	node, err := htmlquery.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML error: %s", err)
	}
	return &Document{
		root: node,
	}, nil
}
