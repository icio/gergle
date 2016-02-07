package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

type Page struct {
	URL       string
	Processed bool
	Depth     int
	Links     []Link
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
		fmt.Printf("%#v\n", page)
	}
}

func crawl(url string, out chan<- Page) {
	tasks := sync.WaitGroup{}

	// Prepare the work queue.
	pending := make(chan Task, 100)
	pending <- URLTask(url)
	tasks.Add(1)

	// Request pending, and requeue discovered pages.
	go func() {
		for task := range pending {
			page := process(task)
			out <- page

			if page.Error == nil {
				for _, link := range page.Links {
					if !link.External {
						// TODO: Don't requeue duplicates.
						pending <- LinkTask(link)
						tasks.Add(1)
					}
				}
			}
			tasks.Done()
		}
	}()

	tasks.Wait()
	close(out)
}

func URLTask(url string) Task {
	return Task{URL: url, Depth: 0}
}

func LinkTask(link Link) Task {
	return Task{URL: link.URL, Depth: link.Depth}
}

func ErrorPage(url string, depth int, err error) Page {
	return Page{url, false, depth, []Link{}, &err}
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
	return Page{url, true, depth, parseLinks(base, body), nil}
}

func parseBase(resp *http.Response, body []byte) *url.URL {
	return resp.Request.URL // TODO: Look for <base /> tags.
}

func parseLinks(base *url.URL, body []byte) []Link {
	fmt.Printf("%s\n", body)
	return []Link{}
}
