// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tracker // import "miniflux.app/v2/internal/reader/tracker"

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"miniflux.app/v2/internal/crypto"
	"miniflux.app/v2/internal/model"
)

const pageContentType = "text/html; charset=utf-8"

func parse(t *testing.T, pageURL, body string) *model.Feed {
	t.Helper()

	feed, err := Parse(pageURL, pageContentType, []byte(body))
	if err != nil {
		t.Fatal(err)
	}

	return feed
}

func titles(feed *model.Feed) []string {
	result := make([]string, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		result = append(result, entry.Title)
	}
	return result
}

func TestParseReadsNewsroomCards(t *testing.T) {
	feed := parse(t, "https://www.example.org/news", `<html lang="en"><head><title>Newsroom \ Example</title></head><body>
		<header><nav><a href="/news/about-this-company-and-its-mission">About this company and its mission</a></nav></header>
		<main>
			<ul class="filters">
				<li><a href="/news/announcements">Announcements</a></li>
				<li><a href="/news/policy">Policy</a></li>
			</ul>
			<article>
				<header><p><a href="/news/announcements">Announcements</a></p><time>Oct 18, 2026</time></header>
				<a tabindex="-1" aria-hidden="true" href="/news/public-benefit"><img alt="Investing in the public benefit"></a>
				<a href="/news/public-benefit"><h3>Investing in the public benefit</h3><p>We are committing a portion of our shares to a new fund for everyone.</p></a>
			</article>
			<article>
				<header><p><a href="/news/announcements">Announcements</a></p><time>Oct 26, 2026</time></header>
				<a href="/news/filing"><h3>Filing a registration statement today</h3><p>Today we filed a registration statement with the regulator in question.</p></a>
			</article>
			<ul>
				<li>
					<a href="/news/expanding"><span><h3>Expanding our partnership with the government</h3></span></a>
					<a tabindex="-1" href="/news/expanding"><p>A much longer description of the partnership that should not become the title.</p></a>
					<time>Oct 20, 2026</time>
				</li>
			</ul>
			<a href="/news?page=2">Next page of the newsroom results</a>
		</main>
		<footer><a href="/news/press-kit-and-brand-assets-download">Press kit and brand assets download</a></footer>
	</body></html>`)

	expected := []string{"Filing a registration statement today", "Expanding our partnership with the government", "Investing in the public benefit"}
	if result := titles(feed); !slices.Equal(result, expected) {
		t.Fatalf("unexpected posts: %q", result)
	}

	newest := feed.Entries[0]
	if newest.URL != "https://www.example.org/news/filing" {
		t.Errorf("unexpected URL %q", newest.URL)
	}

	if newest.Hash != crypto.SHA256("https://www.example.org/news/filing") {
		t.Errorf("unexpected hash %q", newest.Hash)
	}

	if !newest.Date.Equal(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected date %v", newest.Date)
	}

	if newest.Content != "<p>Today we filed a registration statement with the regulator in question.</p>" {
		t.Errorf("unexpected content %q", newest.Content)
	}

	if feed.Title != `Newsroom \ Example` || feed.Language != "en" || feed.SiteURL != "https://www.example.org/news" {
		t.Errorf("unexpected feed %q %q %q", feed.Title, feed.Language, feed.SiteURL)
	}
}

func TestParseFollowsPostsOutsideThePagePath(t *testing.T) {
	feed := parse(t, "https://example.org/news", `<html><head>
		<meta property="og:site_name" content="Example AI"><title>News | Example AI</title></head><body><main>
		<a href="/news/models">Models</a>
		<a href="/news/company-news">Company news</a>
		<ul>
			<li><time dateTime="2026-10-22">10.22.2026</time><a href="/blog/low-latency">Real-time intelligence when it counts</a></li>
			<li><time dateTime="2026-10-15">10.15.2026</time><a href="/blog/decision-model">Introducing our decision model</a></li>
			<li><time dateTime="2026-10-27">10.27.2026</time><a href="/blog/open-model">Open models for text and vision</a></li>
		</ul>
		<a href="/news/page/2">Page two of the news archive</a>
		<a href="https://arxiv.org/abs/2026.12345">A paper hosted on another website</a>
	</main></body></html>`)

	expected := []string{"Open models for text and vision", "Real-time intelligence when it counts", "Introducing our decision model"}
	if result := titles(feed); !slices.Equal(result, expected) {
		t.Fatalf("unexpected posts: %q", result)
	}

	if feed.Title != "Example AI" {
		t.Errorf("unexpected title %q", feed.Title)
	}
}

func TestParseGroupsDatedAndSectionURLs(t *testing.T) {
	feed := parse(t, "https://example.org/", `<html><body>
		<h2><a href="/2026/10/03/first-post">The first post of the month</a></h2>
		<h2><a href="/2026/10/12/second-post">The second post of the month</a></h2>
		<h2><a href="/2026/09/30/third-post">The third post of last month</a></h2>
		<p><a href="/shop/buy-this-very-nice-shirt">Buy this very nice shirt</a></p>
	</body></html>`)

	if len(feed.Entries) != 3 {
		t.Fatalf("unexpected posts: %q", titles(feed))
	}

	if feed.Entries[0].Date.IsZero() {
		t.Error("expected a fallback date")
	}
}

