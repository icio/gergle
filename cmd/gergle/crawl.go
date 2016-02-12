package main

import (
	"net/url"
	"sync"
)

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
