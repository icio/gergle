package main

// TODO: Investigate some of the libraries for properly parsing and finding tags.

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

type ResponsePageParser interface {
	Parse(*Task, *http.Response) Page
}

type RegexPageParser struct {
	baseRegex   regexp.Regexp
	anchorRegex regexp.Regexp
	assetRegex  regexp.Regexp
}

func (r *RegexPageParser) Parse(task *Task, resp *http.Response) Page {
	if resp.StatusCode != 200 {
		logger.Debug("Not processing non-200 status code", "url", task.URL, "status", resp.StatusCode)
		return ErrorPage(task.URL, task.Depth, errors.New("Non-200 response"))
	}

	mime := resp.Header.Get("Content-Type")
	if strings.Split(strings.ToLower(mime), "/")[0] != "text" {
		logger.Debug("'Content-Type' is not text/*", "url", task.URL, "content-type", mime)
		return ErrorPage(task.URL, task.Depth, errors.New("'Content-Type' is not text/*"))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read body", "url", task.URL)
		return ErrorPage(task.URL, task.Depth, err)
	}

	base := r.parseBase(resp, body)
	return Page{
		URL:       task.URL,
		Processed: true,
		Depth:     task.Depth,
		Links:     r.parseLinks(base, body, task.Depth+1),
		Assets:    r.parseAssets(base, body, task.Depth+1),
		Error:     nil,
	}
}

// parseBase returns the URL which all relative URLs of the given page should be considered relative to.
func (r *RegexPageParser) parseBase(resp *http.Response, body []byte) *url.URL {
	base := r.baseRegex.FindSubmatch(body)
	if base != nil {
		baseUrl, err := url.Parse(string(base[1]))
		if err == nil {
			// Use the <base href="..."> from the page body.
			return resp.Request.URL.ResolveReference(baseUrl)
		}
	}

	return resp.Request.URL
}

// parseLinks returns all of the anchor links on the given page.
func (r *RegexPageParser) parseLinks(base *url.URL, body []byte, depth uint16) (links []*Link) {
	n := bytes.IndexByte(body, 0)
	for _, anchor := range r.anchorRegex.FindAllSubmatch(body, n) {
		link, err := AnchorLink(string(anchor[1]), base, depth)
		if err != nil {
			logger.Debug("Failed to parse href", "href", anchor[1])
			continue
		}
		links = append(links, link)
	}

	return
}

func (r *RegexPageParser) parseAssets(base *url.URL, body []byte, depth uint16) (assets []*Link) {
	// TODO: Consider <link>, <object> tags.
	n := bytes.IndexByte(body, 0)
	for _, assetTag := range r.assetRegex.FindAllSubmatch(body, n) {
		asset, err := AssetLink(string(assetTag[1]), string(assetTag[2]), base, depth)
		if err != nil {
			logger.Debug("Failed to parse asset source", "src", assetTag[2])
			continue
		}
		assets = append(assets, asset)
	}

	return
}
