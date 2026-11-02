// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tracker // import "miniflux.app/v2/internal/reader/tracker"

import (
	"errors"
	"fmt"
	"html"
	"math"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"

	"miniflux.app/v2/internal/crypto"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/reader/date"
	"miniflux.app/v2/internal/reader/encoding"
)

const (
	minPosts            = 3
	maxPosts            = 25
	minTitleWords       = 3
	minTitleLength      = 24
	minSummaryLength    = 40
	maxPaginationDigits = 2
	minRootSlugDashes   = 2
	maxUndatedLabelSize = 3
	minGroupShare       = 0.4
	minDatedShare       = 0.8
	maxFutureDate       = 24 * time.Hour
	headingTags         = "h1, h2, h3, h4, h5, h6"
	dateElements        = `time, [datetime], [class*="date" i]`
	articleDateElements = `meta[property="article:published_time"], meta[name="date"], meta[itemprop="datePublished"]`
	sharedByPosts       = ""
)

const (
	titleFromImage = iota
	titleFromHiddenLink
	titleFromLink
	titleFromHeading
)

var ErrNoPosts = errors.New("tracker: no posts found on this page")

var (
	assetExtensions = []string{".css", ".csv", ".doc", ".docx", ".gif", ".jpeg", ".jpg", ".js", ".json", ".mp3", ".mp4", ".pdf", ".png", ".svg", ".webp", ".xml", ".zip"}
	listingSegments = []string{"about", "account", "author", "authors", "careers", "categories", "category", "contact", "cookies", "jobs", "legal", "login", "page", "people", "pricing", "privacy", "search", "signin", "signup", "staff", "tag", "tags", "team", "terms", "topic", "topics"}
	paginationQuery = []string{"offset", "p", "page"}
	inlineTags      = []string{"a", "abbr", "b", "bdi", "br", "cite", "code", "em", "i", "kbd", "mark", "s", "small", "strong", "sub", "sup", "u", "wbr"}
	nonTextTags     = []string{"img", "noscript", "picture", "script", "style", "svg", "time"}

	jsonLDDate = regexp.MustCompile(`"datePublished"\s*:\s*"([^"]+)"`)
	urlDate    = regexp.MustCompile(`/((?:19|20)\d{2})/(\d{2})/(\d{2})/`)
	textDate   = regexp.MustCompile(`(?i)\b(?:\d{1,2}\s+)?(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+(?:\d{1,2},?\s+)?(?:19|20)\d{2}\b|\b(?:19|20)\d{2}-\d{2}-\d{2}\b`)
)

type post struct {
	key        string
	url        string
	group      string
	title      string
	titleScore int
	summary    string
	date       time.Time
	link       *goquery.Selection
}

func Parse(pageURL, contentType string, body []byte) (*model.Feed, error) {
	page, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("tracker: invalid page URL: %w", err)
	}

	reader, err := encoding.NewCharsetReaderFromBytes(body, contentType)
	if err != nil {
		return nil, fmt.Errorf("tracker: unable to decode the page: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("tracker: unable to parse the page: %w", err)
	}

	posts := findPosts(doc, page)
	if len(posts) < minPosts {
		return nil, ErrNoPosts
	}

	return &model.Feed{
		Title:    pageTitle(doc, page),
		FeedURL:  pageURL,
		SiteURL:  pageURL,
		Language: doc.Find("html").AttrOr("lang", ""),
		Entries:  newEntries(posts),
	}, nil
}

func findPosts(doc *goquery.Document, page *url.URL) []*post {
	posts := largestGroups(collectPosts(doc, page))
	addCardDetails(posts)

	dated := 0
	for _, item := range posts {
		if !item.date.IsZero() {
			dated++
		}
	}

	if float64(dated) >= float64(len(posts))*minDatedShare {
		posts = slices.DeleteFunc(posts, isUndatedLabel)
		slices.SortStableFunc(posts, func(a, b *post) int {
			return b.date.Compare(a.date)
		})
	}

	return posts[:min(len(posts), maxPosts)]
}

func isUndatedLabel(item *post) bool {
	return item.date.IsZero() && len(strings.Fields(item.title)) <= maxUndatedLabelSize
}

