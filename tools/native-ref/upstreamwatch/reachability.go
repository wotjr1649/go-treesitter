package main

import (
	"fmt"
	"net/http"
	"net/url"
)

// A release-branch pin can be absent from the default branch without being orphaned.
func pinReachability(api *githubAPI, base, pin, branch, head, release string) (string, error) {
	contains := func(ref string) (bool, error) {
		var cmp struct {
			Status string `json:"status"`
		}
		err := api.get(base+"/compare/"+url.PathEscape(pin)+"..."+url.PathEscape(ref)+"?per_page=1", &cmp)
		if isStatus(err, http.StatusNotFound) {
			return false, nil
		} // no common ancestor or missing commit
		if err != nil {
			return false, err
		}
		switch cmp.Status {
		case "identical", "ahead":
			return true, nil
		case "behind", "diverged":
			return false, nil
		default:
			return false, fmt.Errorf("unexpected comparison status %q", cmp.Status)
		}
	}
	if ok, err := contains(head); err != nil {
		return "", err
	} else if ok {
		return "reachable from default branch " + branch, nil
	}
	// The newest published release is already known and often proves a release pin without listing refs.
	if release != "" {
		if ok, err := contains(release); err != nil {
			return "", err
		} else if ok {
			return "reachable from release tag " + release, nil
		}
	}
	for _, kind := range []string{"branches", "tags"} {
		for page := 1; ; page++ {
			var refs []struct {
				Name   string `json:"name"`
				Commit struct {
					SHA string `json:"sha"`
				} `json:"commit"`
			}
			if err := api.get(fmt.Sprintf("%s/%s?per_page=100&page=%d", base, kind, page), &refs); err != nil {
				return "", err
			}
			for _, ref := range refs {
				if ref.Name == "" || ref.Commit.SHA == "" {
					return "", fmt.Errorf("%s returned an incomplete ref", kind)
				}
				if ok, err := contains(ref.Commit.SHA); err != nil {
					return "", err
				} else if ok {
					if kind == "branches" {
						return "reachable from branch " + ref.Name, nil
					}
					return "reachable from release tag " + ref.Name, nil
				}
			}
			if len(refs) < 100 {
				break
			}
		}
	}
	return "", nil
}