func TestParseKeepsPageOrderWithoutEnoughDates(t *testing.T) {
	feed := parse(t, "https://example.org/blog", `<html><body>
		<article><a href="/blog/one">The first listed post</a></article>
		<article><a href="/blog/two">The second listed post</a></article>
		<article><a href="/blog/three">The third listed post</a><span class="postDate">Oct 3, 2026</span></article>
	</body></html>`)

	if feed.Entries[2].Date.Year() != 2026 || feed.Entries[2].Date.Month() != time.October {
		t.Errorf("expected the date from the card, got %v", feed.Entries[2].Date)
	}

	expected := []string{"The first listed post", "The second listed post", "The third listed post"}
	if result := titles(feed); !slices.Equal(result, expected) {
		t.Fatalf("unexpected posts: %q", result)
	}
}

func TestParseLimitsThePostsToTheNewest(t *testing.T) {
	var page strings.Builder
	page.WriteString("<html><body>")
	for day := 1; day <= 28; day++ {
		fmt.Fprintf(&page, `<article><a href="/blog/post-%d">Blog post number %d</a><time datetime="2026-10-%02d"></time></article>`, day, day, day)
	}
	page.WriteString("</body></html>")

	feed := parse(t, "https://example.org/blog", page.String())
	if len(feed.Entries) != maxPosts {
		t.Fatalf("unexpected number of posts: %d", len(feed.Entries))
	}

	if feed.Entries[0].Title != "Blog post number 28" {
		t.Errorf("unexpected first post %q", feed.Entries[0].Title)
	}
}

func TestParseMergesDuplicateLinks(t *testing.T) {
	feed := parse(t, "https://example.org/blog", `<html><body>
		<article><a href="/blog/one#comments">The first listed post</a></article>
		<article><a href="/blog/one/">The first listed post</a></article>
		<article><a href="/blog/two">The second listed post</a></article>
		<article><a href="/blog/three">The third listed post</a></article>
	</body></html>`)

	if len(feed.Entries) != 3 {
		t.Fatalf("unexpected posts: %q", titles(feed))
	}
}

func TestParseRejectsPagesWithoutPosts(t *testing.T) {
	scenarios := map[string]string{
		"empty":        `<html><body></body></html>`,
		"too few":      `<html><body><a href="/blog/one">The first listed post</a><a href="/blog/two">The second listed post</a></body></html>`,
		"only labels":  `<html><body><a href="/news/models">Models</a><a href="/news/policy">Policy</a><a href="/news/research">Research</a></body></html>`,
		"only chrome":  `<html><body><nav><a href="/blog/one">The first listed post</a><a href="/blog/two">The second listed post</a><a href="/blog/three">The third listed post</a></nav></body></html>`,
		"only assets":  `<html><body><a href="/files/one.pdf">The first listed report</a><a href="/files/two.pdf">The second listed report</a><a href="/files/three.pdf">The third listed report</a></body></html>`,
		"only foreign": `<html><body><a href="https://a.test/blog/one">The first listed post</a><a href="https://a.test/blog/two">The second listed post</a><a href="https://a.test/blog/three">The third listed post</a></body></html>`,
	}

	for name, body := range scenarios {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("https://example.org/news", pageContentType, []byte(body)); !errors.Is(err, ErrNoPosts) {
				t.Errorf("expected no posts, got %v", err)
			}
		})
	}
}

func TestParseUsesDatesFromURLs(t *testing.T) {
	feed := parse(t, "https://example.org/", `<html><body>
		<a href="/news/2026/10/29/first-story">The first story of the day</a>
		<a href="/news/2026/10/06/second-story">The second story of the month</a>
		<a href="/news/2026/09/14/third-story">The third story of the year</a>
	</body></html>`)

	for index, day := range []string{"2026-10-29", "2026-10-06", "2026-09-14"} {
		if result := feed.Entries[index].Date.Format("2006-01-02"); result != day {
			t.Errorf("post %d: expected %s, got %s", index, day, result)
		}
	}
}

func TestParseUsesDatesFromCardText(t *testing.T) {
	feed := parse(t, "https://example.org/news", `<html><body>
		<div><a href="/news/first-story"><span>The first story of the year</span><div>Sep 14, 2026</div></a></div>
		<div><a href="/news/second-story">The second story of the year</a><div>Published 3 August 2026 by the editors</div></div>
		<div><a href="/news/third-story">The third story of the year</a><div>2026-07-01</div></div>
	</body></html>`)

	for index, day := range []string{"2026-09-14", "2026-08-03", "2026-07-01"} {
		if result := feed.Entries[index].Date.Format("2006-01-02"); result != day {
			t.Errorf("post %d: expected %s, got %s", index, day, result)
		}
	}
}

