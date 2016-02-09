package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

type Follower interface {
	Follow(link *Link) error
}

type UnanimousFollower []Follower

func (all UnanimousFollower) Follow(link *Link) error {
	for _, follower := range all {
		if err := follower.Follow(link); err != nil {
			return err
		}
	}
	return nil
}

type LocalFollower struct{}

func (l *LocalFollower) Follow(link *Link) error {
	if link.External {
		return errors.New("Not internal link")
	}
	return nil
}

type ShallowFollower struct {
	MaxDepth uint16
}

func (s *ShallowFollower) Follow(link *Link) error {
	if link.Depth > s.MaxDepth {
		return errors.New(fmt.Sprintf("Link beyond depth %d", s.MaxDepth))
	}
	return nil
}

type UnseenFollower struct {
	seen map[string]bool
	lock sync.RWMutex
}

func NewUnseenFollower(seen ...*url.URL) *UnseenFollower {
	follower := &UnseenFollower{seen: make(map[string]bool, len(seen))}
	for _, u := range seen {
		follower.recordSeen(follower.sanitizeURL(u))
	}
	return follower
}

// sanitizeURL returns a stripped-down string representation of a URL designed
// to maximise overlap of equivalent URLs with slight variations.
func (_ *UnseenFollower) sanitizeURL(u *url.URL) string {
	us := u.String()

	// Remove the fragment.
	f := strings.Index(us, "#")
	if f != -1 {
		us = us[:f]
	}

	// Remove trailing slashes.
	return strings.TrimRight(us, "/")
}

func (u *UnseenFollower) hasSeen(href string) bool {
	u.lock.RLock()
	_, seen := u.seen[href]
	u.lock.RUnlock()
	return seen
}

func (u *UnseenFollower) recordSeen(href string) {
	u.lock.Lock()
	u.seen[href] = true
	u.lock.Unlock()
}

func (u *UnseenFollower) Follow(link *Link) error {
	href := u.sanitizeURL(link.URL)
	if u.hasSeen(href) {
		return errors.New("Not following seen link")
	}

	u.recordSeen(href)
	return nil
}

type RegexpDisallowFollower struct {
	Rules []*regexp.Regexp
}

func (r *RegexpDisallowFollower) Follow(link *Link) error {
	for _, rule := range r.Rules {
		if rule.MatchString(link.URL.Path) {
			return errors.New(fmt.Sprintf("Link disallowed by rule %s", rule))
		}
	}
	return nil
}

func NewRobotsDisallowFollower(disallowRule ...string) *RegexpDisallowFollower {
	follower := &RegexpDisallowFollower{make([]*regexp.Regexp, len(disallowRule))}

	for _, rule := range disallowRule {
		regexpRule, err := regexp.Compile("^/?" + strings.Replace(regexp.QuoteMeta(strings.TrimLeft(rule, "/")), "\\*", "*", -1))
		if err != nil {
			// TODO: Log that we couldn't generate the regex.
			continue
		}

		follower.Rules = append(follower.Rules, regexpRule)
	}

	return follower
}
