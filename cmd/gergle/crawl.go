package main

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Fetcher interface {
	Fetch(*Task) Page
}

type HTTPFetcher struct {
	Client *http.Client
	Parser ResponsePageParser
}

func (h *HTTPFetcher) Fetch(task *Task) Page {
	resp, err := h.Client.Get(task.URL.String())
	if err != nil {
		return ErrorPage(task.URL, task.Depth, err)
	}

	defer resp.Body.Close()
	return h.Parser.Parse(task, resp)
}

type Stopper interface {
	Stop()
}

type RateLimitedFetcher struct {
	ticker  *time.Ticker
	fetcher Fetcher
}

func (r *RateLimitedFetcher) Fetch(task *Task) Page {
	<-r.ticker.C
	return r.fetcher.Fetch(task)
}

func (r *RateLimitedFetcher) Stop() {
	r.ticker.Stop()
}

func NewRateLimitedFetcher(delay time.Duration, fetcher Fetcher) *RateLimitedFetcher {
	return &RateLimitedFetcher{
		ticker:  time.NewTicker(delay),
		fetcher: fetcher,
	}
}

type MockFetcher struct {
	pages map[string]Page
}

func (m *MockFetcher) Fetch(task *Task) Page {
	page, found := m.pages[task.URL.String()]
	if found {
		return page
	}

	// TODO: Switch for a fake 404 response?
	return ErrorPage(task.URL, task.Depth, errors.New("Page not found"))
}

func NewMockFetcher(pages ...Page) *MockFetcher {
	fetcher := &MockFetcher{make(map[string]Page, len(pages))}
	for _, page := range pages {
		fetcher.pages[page.URL.String()] = page
	}
	return fetcher
}

// crawl is the website-crawling loop. It fetches URLs, discovers more, and
// fetches those too, until there are no unseen pages to fetch. This is a
// behemoth of a function which really ought to be broken down into smaller,
// more testable chunks. But later, when it's not 1am.
func crawl(
	fetcher Fetcher, initUrl *url.URL, out chan<- Page, follower Follower,
) {
	logger.Info("Starting crawl", "url", initUrl)

	unexplored := sync.WaitGroup{}
	unexplored.Add(1)

	// Seed the work queue.
	pending := make(chan Task, 100)
	pending <- Task{initUrl, 0}

	// Request pending, and requeue discovered pages.
	go func() {
		for task := range pending {
			go func(task Task) {
				logger.Debug("Starting", "url", task.URL)
				page := fetcher.Fetch(&task)
				out <- page

				for _, link := range page.Links {
					if err := follower.Follow(link); err != nil {
						logger.Debug("Not following link", "link", link, "reason", err)
					} else {
						unexplored.Add(1)
						pending <- LinkTask(link)
					}
				}
				unexplored.Done()
			}(task)
		}
	}()

	// Tie eveything off so that we exit clearly.
	unexplored.Wait()
	close(pending)
}
