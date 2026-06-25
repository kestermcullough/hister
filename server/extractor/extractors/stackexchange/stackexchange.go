// Package stackexchange provides an extractor for Stack Exchange network
package stackexchange

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/extractor/urlutil"
	"github.com/asciimoo/hister/server/sanitizer"
	"github.com/asciimoo/hister/server/types"
)

// possible apex domains for the stack exchange network
var seDomains = []string{
	"stackexchange.com",
	"stackoverflow.com",
	"serverfault.com",
	"superuser.com",
	"askubuntu.com",
	"mathoverflow.net",
	"stackapps.com",
}

type StackExchangeExtractor struct {
	cfg *config.Extractor
}

func (e *StackExchangeExtractor) Name() string {
	return "StackExchange"
}

func (e *StackExchangeExtractor) Description() string {
	return "Extracts the question and all answers from Stack Exchange network question pages (Stack Overflow, Server Fault, Super User, Ask Ubuntu, *.stackexchange.com, and more)."
}

func (e *StackExchangeExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{Enable: true, Options: map[string]any{}}
	}
	return e.cfg
}

func (e *StackExchangeExtractor) SetConfig(c *config.Extractor) error {
	for k := range c.Options {
		return fmt.Errorf("unknown option %q", k)
	}
	e.cfg = c
	return nil
}

func isQuestionPath(path string) bool {
	const prefix = "/questions/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]
	return rest != "" && rest[0] >= '0' && rest[0] <= '9'
}

func (e *StackExchangeExtractor) Match(d *document.Document) bool {
	u, err := url.Parse(d.URL)
	if err != nil {
		return false
	}
	if !isQuestionPath(u.Path) {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, dom := range seDomains {
		if host == dom || strings.HasSuffix(host, "."+dom) {
			return true
		}
	}
	return false
}

func questionTitle(doc *goquery.Document) string {
	if t := strings.TrimSpace(doc.Find("#question-header h1").Text()); t != "" {
		return t
	}
	return strings.TrimSpace(doc.Find("title").First().Text())
}

func answerBodyText(s *goquery.Selection) string {
	body := s.Find(".js-post-body").First()
	if body.Length() == 0 {
		body = s.Find(".s-prose").First()
	}
	return strings.TrimSpace(body.Text())
}

func setMetadata(d *document.Document, doc *goquery.Document) {
	if d.Metadata == nil {
		d.Metadata = make(map[string]any)
	}
	q := doc.Find(".question").First()
	if author := strings.TrimSpace(q.Find(".user-details a").First().Text()); author != "" {
		d.Metadata["author"] = author
	}

	dateCreatedSel := q.Find("[itemprop=dateCreated]").First()
	if dateCreatedSel.Length() == 0 {
		dateCreatedSel = doc.Find("[itemprop=dateCreated]").First()
	}
	if dt, ok := dateCreatedSel.Attr("datetime"); ok && dt != "" {
		if parsed, err := time.Parse("2006-01-02 15:04:05Z07:00", dt); err == nil {
			d.Metadata["published"] = parsed.Format(time.RFC3339)
		} else if parsed, err := time.Parse(time.RFC3339, dt); err == nil {
			d.Metadata["published"] = parsed.Format(time.RFC3339)
		} else {
			d.Metadata["published"] = dt
		}
	}
	tags := make([]string, 0)
	q.Find(".post-tag").Each(func(_ int, s *goquery.Selection) {
		if tag := strings.TrimSpace(s.Text()); tag != "" {
			tags = append(tags, tag)
		}
	})
	if len(tags) > 0 {
		d.Metadata["tags"] = strings.Join(tags, ", ")
	}
	if score := strings.TrimSpace(q.Find(".js-vote-count").First().Text()); score != "" {
		d.Metadata["score"] = score
	}
	if n := doc.Find(".answer").Length(); n > 0 {
		d.Metadata["answers"] = n
	}
}

func (e *StackExchangeExtractor) Extract(d *document.Document) (types.ExtractorState, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(d.HTML))
	if err != nil {
		return types.ExtractorContinue, err
	}

	question := doc.Find(".question .js-post-body").First()
	if question.Length() == 0 {
		question = doc.Find(".js-post-body").First()
	}
	if question.Length() == 0 {
		return types.ExtractorContinue, fmt.Errorf("no question body found")
	}

	d.Title = questionTitle(doc)

	var b strings.Builder
	b.WriteString(strings.TrimSpace(question.Text()))
	doc.Find(".answer").Each(func(_ int, s *goquery.Selection) {
		text := answerBodyText(s)
		if text == "" {
			return
		}
		b.WriteString("\n\n")
		if s.HasClass("accepted-answer") {
			b.WriteString("[Accepted Answer]\n")
		}
		b.WriteString(text)
	})
	d.Text = strings.TrimSpace(b.String())

	setMetadata(d, doc)

	return types.ExtractorStop, nil
}

func (e *StackExchangeExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(d.HTML))
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}
	base, _ := url.Parse(d.URL)

	// removes "copy" button on codeblocks
	doc.Find("pre div").Remove()

	qSel := doc.Find(".question .js-post-body").First()
	if qSel.Length() == 0 {
		qSel = doc.Find(".js-post-body").First()
	}
	if qSel.Length() == 0 {
		return types.PreviewResponse{}, types.ExtractorContinue, fmt.Errorf("no question body found")
	}
	urlutil.RewriteURLs(qSel, base)
	question, err := qSel.Html()
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<h2>Question</h2>%s", question)

	n := 0
	doc.Find(".answer").Each(func(_ int, s *goquery.Selection) {
		body := s.Find(".js-post-body").First()
		if body.Length() == 0 {
			body = s.Find(".s-prose").First()
		}
		if body.Length() == 0 {
			return
		}
		urlutil.RewriteURLs(body, base)
		h, err := body.Html()
		if err != nil {
			return
		}
		n++
		heading := fmt.Sprintf("Answer #%d", n)
		if s.HasClass("accepted-answer") {
			heading += " (accepted)"
		}
		fmt.Fprintf(&b, "<hr /><h2>%s</h2>%s", heading, h)
	})

	return types.PreviewResponse{Content: sanitizer.SanitizeHTML(b.String())}, types.ExtractorStop, nil
}
