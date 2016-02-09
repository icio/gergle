package main

import (
	"net/url"
)

// A pending Task for crawl workers to complete.
type Task struct {
	URL   *url.URL
	Depth uint16
}

// The Task for following a Link.
func LinkTask(link *Link) Task {
	return Task{URL: link.URL, Depth: link.Depth}
}

// A Page the crawler has scraped and parsed.
type Page struct {
	URL       *url.URL
	Processed bool
	Depth     uint16
	Links     []*Link
	Assets    []*Link
	Error     *error
}

func ErrorPage(pageURL *url.URL, depth uint16, err error) Page {
	return Page{pageURL, false, depth, []*Link{}, []*Link{}, &err}
}

// A link on a page to another resource.
type Link struct {
	Type     string
	URL      *url.URL
	External bool
	Depth    uint16
}

// AnchorLink returns a Link object from an <a> href, according to the base URL.
func AnchorLink(href string, base *url.URL, depth uint16) (*Link, error) {
	return AssetLink("anchor", href, base, depth)
}

// AssetLink returns a Link object describing a Page's dependency on another resource.
func AssetLink(assetType string, href string, base *url.URL, depth uint16) (*Link, error) {
	hrefUrl, err := url.Parse(href)
	if err != nil {
		return nil, err
	}

	abs := base.ResolveReference(hrefUrl)
	return &Link{
		Type:     assetType,
		URL:      abs,
		External: abs.Scheme != base.Scheme || abs.Host != base.Host,
		Depth:    depth,
	}, nil
}
