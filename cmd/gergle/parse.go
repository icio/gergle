package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var robotsTxtDisallowRegex = regexp.MustCompile("(?is)Disallow:\\s*(.+?)(\\s|$)")

// readDisallowRules extracts all of the Disallow directives from a robots.txt body.
func readDisallowRules(body []byte) (rules []string) {
	n := bytes.IndexByte(body, 0)
	for _, rule := range robotsTxtDisallowRegex.FindAllSubmatch(body, n) {
		rules = append(rules, string(rule[1]))
	}
	return
}

var crawlDelayRegex = regexp.MustCompile("(?si)\\s*Crawl-Delay:\\s*([\\d\\.]+)")

// readCrawlDelay parses the first Crawl-Delay directive from a robots.txt body.
func readCrawlDelay(body []byte) float64 {
	delayMatch := crawlDelayRegex.FindSubmatch(body)
	if delayMatch == nil {
		return 0
	}
	delay, err := strconv.ParseFloat(string(delayMatch[1]), 64)
	if err != nil {
		return 0
	}
	return delay
}

// parseDisallowRule transforms a Disallow rule pattern into a regexp.Regexp
func parseDisallowRule(rule string) *regexp.Regexp {
	return regexp.MustCompile("^/?" + strings.Replace(regexp.QuoteMeta(strings.TrimLeft(rule, "/")), "\\*", "*", -1))
}

// parseDisallowRules transforms a slice of Disallow rule patterns into regexp.Regexps.
func parseDisallowRules(rules []string) (regexpRules []*regexp.Regexp) {
	regexpRules = make([]*regexp.Regexp, 0)
	for _, rule := range rules {
		regexpRules = append(regexpRules, parseDisallowRule(rule))
	}
	return
}

// parsePage extracts all of the information from the page that we need to
// perform all of the business logic of the application.
func parsePage(pageUrl *url.URL, depth uint16, resp *http.Response) Page {
	if resp.StatusCode != 200 {
		logger.Debug("Not processing non-200 status code", "url", pageUrl, "status", resp.StatusCode)
		return ErrorPage(pageUrl, depth, errors.New("Non-200 response"))
	}

	mime := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(mime), "html") {
		logger.Debug("Doesn't look like HTML", "url", pageUrl, "content-type", mime)
		return ErrorPage(pageUrl, depth, errors.New("Doesn't look like HTML"))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read body", "url", pageUrl)
		return ErrorPage(pageUrl, depth, err)
	}

	base := parseBase(resp, body)
	return Page{pageUrl, true, depth, parseLinks(base, body, depth+1), nil}
}

var baseRegex = regexp.MustCompile("(?is)<base[^>]+href=[\"']?(.+?)['\"\\s>]")

// parseBase returns the URL which all relative URLs of the given page should be considered relative to.
func parseBase(resp *http.Response, body []byte) *url.URL {
	base := baseRegex.FindSubmatch(body)
	if base != nil {
		baseUrl, err := url.Parse(string(base[1]))
		if err == nil {
			// Use the <base href="..."> from the page body.
			return resp.Request.URL.ResolveReference(baseUrl)
		}
	}

	return resp.Request.URL
}

// Attribution: definitely not http://stackoverflow.com/a/1732454/123600.
var anchorRegex = regexp.MustCompile("(?is)<a[^>]+href=[\"']?(.+?)['\"\\s>]")

// parseLinks returns all of the anchor links on the given page.
func parseLinks(base *url.URL, body []byte, depth uint16) (links []*Link) {
	n := bytes.IndexByte(body, 0)
	for _, anchor := range anchorRegex.FindAllSubmatch(body, n) {
		link, err := FollowLink(string(anchor[1]), base, depth)
		if err != nil {
			logger.Debug("Failed to parse href", "href", anchor[1])
			continue
		}
		links = append(links, link)
	}

	return
}
