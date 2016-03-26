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
	Client   *http.Client
	Parser   ResponsePageParser
	Username string
	Password string
}

func (h *HTTPFetcher) Fetch(task *Task) Page {
	req, err := http.NewRequest("GET", task.URL.String(), nil)

	if h.Username != "" || h.Password != "" {
		req.SetBasicAuth(h.Username, h.Password)
	}

	resp, err := h.Client.Do(req)
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
