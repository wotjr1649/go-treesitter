package main

// Activity of every pinned grammar repository (information only: nothing here is a warning or a failure).
// One GraphQL query per repository returns the merged pull requests of the last 12 months and the maintainer
// response on the 20 newest issues. GraphQL requires a token, so without GITHUB_TOKEN the section is skipped.
// It runs after every other check, inside a short time budget and stops at the first GraphQL limit, so it can
// never keep the issue body or the state file from being written. Errors are printed as fixed kinds, never as
// remote text.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const activityQuery = `query($o:String!,$n:String!,$mq:String!){
 merged:search(query:$mq,type:ISSUE,first:1){issueCount}
 repository(owner:$o,name:$n){issues(first:20,orderBy:{field:CREATED_AT,direction:DESC}){nodes{comments(first:20){pageInfo{hasNextPage} nodes{authorAssociation}}}}}}`

// activity holds what one response proved; a nil merged or !haveIssues is missing data, not a zero.
type activity struct {
	merged               *int
	replied, issues      int
	haveIssues, moreSeen bool // moreSeen: an issue without a maintainer comment has more comments than were read
}

// graphql posts a query to the API next to the REST client (same cache, headers and token); the token is never printed.
func (a *githubAPI) graphql(ctx context.Context, query string, vars map[string]string, v any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	reply := a.postGraphQL(ctx, body) // activitySection already queries each canonical repository once
	if reply.err != nil {
		return reply.err
	}
	if json.Unmarshal(reply.body, v) != nil {
		return errors.New("unexpected response shape")
	}
	return nil
}

// postGraphQL keeps its own limit state (gqlStop): GraphQL and REST limits are independent.
func (a *githubAPI) postGraphQL(ctx context.Context, body []byte) apiReply {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"graphql", bytes.NewReader(body))
	if err != nil {
		return apiReply{err: errors.New("bad request")}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "upstream-watch")
	req.Header.Set("Authorization", "Bearer "+a.token)
	a.requests++
	resp, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return apiReply{err: errors.New("time budget used")}
		}
		return apiReply{err: errors.New("request failed")}
	}
	defer resp.Body.Close()
	switch {
	case resp.Header.Get("X-RateLimit-Remaining") == "0":
		a.gqlStop = "GraphQL rate limit reached"
	// Secondary limits may answer 403 without either header.
	case resp.Header.Get("Retry-After") != "" || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden:
		a.gqlStop = "GraphQL asked to retry later"
	}
	if resp.StatusCode != http.StatusOK {
		return apiReply{err: &statusError{resp.StatusCode, errors.New("HTTP " + strconv.Itoa(resp.StatusCode))}}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return apiReply{err: errors.New("unreadable response")}
	}
	var env struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return apiReply{err: errors.New("unexpected response shape")}
	}
	if len(env.Errors) > 0 {
		return apiReply{err: errors.New("GraphQL error")}
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return apiReply{err: errors.New("empty data")}
	}
	return apiReply{body: env.Data}
}

func fetchActivity(ctx context.Context, api *githubAPI, repo string) (activity, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return activity{}, errors.New("bad repository name")
	}
	since := api.now.AddDate(-1, 0, 0).Format(time.DateOnly)
	var r struct {
		Merged struct {
			Count *int `json:"issueCount"`
		}
		Repository *struct {
			Issues *struct {
				Nodes []struct {
					Comments struct {
						PageInfo struct {
							More bool `json:"hasNextPage"`
						}
						Nodes []struct {
							Association string `json:"authorAssociation"`
						}
					}
				}
			}
		}
	}
	vars := map[string]string{"o": owner, "n": name, "mq": fmt.Sprintf("repo:%s is:pr is:merged merged:>=%s", repo, since)}
	if err := api.graphql(ctx, activityQuery, vars, &r); err != nil {
		return activity{}, err
	}
	act := activity{merged: r.Merged.Count}
	if r.Repository != nil && r.Repository.Issues != nil {
		act.haveIssues, act.issues = true, len(r.Repository.Issues.Nodes)
		for _, issue := range r.Repository.Issues.Nodes {
			maintainer := false
			for _, c := range issue.Comments.Nodes {
				if c.Association == "OWNER" || c.Association == "MEMBER" || c.Association == "COLLABORATOR" {
					maintainer = true
					break
				}
			}
			if maintainer {
				act.replied++
			} else if issue.Comments.PageInfo.More {
				act.moreSeen = true
			}
		}
	}
	if act.merged == nil && !act.haveIssues {
		return activity{}, errors.New("incomplete data")
	}
	return act, nil
}

// activitySection prints one row per distinct grammar repository. Low values are not warnings: an inactive
// repository already gets the 12-month commit warning, and a reply share over 20 issues is too noisy to alert on.
func activitySection(p *pins, w io.Writer, api *githubAPI) {
	fmt.Fprintln(w, "\n## Grammar repository activity (information only; no warning or failure)")
	fmt.Fprintln(w)
	if api.token == "" {
		fmt.Fprintln(w, "Skipped: the GraphQL API needs GITHUB_TOKEN.")
		return
	}
	byRepo := map[string][]string{}
	for n, g := range p.Grammars {
		repo := canonical(api, repoOf(g.Repo))
		byRepo[repo] = append(byRepo[repo], n)
	}
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
		sort.Strings(byRepo[r])
	}
	sort.Strings(repos)
	fmt.Fprintln(w, "| grammars | repository | merged PRs (12 months) | newest issues with a maintainer comment (first 20 comments read; >= means more comments were not read) |")
	fmt.Fprintln(w, "|---|---|---|---|")
	ctx, cancel := context.WithTimeout(context.Background(), api.activityBudget)
	defer cancel()
	before := api.requests
	for _, repo := range repos {
		var act activity
		var err error
		switch {
		case api.gqlStop != "":
			err = errors.New("skipped, " + api.gqlStop)
		case ctx.Err() != nil:
			err = errors.New("skipped, time budget used")
		default:
			act, err = fetchActivity(ctx, api, repo)
		}
		grammars := strings.Join(byRepo[repo], ", ")
		if err != nil {
			fmt.Fprintf(w, "| %s | %s | ? | unavailable: %v |\n", grammars, repo, err)
			continue
		}
		merged, replies := "?", "?"
		if act.merged != nil {
			merged = strconv.Itoa(*act.merged)
		}
		if act.haveIssues {
			replies = fmt.Sprintf("%d/%d", act.replied, act.issues)
			if act.moreSeen {
				replies = ">=" + replies
			}
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", grammars, repo, merged, replies)
	}
	fmt.Fprintf(w, "\nGraphQL requests: %d. Maintainer = OWNER, MEMBER or COLLABORATOR; the 20 newest issues (pull requests excluded).\n", api.requests-before)
}
