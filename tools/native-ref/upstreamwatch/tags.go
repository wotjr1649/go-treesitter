package main

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

// ownerOrg owns the owner repositories of ADR-0024 rule 7. Only they get grammar warnings for a pin that trails the
// repository: the owner moves those pins (AGENTS.md); for third-party repositories the distance is information.
const ownerOrg = "wotjr1649"

func isOwner(repo string) bool { return strings.HasPrefix(strings.ToLower(repo), ownerOrg+"/") }

var (
	prerelease = regexp.MustCompile(`(?i)[-_.](alpha|beta|rc|pre|preview|dev|nightly)([-_.0-9]|$)`)
	tagPrefix  = regexp.MustCompile(`^\D*`)
)

// newestTag returns the highest version among the repository's tags that start with the non-digit prefix of like
// followed by a digit, skipping pre-release names; "" when there is none. It reads the tags API, so it also sees a
// tag that has no GitHub Release. shortcut: at most 500 tags (5 pages) are read; switch to git ls-remote --tags
// (nvim-treesitter's method) when a repository has more.
func newestTag(api *githubAPI, repo, like string) (string, error) {
	prefix := tagPrefix.FindString(like)
	best := ""
	for page := 1; page <= 5; page++ {
		var tags []struct {
			Name string `json:"name"`
		}
		if err := api.get(fmt.Sprintf("repos/%s/tags?per_page=100&page=%d", repo, page), &tags); err != nil {
			return "", err
		}
		for _, t := range tags {
			rest, ok := strings.CutPrefix(t.Name, prefix)
			if !ok || rest == "" || rest[0] < '0' || rest[0] > '9' || prerelease.MatchString(t.Name) {
				continue
			}
			if best == "" || newer(best, t.Name) {
				best = t.Name
			}
		}
		if len(tags) < 100 {
			break
		}
	}
	return best, nil
}

// pinBehind reports how far the pin trails the default branch. The comparison path is the one pinReachability uses, so
// the request is shared. Only an owner repository gets a warning.
func pinBehind(api *githubAPI, w io.Writer, base string, s source, branch, head string, health *[]string) {
	var cmp struct {
		Status  string `json:"status"`
		AheadBy int    `json:"ahead_by"`
	}
	if err := api.get(base+"/compare/"+url.PathEscape(s.pin)+"..."+url.PathEscape(head)+"?per_page=1", &cmp); err != nil || cmp.Status != "ahead" {
		return // identical, diverged (a release-branch pin) or unreadable: pinReachability reports what matters
	}
	text := fmt.Sprintf("pin is %d commit(s) behind %s", cmp.AheadBy, branch)
	if !isOwner(s.repo) {
		*health = append(*health, text)
		return
	}
	*health = append(*health, "WARNING "+text)
	api.warn(w, s.name, "pin behind default branch", text)
}

// tagOnlyRelease warns when an owner repository has a newer tag than its latest release (or no release at all) that
// contains the pin: the owner's release flow tags first.
func tagOnlyRelease(api *githubAPI, w io.Writer, base string, s source, release string, health *[]string, failed *bool) {
	tag, err := newestTag(api, strings.TrimPrefix(base, "repos/"), "v")
	if err != nil {
		*health = append(*health, "ERROR tags: "+err.Error())
		*failed = true
		return
	}
	if tag == "" || tag == release || (release != "" && !newer(release, tag)) {
		return
	}
	var cmp struct {
		Status string `json:"status"`
	}
	if err := api.get(base+"/compare/"+url.PathEscape(s.pin)+"..."+url.PathEscape(tag)+"?per_page=1", &cmp); err == nil && cmp.Status == "ahead" {
		*health = append(*health, "WARNING newer tag "+tag+" without a release")
		api.warn(w, s.name, "newer tag "+tag+" than pin", fmt.Sprintf("newer tag %s (no release) than pin %.12s", tag, s.pin))
	}
}