func collectPosts(doc *goquery.Document, page *url.URL) []*post {
	byKey := make(map[string]*post)
	var posts []*post

	doc.Find("a[href]").Each(func(_ int, link *goquery.Selection) {
		if isSiteChrome(link) {
			return
		}

		target, err := page.Parse(strings.TrimSpace(link.AttrOr("href", "")))
		if err != nil || !isPostURL(target, page) {
			return
		}

		target.Fragment = ""
		key := strings.TrimRight(target.Host+target.Path, "/") + "?" + target.RawQuery
		title, titleScore := linkTitle(link)

		existing, found := byKey[key]
		if !found {
			existing = &post{key: key, url: target.String(), group: urlGroup(target), link: link, titleScore: -1}
			byKey[key] = existing
			posts = append(posts, existing)
		}

		if title != "" && titleScore > existing.titleScore {
			existing.title = title
			existing.titleScore = titleScore
		}
	})

	return slices.DeleteFunc(posts, func(item *post) bool {
		return !isPostTitle(item.title)
	})
}

func isSiteChrome(link *goquery.Selection) bool {
	if link.Closest("nav, footer, aside").Length() > 0 {
		return true
	}

	header := link.Closest("header")
	return header.Length() > 0 && header.Closest("main, article").Length() == 0
}

func isPostURL(target, page *url.URL) bool {
	if (target.Scheme != "https" && target.Scheme != "http") || !isSameSite(target.Host, page.Host) {
		return false
	}

	segments := pathSegments(target)
	if len(segments) == 0 || slices.Equal(segments, pathSegments(page)) {
		return false
	}

	last := segments[len(segments)-1]
	if slices.Contains(assetExtensions, strings.ToLower(path.Ext(last))) || isPageNumber(last) {
		return false
	}

	for _, name := range paginationQuery {
		if target.Query().Has(name) {
			return false
		}
	}

	if len(segments) == 1 {
		return strings.Count(last, "-") >= minRootSlugDashes
	}

	return !slices.ContainsFunc(segments[:len(segments)-1], func(segment string) bool {
		return slices.Contains(listingSegments, strings.ToLower(segment))
	})
}

func isSameSite(a, b string) bool {
	return strings.EqualFold(strings.TrimPrefix(a, "www."), strings.TrimPrefix(b, "www."))
}

func pathSegments(target *url.URL) []string {
	return strings.FieldsFunc(target.Path, func(r rune) bool { return r == '/' })
}

func isPageNumber(segment string) bool {
	_, err := strconv.Atoi(segment)
	return err == nil && len(segment) <= maxPaginationDigits
}

func urlGroup(target *url.URL) string {
	segments := pathSegments(target)
	folders := slices.Clone(segments[:len(segments)-1])
	for index, folder := range folders {
		if strings.IndexFunc(folder, func(r rune) bool { return !unicode.IsDigit(r) }) == -1 {
			folders[index] = "#"
		}
	}

	return "/" + strings.Join(folders, "/")
}

func linkTitle(link *goquery.Selection) (string, int) {
	if heading := link.Find(headingTags).First(); heading.Length() > 0 {
		return cleanText(heading.Text()), titleFromHeading
	}

	text := mainText(link)
	switch {
	case text == "":
		return cleanText(link.Find("img[alt]").First().AttrOr("alt", "")), titleFromImage
	case link.Closest(headingTags).Length() > 0:
		return text, titleFromHeading
	case link.AttrOr("tabindex", "") == "-1" || link.AttrOr("aria-hidden", "") == "true":
		return text, titleFromHiddenLink
	}

	return text, titleFromLink
}

func mainText(link *goquery.Selection) string {
	blocks := textBlocks(link.Get(0))
	if index := slices.IndexFunc(blocks, isPostTitle); index != -1 {
		return blocks[index]
	}

	return slices.MaxFunc(append(blocks, ""), func(a, b string) int {
		return len(a) - len(b)
	})
}

func textBlocks(root *xhtml.Node) []string {
	var blocks []string
	var block strings.Builder

	endBlock := func() {
		if text := cleanText(block.String()); text != "" {
			blocks = append(blocks, text)
		}
		block.Reset()
	}

	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			switch {
			case child.Type == xhtml.TextNode:
				block.WriteString(child.Data)
			case child.Type != xhtml.ElementNode || slices.Contains(nonTextTags, child.Data):
			case slices.Contains(inlineTags, child.Data), child.Data == "span" && hasOwnText(node):
				block.WriteByte(' ')
				visit(child)
				block.WriteByte(' ')
			default:
				endBlock()
				visit(child)
				endBlock()
			}
		}
	}

	visit(root)
	endBlock()
	return blocks
}

