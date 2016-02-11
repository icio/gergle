package main

import (
	"net/url"
	"testing"
)

func TestAlwaysFollow(t *testing.T) {
	f := &AlwaysFollow{}
	if f.Follow(nil) != nil {
		t.Error("AlwaysFollow.Follow should never return an error.")
	}
}

func TestNeverFollow(t *testing.T) {
	f := &NeverFollow{}
	if f.Follow(nil) == nil {
		t.Error("NeverFollow.Follow should always return an error.")
	}
}

func TestUnanimousFollower(t *testing.T) {
	ye := &AlwaysFollow{}
	no := &NeverFollow{}

	results := []struct {
		f  *UnanimousFollower
		ok bool // Whether _no_ error should be returned.
	}{
		{&UnanimousFollower{}, true},
		{&UnanimousFollower{ye}, true},
		{&UnanimousFollower{ye, ye}, true},
		{&UnanimousFollower{ye, ye, ye}, true},

		{&UnanimousFollower{no}, false},
		{&UnanimousFollower{no, no}, false},
		{&UnanimousFollower{no, no, no}, false},

		{&UnanimousFollower{no, ye}, false},
		{&UnanimousFollower{ye, no}, false},
		{&UnanimousFollower{no, ye, ye}, false},
		{&UnanimousFollower{ye, no, ye}, false},
	}

	for _, test := range results {
		if (test.f.Follow(nil) == nil) != test.ok {
			if test.ok {
				t.Errorf("%#v.Follow(nil) should not return error.", test.f)
			} else {
				t.Errorf("%#v.Follow(nil) should have returned an error, but did not.", test.f)
			}
		}
	}
}

func TestLocalFollower(t *testing.T) {
	f := LocalFollower{}
	if f.Follow(&Link{External: true}) == nil {
		t.Error("LocalFollower.Follow should return an error when link is external.")
	}
	if f.Follow(&Link{External: false}) != nil {
		t.Error("LocalFollower.Follow should not return an error when link is not external.")
	}
}

func TestShallowFollower(t *testing.T) {
	f := &ShallowFollower{MaxDepth: 10}

	if f.Follow(&Link{Depth: 9}) != nil {
		t.Error("ShallowFollower.Follow should not return an error for depths less than its MaxDepth.")
	}
	if f.Follow(&Link{Depth: 10}) != nil {
		t.Error("ShallowFollower.Follow should not return an error for depths equal to its MaxDepth.")
	}
	if f.Follow(&Link{Depth: 11}) == nil {
		t.Error("ShallowFollower.Follow should return an error for depths greater than its MaxDepth.")
	}
}

func TestUnseenFollower(t *testing.T) {
	f := NewUnseenFollower(&url.URL{Path: "/seen"})

	if f.Follow(&Link{URL: &url.URL{Path: "/seen"}}) == nil {
		t.Error("UnseenFollower.Follow should return an error for URLs it was instantiated with.")
	}
	if f.Follow(&Link{URL: &url.URL{Path: "/seen/"}}) == nil {
		t.Error("UnseenFollower.Follow should return an error for URLs probably the same as other it's already seen.")
	}
	if f.Follow(&Link{URL: &url.URL{Path: "/seen", Fragment: "#irrelevant"}}) == nil {
		t.Error("UnseenFollower.Follow should return an error for URLs only differing in fragment from another it's already seen.")
	}

	if f.Follow(&Link{URL: &url.URL{Path: "/unseen/1"}}) != nil {
		t.Error("UnseenFollower.Follow should not return an error for URLs previously unseen.")
	}
	if f.Follow(&Link{URL: &url.URL{Path: "/unseen/1"}}) == nil {
		t.Error("UnseenFollower.Follow should return an error for URLs it's been asked about previously.")
	}
	if f.Follow(&Link{URL: &url.URL{Path: "/unseen/1", Fragment: "ignored"}}) == nil {
		t.Error("UnseenFollower.Follow should return an error for URLs it's been asked about previously, even if they differ in fragment.")
	}

	f.Follow(&Link{URL: &url.URL{Path: "/unseen/2", Fragment: "ignored"}})
	if f.Follow(&Link{URL: &url.URL{Path: "/unseen/2"}}) == nil {
		t.Error("UnseenFollower.Follow should return an error for URLs it's previously seen with a fragment.")
	}
}

func TestRegexpDisallowFollower(t *testing.T) {
	f := NewRobotsDisallowFollower("/hel.lo", "hello/*/world")

	expRules := []string{
		"^/?hel\\.lo",
		"^/?hello/.*/world",
	}
	if len(f.Rules) != len(expRules) {
		t.Errorf("Expected %d rules but found %d", len(expRules), len(f.Rules))
	} else {
		for i, rule := range f.Rules {
			if rule.String() != expRules[i] {
				t.Errorf("Expected rule %s but got %s", expRules[i], rule)
			}
		}
	}

	if f.Follow(&Link{URL: &url.URL{Path: "hello/asdf/world"}}) == nil {
		t.Error("RegexpDisallowFollower should disallow.")
	}
	if f.Follow(&Link{URL: &url.URL{Path: "hel.lo"}}) == nil {
		t.Error("RegexpDisallowFollower should disallow.")
	}
	if f.Follow(&Link{URL: &url.URL{Path: "goodbye"}}) != nil {
		t.Error("RegexpDisallowFollower should allow.")
	}
}
