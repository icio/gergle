# :dizzy: gergle

`gergle` is a silly little website-scraping tool, written in Go. By no coincidence, very similar to [`crul`](http://github.com/icio/crul). It will attempt to abide by robots.txt unless you tell it otherwise, spawning a new goroutine for every request being made.


## Installation

```
go get github.com/icio/gergle/cmd/gergle
```


## Usage

```
$ gergle -h
Website crawler.

Usage:
  gergle URL [flags]

Flags:
  -c, --connections int   Maximum number of open connections to the server. (default 5)
  -t, --delay float       The number of seconds between requests to the server. (default -1)
  -d, --depth value       Maximum crawl depth. (default 100)
  -i, --disallow value    Disallowed paths. (default [])
  -q, --quiet             No logging to stderr.
  -v, --verbose           Verbose output logging.
      --zero              The number of bothers to give about robots.txt.
```


## Examples

``` bash
# Crawl paul-scott.com with one second between each page request.
$ gergle http://www.paul-scott.com/ -t 1

# Crawl kirupa.com, excluding /forum*, up to three levels deep (first page is
# depth 0), ignoring robots.txt and using up to 30 simultaneous connections.
# 640 pages in 9 seconds on my local.
$ gergle -q https://www.kirupa.com/ --zero -c 30 -d 3 -iforum
```


## Todo

- [ ] Actual tests -- something beyond [manual testing](https://github.com/icio/crawler-target) :disappointed:
- [ ] Extraction and display of assets
- [ ] Display of links per page
- [ ] First-class tracking of redirects and canonical URLs
- [ ] Vendoring of dependencies
