package main

import (
	"bytes"
	"errors"
	"fmt"
	log "gopkg.in/inconshreveable/log15.v2"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

type Page struct {
	URL       *url.URL
	Processed bool
	Depth     int
	Links     []*Link
	Error     *error
}

type Link struct {
	URL      *url.URL
	External bool
	Depth    int
}

type Task struct {
	URL   *url.URL
	Depth int
}

var logger = log.New()

func main() {
	logger.SetHandler(log.StderrHandler)

	if len(os.Args) < 2 {
		panic("Usage: gergle URL")
	}

	url, err := url.Parse(os.Args[1])
	if err != nil {
		panic("Invalid URL")
	}

	if url.Scheme != "http" && url.Scheme != "https" {
		panic("Only http/https URLs are supported.")
	}

	pages := make(chan Page, 10)
	go crawl(url, pages)

	for page := range pages {
		fmt.Printf("URL: %s, Depth: %d, Links: %d\n", page.URL, page.Depth, len(page.Links))
	}
}

func sanitizeURL(u *url.URL) string {
	us := u.String()

	// Remove the fragment
	f := strings.Index(us, "#")
	if f != -1 {
		us = us[:f]
	}

	// Remove trailing slashes
	return strings.TrimRight(us, "/")
}

func crawl(initUrl *url.URL, out chan<- Page) {
	maxDepth := 1
	disallow := []*regexp.Regexp{
		regexp.MustCompile(strings.Replace(regexp.QuoteMeta("/react/*"), "\\*", "*", -1)),
	}

	unexplored := sync.WaitGroup{}
	logger.Info("Starting crawl", "url", initUrl)

	// Prepare the work queue.
	pending := make(chan Task, 100)
	pending <- Task{initUrl, 0}
	unexplored.Add(1)

	follow := func(link *Link) bool {
		if link.External {
			logger.Debug("Skipping external link", "url", link.URL)
			return false
		}
		if link.Depth > maxDepth {
			logger.Debug("Skipping deep link", "url", link.URL, "depth", link.Depth, "maxDepth", maxDepth)
			return false
		}

		for _, disallowRule := range disallow {
			if disallowRule.MatchString(link.URL.Path) {
				logger.Debug("Skipping disallowed link", "url", link.URL, "rule", disallowRule)
				return false
			}
		}

		return true
	}

	links := make(chan *Link, 100)
	go func(init ...*url.URL) {
		seen := make(map[string]bool)

		// Mark the URLs queued elsewhere as being seen.
		for _, initUrl := range init {
			seen[sanitizeURL(initUrl)] = true
		}

		for link := range links {
			if !follow(link) {
				// fmt.Printf("- Skipping external link: %#v\n", link)
				unexplored.Done()
				continue
			}

			sanUrl := sanitizeURL(link.URL)
			_, linkSeen := seen[sanUrl]
			if linkSeen {
				// fmt.Printf("- Skipping seen link: %#v\n", link)
				unexplored.Done()
				continue
			}

			// Queue the link.
			seen[sanUrl] = true
			pending <- LinkTask(link)
		}
	}(initUrl)

	// Request pending, and requeue discovered pages.
	for w := 0; w < 4; w++ {
		go func() {
			for task := range pending {
				// <-ticker
				page := process(task)
				out <- page

				for _, link := range page.Links {
					// fmt.Printf("  Found link %#v\n", link)
					unexplored.Add(1)
					links <- link
				}
				unexplored.Done()
			}
		}()
	}

	unexplored.Wait()
	close(links)
	close(out)
}

func LinkTask(link *Link) Task {
	return Task{URL: link.URL, Depth: link.Depth}
}

func ErrorPage(pageURL *url.URL, depth int, err error) Page {
	return Page{pageURL, false, depth, []*Link{}, &err}
}

func FollowLink(href string, base *url.URL, depth int) (*Link, error) {
	hrefUrl, err := url.Parse(href)
	if err != nil {
		return nil, err
	}

	abs := base.ResolveReference(hrefUrl)
	return &Link{
		URL:      abs,
		External: abs.Scheme != base.Scheme || abs.Host != base.Host,
		Depth:    depth,
	}, nil
}

func process(task Task) Page {
	resp, err := http.Get(task.URL.String())
	if err != nil {
		return ErrorPage(task.URL, task.Depth, err)
	}

	defer resp.Body.Close()
	return parsePage(task.URL, task.Depth, resp)
}

func parsePage(pageUrl *url.URL, depth int, resp *http.Response) Page {
	if resp.StatusCode != 200 {
		logger.Debug("Non-200 response", "url", pageUrl)
		return ErrorPage(pageUrl, depth, errors.New("Non-200 response"))
	}

	mime := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(mime), "html") {
		logger.Debug("Doesn't look like HTML")
		return ErrorPage(pageUrl, depth, errors.New("Doesn't look like HTML"))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read body", "url", pageUrl)
		return ErrorPage(pageUrl, depth, err)
	}

	base := parseBase(resp, body)
	return Page{pageUrl, true, depth, parseLinks(base, body, depth+1), nil}
}

var baseRegex = regexp.MustCompile("(?is)<base[^>]+href=[\"']?(.+?)['\"\\s>]")

func parseBase(resp *http.Response, body []byte) *url.URL {
	base := baseRegex.FindSubmatch(body)
	if base != nil {
		baseUrl, err := url.Parse(string(base[1]))
		if err == nil {
			// Use the <base href="..."> from the page body.
			return resp.Request.URL.ResolveReference(baseUrl)
		}
	}

	return resp.Request.URL
}

// Attribution: definitely not http://stackoverflow.com/a/1732454/123600.
var anchorRegex = regexp.MustCompile("(?is)<a[^>]+href=[\"']?(.+?)['\"\\s>]")

func parseLinks(base *url.URL, body []byte, depth int) (links []*Link) {
	n := bytes.IndexByte(body, 0)
	for _, anchor := range anchorRegex.FindAllSubmatch(body, n) {
		link, err := FollowLink(string(anchor[1]), base, depth)
		if err != nil {
			logger.Debug("Failed to parse href", "href", anchor[1])
			continue
		}
		links = append(links, link)
	}

	return
}
