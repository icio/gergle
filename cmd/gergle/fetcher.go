package main

import (
	"errors"
	"net/http"
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
