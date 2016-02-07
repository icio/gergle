package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

type Page struct {
	URL       string
	Processed bool
	Depth     int
	Links     []*Link
	Error     *error
}

type Link struct {
	URL      string
	External bool
	Depth    int
}

type Task struct {
	URL   string
	Depth int
}

func main() {
	if len(os.Args) < 2 {
		panic("Usage: gergle URL")
	}
	url := os.Args[1]

	pages := make(chan Page, 10)
	go crawl(url, pages)

	for page := range pages {
		fmt.Printf("URL: %s, Depth: %d, Links: %d\n", page.URL, page.Depth, len(page.Links))
	}
}

func sanitizeURL(url string) string {
	// Remove the fragment
	f := strings.Index(url, "#")
	if f != -1 {
		url = url[:f]
	}

	// Remove trailing slashes
	return strings.TrimRight(url, "/")
}

func crawl(url string, out chan<- Page) {
	unexplored := sync.WaitGroup{}

	// Prepare the work queue.
	pending := make(chan Task, 100)
	pending <- URLTask(url)
	unexplored.Add(1)

	links := make(chan *Link, 100)
	go func(init ...string) {
		seen := make(map[string]bool)

		// Mark the URLs queued elsewhere as being seen.
		for _, initUrl := range init {
			seen[sanitizeURL(initUrl)] = true
		}

		for link := range links {
			if link.External {
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
	}(url)

	// Request pending, and requeue discovered pages.
	for w := 0; w < 4; w++ {
		go func() {
			for task := range pending {
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

func URLTask(url string) Task {
	return Task{URL: url, Depth: 0}
}

func LinkTask(link *Link) Task {
	return Task{URL: link.URL, Depth: link.Depth}
}

func ErrorPage(url string, depth int, err error) Page {
	return Page{url, false, depth, []*Link{}, &err}
}

func FollowLink(href string, base *url.URL, depth int) (*Link, error) {
	hrefUrl, err := url.Parse(href)
	if err != nil {
		return nil, err
	}

	abs := base.ResolveReference(hrefUrl)
	return &Link{
		URL:      abs.String(),
		External: abs.Scheme != base.Scheme || abs.Host != base.Host,
		Depth:    depth,
	}, nil
}

func process(task Task) Page {
	resp, err := http.Get(task.URL)
	if err != nil {
		return ErrorPage(task.URL, task.Depth, err)
	}

	defer resp.Body.Close()
	return parsePage(task.URL, task.Depth, resp)
}

func parsePage(url string, depth int, resp *http.Response) Page {
	if resp.StatusCode != 200 {
		return ErrorPage(url, depth, errors.New("Non-200 response"))
	}

	mime := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(mime), "html") {
		return ErrorPage(url, depth, errors.New("Doesn't look like HTML"))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ErrorPage(url, depth, err)
	}

	base := parseBase(resp, body)
	return Page{url, true, depth, parseLinks(base, body, depth+1), nil}
}

func parseBase(resp *http.Response, body []byte) *url.URL {
	return resp.Request.URL // TODO: Look for <base /> tags.
}

// Attribution: definitely not http://stackoverflow.com/a/1732454/123600.
var anchorRegex = regexp.MustCompile("(?is)<a[^>]+href=\"?(.+?)[\"\\s>]")

func parseLinks(base *url.URL, body []byte, depth int) (links []*Link) {
	n := bytes.IndexByte(body, 0)
	for _, anchor := range anchorRegex.FindAllSubmatch(body, n) {
		link, err := FollowLink(string(anchor[1]), base, depth)
		if err != nil {
			continue // TODO: Log that we couldn't parse the link.
		}
		links = append(links, link)
	}

	return
}
