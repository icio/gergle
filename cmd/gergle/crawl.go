package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// sanitizeURL returns a stripped-down string representation of a URL designed
// to maximise overlap of equivalent URLs with slight variations.
func sanitizeURL(u *url.URL) string {
	us := u.String()

	// Remove the fragment.
	f := strings.Index(us, "#")
	if f != -1 {
		us = us[:f]
	}

	// Remove trailing slashes.
	return strings.TrimRight(us, "/")
}

// crawl is the website-crawling loop. It fetches URLs, discovers more, and
// fetches those too, until there are no unseen pages to fetch. This is a
// behemoth of a function which really ought to be broken down into smaller,
// more testable chunks. But later, when it's not 1am.
func crawl(
	client *http.Client, initUrl *url.URL, out chan<- Page, maxDepth uint16,
	disallow []*regexp.Regexp, numWorkers uint16, delay time.Duration,
) {
	logger.Info(
		"Starting crawl", "url", initUrl, "delay", delay, "maxDepth",
		maxDepth, "disallow", disallow, "workers", numWorkers,
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

	// follow returns whether a link should be requeued for further processing,
	// according to the depth of the page traversal, whether it's a link to
	// another site, or whether the link is to a disallowed page.
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

	// Filter the links channel onto the pending channel.
	go func(init ...*url.URL) {
		seen := make(map[string]bool)

		// Mark the URLs queued elsewhere as having been seen.
		for _, initUrl := range init {
			seen[sanitizeURL(initUrl)] = true
		}

		// Forward links to the pending queue if we're interested in following.
		for link := range links {
			if !follow(link) {
				unexplored.Done()
				continue
			}

			sanUrl := sanitizeURL(link.URL)
			_, linkSeen := seen[sanUrl]
			if linkSeen {
				unexplored.Done()
				continue
			}

			// Queue the link.
			seen[sanUrl] = true
			pending <- LinkTask(link)
		}
	}(initUrl)

	// Request pending, and requeue discovered pages.
	for w := uint16(0); w < numWorkers; w++ {
		go func() {
			for task := range pending {
				if ticker != nil {
					<-ticker.C
				}

				resp, err := client.Get(task.URL.String())
				var page Page
				if err != nil {
					page = ErrorPage(task.URL, task.Depth, err)
				} else {
					page = parsePage(task.URL, task.Depth, resp)
				}
				resp.Body.Close()
				out <- page

				for _, link := range page.Links {
					unexplored.Add(1)
					links <- link
				}
				unexplored.Done()
			}
		}()
	}

	// Tie eveything off so that we exit clearly.
	unexplored.Wait()
	if ticker != nil {
		ticker.Stop()
	}
	close(links)
	close(out)
}