func hasOwnText(node *xhtml.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.TextNode && strings.TrimSpace(child.Data) != "" {
			return true
		}
	}

	return false
}

func cleanText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func isPostTitle(title string) bool {
	return len(strings.Fields(title)) >= minTitleWords || len([]rune(title)) >= minTitleLength
}

func largestGroups(posts []*post) []*post {
	sizes := make(map[string]int)
	largest := 0
	for _, item := range posts {
		sizes[item.group]++
		largest = max(largest, sizes[item.group])
	}

	minSize := max(minPosts, int(math.Ceil(float64(largest)*minGroupShare)))
	return slices.DeleteFunc(posts, func(item *post) bool {
		return sizes[item.group] < minSize
	})
}

func addCardDetails(posts []*post) {
	owners := make(map[*xhtml.Node]string)
	for _, item := range posts {
		for node := item.link.Get(0).Parent; node != nil; node = node.Parent {
			if owner, found := owners[node]; found && owner != item.key {
				owners[node] = sharedByPosts
			} else {
				owners[node] = item.key
			}
		}
	}

	for _, item := range posts {
		card := item.link
		for parent := card.Parent(); parent.Length() > 0 && owners[parent.Get(0)] == item.key; parent = parent.Parent() {
			card = parent
		}

		item.date = postDate(card, item.url)
		item.summary = cardSummary(card, item.title)
	}
}

func postDate(card *goquery.Selection, postURL string) time.Time {
	var published time.Time
	card.Find(dateElements).EachWithBreak(func(_ int, element *goquery.Selection) bool {
		published = parseDate(element.AttrOr("datetime", cleanText(element.Text())))
		return published.IsZero()
	})

	if !published.IsZero() {
		return published
	}

	if parts := urlDate.FindStringSubmatch(postURL); parts != nil {
		return parseDate(parts[1] + "-" + parts[2] + "-" + parts[3])
	}

	return parseDate(textDate.FindString(strings.Join(textBlocks(card.Get(0)), " ")))
}

func parseDate(value string) time.Time {
	parsed, err := date.Parse(value)
	if err != nil || parsed.After(time.Now().Add(maxFutureDate)) {
		return time.Time{}
	}

	return parsed
}

func cardSummary(card *goquery.Selection, title string) string {
	summary := ""
	card.Find("p").EachWithBreak(func(_ int, paragraph *goquery.Selection) bool {
		text := cleanText(paragraph.Text())
		if len([]rune(text)) < minSummaryLength || text == title {
			return true
		}

		summary = text
		return false
	})

	return summary
}

func PublishedDate(contentType string, body []byte) time.Time {
	reader, err := encoding.NewCharsetReaderFromBytes(body, contentType)
	if err != nil {
		return time.Time{}
	}

	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return time.Time{}
	}

	if published := parseDate(doc.Find(articleDateElements).First().AttrOr("content", "")); !published.IsZero() {
		return published
	}

	if parts := jsonLDDate.FindSubmatch(body); parts != nil {
		return parseDate(string(parts[1]))
	}

	return parseDate(doc.Find("time[datetime]").First().AttrOr("datetime", ""))
}

func pageTitle(doc *goquery.Document, page *url.URL) string {
	if siteName := cleanText(doc.Find(`meta[property="og:site_name"]`).AttrOr("content", "")); siteName != "" {
		return siteName
	}

	if title := cleanText(doc.Find("title").First().Text()); title != "" {
		return title
	}

	return page.Hostname()
}

func newEntries(posts []*post) model.Entries {
	now := time.Now()
	entries := make(model.Entries, 0, len(posts))
	for _, item := range posts {
		entry := model.NewEntry()
		entry.URL = item.url
		entry.Title = item.title
		entry.Hash = crypto.SHA256(item.url)
		entry.Date = item.date
		if entry.Date.IsZero() {
			entry.Date = now
		}
		if item.summary != "" {
			entry.Content = "<p>" + html.EscapeString(item.summary) + "</p>"
		}
		entries = append(entries, entry)
	}

	return entries
}
