package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
	log "gopkg.in/inconshreveable/log15.v2"
)

var logger = log.New()

func main() {
	var maxDepth uint16
	var disallow []string
	var quiet bool
	var username string
	var password string
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
	cmd.Flags().StringVarP(&username, "http-user", "u", "", "HTTP authentication username.")
	cmd.Flags().StringVarP(&password, "http-pass", "p", "", "HTTP authentication password.")
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
		client := &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: numConns,
		}}

		if !zeroBothers {
			// Be a good citizen: fetch the target's preferred defaults.
			robots, err := fetchRobots(client, initUrl)
			if err == nil {
				disallow = append(disallow, readDisallowRules(robots)...)
				if delay < 0 {
					delay = readCrawlDelay(robots)
				}
			} else {
				logger.Info("Failed to fetch robots.txt", "error", err)
			}
		}

		var fetcher Fetcher = &HTTPFetcher{client, &RegexPageParser{}, username, password}

		// Rate-limiting.
		if delay > 0 {
			duration := time.Duration(delay * 1e9)
			fetcher = NewRateLimitedFetcher(duration, fetcher)
			logger.Info("Using rate-limiting", "interval", duration)
		}

		// Construct our rules for following links.
		follower := UnanimousFollower{}

		logger.Info("Ignoring external links")
		follower = append(follower, &LocalFollower{})

		if maxDepth >= 0 {
			logger.Info("Ignoring deep links", "maxDepth", maxDepth)
			follower = append(follower, &ShallowFollower{maxDepth})
		}

		if len(disallow) > 0 {
			disallowFollower := NewRobotsDisallowFollower(disallow...)
			logger.Info("Ignoring paths", "disallow", disallowFollower.Rules)
			follower = append(follower, disallowFollower)
		}

		logger.Info("Ignoring previously seen paths")
		follower = append(follower, NewUnseenFollower(initUrl))

		// Crawling.
		pages := make(chan Page, 10)
		go func() {
			crawl(fetcher, initUrl, pages, follower)
			close(pages)
			if stoppable, ok := fetcher.(Stopper); ok {
				stoppable.Stop()
			}
		}()

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
