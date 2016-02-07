package main

import (
	"fmt"
	"os"
	"sync"
)

type Page struct {
	URL   string
	HTML  bool
	Depth int
	Links []Link
}

type Link struct {
	URL      string
	External bool
}

type Task struct {
	URL string
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
			for _, link := range page.Links {
				if !link.External {
					pending <- LinkTask(link)
					tasks.Add(1)
				}
			}
			out <- page
			tasks.Done()
		}
	}()

	tasks.Wait()
	close(out)
}

func URLTask(url string) Task {
	return Task{URL: url}
}

func LinkTask(link Link) Task {
	return URLTask(link.URL)
}

func process(task Task) Page {
	return Page{task.URL, true, 0, []Link{}}
}
