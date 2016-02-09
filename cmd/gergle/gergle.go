package main

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	log "gopkg.in/inconshreveable/log15.v2"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

var logger = log.New()

func main() {
	var maxDepth uint16
	var disallow []string
	var quiet bool
	var verbose bool
	var numConns int
	var zeroBothers bool
	var delay float64
	var longOutput bool

	cmd := &cobra.Command{
		Use:   "gergle URL",
		Short: "Website crawler.",
	}
	cmd.Flags().Uint16VarP(&maxDepth, "depth", "d", 100, "Maximum crawl depth.")
	cmd.Flags().StringSliceVarP(&disallow, "disallow", "i", nil, "Disallowed paths.")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "No logging to stderr.")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output logging.")
	cmd.Flags().IntVarP(&numConns, "connections", "c", 5, "Maximum number of open connections to the server.")
	cmd.Flags().BoolVarP(&zeroBothers, "zero", "", false, "The number of bothers to give about robots.txt. ")
	cmd.Flags().Float64VarP(&delay, "delay", "t", -1, "The number of seconds between requests to the server.")
	cmd.Flags().BoolVarP(&longOutput, "long", "", false, "List all of the links and assets from a page.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Configure logging.
		var logLevel log.Lvl
		if verbose && quiet {
			return errors.New("--verbose and --quiet are mutually exclusive options.")
		} else if verbose {
			logLevel = log.LvlDebug
		} else if quiet {
			logLevel = log.LvlCrit
		} else {
			logLevel = log.LvlInfo
		}
		logger.SetHandler(log.LvlFilterHandler(logLevel, log.StderrHandler))

		// Ensure the user provides only a single URL.
		if len(args) < 1 {
			return errors.New("URL argument required.")
		} else if len(args) > 1 {
			return errors.New("Unexpected arguments after URL.")
		}

		// Ensure the user has provided a valid URL.
		initUrl, err := url.Parse(args[0])
		if err != nil || (initUrl.Scheme != "http" && initUrl.Scheme != "https") {
			return errors.New("Expected URL of the form http[s]://...")
		}

		// Prepare the HTTP Client with a series of connections.
		fetcher := NewHTTPFetcher(&RegexPageParser{}, numConns)

		if !zeroBothers {
			// Be a good citizen: fetch the target's preferred defaults.
			robots, err := fetchRobots(fetcher.Client, initUrl)
			if err == nil {
				disallow = append(disallow, readDisallowRules(robots)...)
				if delay < 0 {
					delay = readCrawlDelay(robots)
				}
			} else {
				logger.Info("Failed to fetch robots.txt", "error", err)
			}
		}

		// Rate-limiting.
		var ticker *time.Ticker
		if delay > 0 {
			ticker = time.NewTicker(time.Duration(delay * 1e9))
		}

		follower := UnanimousFollower{
			&LocalFollower{},
			&ShallowFollower{maxDepth},
			NewRobotsDisallowFollower(disallow...),
			NewUnseenFollower(initUrl),
		}

		// Crawling.
		pages := make(chan Page, 10)
		go crawl(fetcher, initUrl, pages, follower, ticker)

		// Output.
		for page := range pages {
			fmt.Printf("URL: %s, Depth: %d, Links: %d, Assets: %d\n", page.URL, page.Depth, len(page.Links), len(page.Assets))
			if longOutput {
				for _, link := range page.Links {
					fmt.Printf("- %s: %s\n", link.Type, link.URL)
				}
				for _, link := range page.Assets {
					fmt.Printf("- %s: %s\n", link.Type, link.URL)
				}
			}
		}

		return nil
	}

	cmd.Execute()
}

// fetchRobots gets the body of robots.txt pertaining to the given URL.
func fetchRobots(client *http.Client, u *url.URL) ([]byte, error) {
	robotsPath, _ := url.Parse("/robots.txt")
	robotsUrl := u.ResolveReference(robotsPath).String()
	logger.Info("Fetching robots.txt", "url", robotsUrl)

	resp, err := client.Get(robotsUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("robots.txt not found (%d)", resp.StatusCode))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// A pending Task for crawl workers to complete.
type Task struct {
	URL   *url.URL
	Depth uint16
}

// The Task for following a Link.
func LinkTask(link *Link) Task {
	return Task{URL: link.URL, Depth: link.Depth}
}

// A Page the crawler has scraped and parsed.
type Page struct {
	URL       *url.URL
	Processed bool
	Depth     uint16
	Links     []*Link
	Assets    []*Link
	Error     *error
}

func ErrorPage(pageURL *url.URL, depth uint16, err error) Page {
	return Page{pageURL, false, depth, []*Link{}, []*Link{}, &err}
}

// A link on a page to another resource.
type Link struct {
	Type     string
	URL      *url.URL
	External bool
	Depth    uint16
}

// AnchorLink returns a Link object from an <a> href, according to the base URL.
func AnchorLink(href string, base *url.URL, depth uint16) (*Link, error) {
	return AssetLink("anchor", href, base, depth)
}

// AssetLink returns a Link object describing a Page's dependency on another resource.
func AssetLink(assetType string, href string, base *url.URL, depth uint16) (*Link, error) {
	hrefUrl, err := url.Parse(href)
	if err != nil {
		return nil, err
	}

	abs := base.ResolveReference(hrefUrl)
	return &Link{
		Type:     assetType,
		URL:      abs,
		External: abs.Scheme != base.Scheme || abs.Host != base.Host,
		Depth:    depth,
	}, nil
}
