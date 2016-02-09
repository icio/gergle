package main

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// crawl is the website-crawling loop. It fetches URLs, discovers more, and
// fetches those too, until there are no unseen pages to fetch. This is a
// behemoth of a function which really ought to be broken down into smaller,
// more testable chunks. But later, when it's not 1am.
func crawl(
	client *http.Client, initUrl *url.URL, out chan<- Page,
	follower Follower, delay time.Duration,
) {
	logger.Info(
		"Starting crawl", "url", initUrl, "delay", delay,
	)

	var ticker *time.Ticker
	if delay > 0 {
		ticker = time.NewTicker(delay)
	}

	unexplored := sync.WaitGroup{}
	unexplored.Add(1)

	// Seed the work queue.
	pending := make(chan Task, 100)
	pending <- Task{initUrl, 0}
	links := make(chan *Link, 100)

	// Filter the links channel onto the pending channel.
	go func() {
		// Forward links to the pending queue if we're interested in following.
		for link := range links {
			if err := follower.Follow(link); err != nil {
				logger.Debug("Not following link", "link", link, "reason", err)
				unexplored.Done()
			} else {
				pending <- LinkTask(link)
			}
		}
	}()

	// Request pending, and requeue discovered pages.
	go func() {
		for task := range pending {
			go func(task Task) {
				logger.Debug("Starting", "url", task.URL)
				if ticker != nil {
					<-ticker.C
				}

				resp, err := client.Get(task.URL.String())
				var page Page
				if err != nil {
					page = ErrorPage(task.URL, task.Depth, err)
				} else {
					page = parsePage(task.URL, task.Depth, resp)
					resp.Body.Close()
				}
				out <- page

				for _, link := range page.Links {
					unexplored.Add(1)
					links <- link
				}
				unexplored.Done()
			}(task)
		}
	}()

	// Tie eveything off so that we exit clearly.
	unexplored.Wait()
	if ticker != nil {
		ticker.Stop()
	}
	close(links)
	close(out)
}