func TestParseKeepsTitlesFreeOfCardDetails(t *testing.T) {
	feed := parse(t, "https://example.org/blog", `<html><body>
		<a href="/blog/one"><img src="/a.png" alt="A photo"><div>Models</div><div>Introducing <em>our</em> workflow engine</div><div>Jul 2, 2026</div></a>
		<a href="/blog/two"><time>Jul 1, 2026</time><span>News</span><span>A second post with a long title</span></a>
		<a href="/blog/three"><div>A third post about something</div><div>A long description of the post that follows the title in the card.</div></a>
	</body></html>`)

	expected := []string{"Introducing our workflow engine", "A second post with a long title", "A third post about something"}
	if result := titles(feed); !slices.Equal(result, expected) {
		t.Fatalf("unexpected titles: %q", result)
	}
}

func TestParseKeepsWholeTitlesAcrossInlineMarkup(t *testing.T) {
	feed := parse(t, "https://example.org/blog", `<html><body>
		<a href="/blog/one"><p>North two: enterprise AI <br>without limits</p></a>
		<a href="/blog/two"><p>A title with a <span>highlighted</span> word inside</p></a>
		<a href="/blog/three"><p>Another title that <strong>stays</strong> whole</p></a>
	</body></html>`)

	expected := []string{"North two: enterprise AI without limits", "A title with a highlighted word inside", "Another title that stays whole"}
	if result := titles(feed); !slices.Equal(result, expected) {
		t.Fatalf("unexpected titles: %q", result)
	}
}

func TestParseIgnoresFutureDates(t *testing.T) {
	feed := parse(t, "https://example.org/events", `<html><body>
		<article><a href="/events/one">A workshop in the far future</a><time datetime="2099-01-01"></time></article>
		<article><a href="/events/two">A second post from last year</a><time datetime="2025-01-01"></time></article>
		<article><a href="/events/three">A third post from last year</a><time datetime="2025-01-02"></time></article>
	</body></html>`)

	if feed.Entries[0].Date.Year() == 2099 {
		t.Errorf("expected the future date to be ignored, got %v", feed.Entries[0].Date)
	}
}

func TestPublishedDate(t *testing.T) {
	scenarios := map[string]struct {
		body     string
		expected string
	}{
		"open graph": {`<head><meta property="article:published_time" content="2026-10-26T00:00:00.000Z"></head>`, "2026-10-26"},
		"json-ld":    {`<script type="application/ld+json">{"@type":"Article", "datePublished" : "2026-09-14"}</script>`, "2026-09-14"},
		"time":       {`<article><time datetime="2026-08-03">August</time></article>`, "2026-08-03"},
		"future":     {`<head><meta property="article:published_time" content="2099-01-01"></head>`, "0001-01-01"},
		"nothing":    {`<p>No date anywhere</p>`, "0001-01-01"},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			if result := PublishedDate(pageContentType, []byte(scenario.body)).Format("2006-01-02"); result != scenario.expected {
				t.Errorf("expected %s, got %s", scenario.expected, result)
			}
		})
	}
}

func TestParseIgnoresPagesThatAreNotPosts(t *testing.T) {
	feed := parse(t, "https://example.org/", `<html><body>
		<a href="/contact-sales">Talk to our friendly sales team</a>
		<a href="/api-platform">The same accurate AI for developers</a>
		<a href="/janedoe">Jane Doe Lead Correspondent Science</a>
		<a href="/team/john-smith">John Smith Editor in Chief</a>
		<a href="/how-we-built-this-thing">How we built this thing</a>
		<a href="/what-we-learned-from-it">What we learned from it</a>
		<a href="/where-we-go-next-year">Where we go next year</a>
	</body></html>`)

	expected := []string{"How we built this thing", "What we learned from it", "Where we go next year"}
	if result := titles(feed); !slices.Equal(result, expected) {
		t.Fatalf("unexpected posts: %q", result)
	}
}

func TestParseDropsUndatedLabelsOnDatedPages(t *testing.T) {
	var page strings.Builder
	page.WriteString(`<html><body><a href="/blog/profiles-and-interviews">Profiles &amp; interviews</a>`)
	for day := 1; day <= 8; day++ {
		fmt.Fprintf(&page, `<article><a href="/blog/post-%d">Blog post number %d</a><time datetime="2026-10-%02d"></time></article>`, day, day, day)
	}
	page.WriteString(`<article><a href="/blog/featured">A featured post without any date</a></article></body></html>`)

	feed := parse(t, "https://example.org/blog", page.String())
	result := titles(feed)
	if slices.Contains(result, "Profiles & interviews") {
		t.Errorf("expected the category to be dropped: %q", result)
	}

	if !slices.Contains(result, "A featured post without any date") {
		t.Errorf("expected the undated post to stay: %q", result)
	}
}
