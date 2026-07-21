package extensions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GitLabConfig struct {
	BaseURL, Token string
	HTTPClient     *http.Client
}
type BitbucketConfig struct {
	BaseURL, Username, Token string
	HTTPClient               *http.Client
}
type GitLabClient struct {
	baseURL, token string
	client         *http.Client
}
type BitbucketClient struct {
	baseURL, username, token string
	client                   *http.Client
}

func NewGitLabClient(c GitLabConfig) (*GitLabClient, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("GitLab token is required")
	}
	return &GitLabClient{strings.TrimRight(c.BaseURL, "/"), c.Token, httpClient(c.HTTPClient)}, nil
}
func NewBitbucketClient(c BitbucketConfig) (*BitbucketClient, error) {
	if c.Username == "" || c.Token == "" {
		return nil, fmt.Errorf("Bitbucket username and token are required")
	}
	return &BitbucketClient{strings.TrimRight(c.BaseURL, "/"), c.Username, c.Token, httpClient(c.HTTPClient)}, nil
}
func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 10 * time.Second}
}
func (c *GitLabClient) GetPullRequest(ctx context.Context, o, r string, n int) (PullRequest, error) {
	var v struct {
		Title, Description, WebURL string
		Author                     struct{ Username string }
		SHA                        string `json:"sha"`
		DiffRefs                   struct {
			BaseSHA string `json:"base_sha"`
		}
	}
	err := c.request(ctx, http.MethodGet, fmt.Sprintf("/projects/%s/merge_requests/%d", url.PathEscape(o+"/"+r), n), nil, &v)
	return PullRequest{Owner: o, Repo: r, Number: n, Title: v.Title, Body: v.Description, Author: v.Author.Username, URL: v.WebURL, HeadSHA: v.SHA, BaseSHA: v.DiffRefs.BaseSHA}, err
}
func (c *GitLabClient) GetPullRequestDiff(ctx context.Context, o, r string, n int) (string, error) {
	var v struct{ Changes []struct{ Diff string } }
	err := c.request(ctx, http.MethodGet, fmt.Sprintf("/projects/%s/merge_requests/%d/changes", url.PathEscape(o+"/"+r), n), nil, &v)
	var b strings.Builder
	for _, x := range v.Changes {
		b.WriteString(x.Diff)
	}
	return b.String(), err
}
func (c *GitLabClient) GetFileContent(ctx context.Context, o, r, p, ref string) ([]byte, error) {
	var raw []byte
	err := c.request(ctx, http.MethodGet, fmt.Sprintf("/projects/%s/repository/files/%s/raw?ref=%s", url.PathEscape(o+"/"+r), url.PathEscape(p), url.QueryEscape(ref)), nil, &raw)
	return raw, err
}
func (c *GitLabClient) PublishCheckRun(ctx context.Context, in CheckRunInput) error {
	state := "failed"
	if in.Conclusion == "success" {
		state = "success"
	}
	return c.request(ctx, http.MethodPost, fmt.Sprintf("/projects/%s/statuses/%s", url.PathEscape(in.RepoOwner+"/"+in.RepoName), in.HeadSHA), map[string]string{"state": state, "name": in.Name, "target_url": in.DetailsURL, "description": in.Summary}, nil)
}
func (c *GitLabClient) request(ctx context.Context, method, path string, payload any, out any) error {
	return request(ctx, c.client, method, c.baseURL+path, payload, out, func(r *http.Request) { r.Header.Set("PRIVATE-TOKEN", c.token) })
}
func (c *BitbucketClient) GetPullRequest(ctx context.Context, o, r string, n int) (PullRequest, error) {
	var v struct {
		Title, Description string
		Links              struct{ HTML struct{ Href string } }
		Author             struct{ Nickname string }
		Source             struct{ Commit struct{ Hash string } }
		Destination        struct{ Commit struct{ Hash string } }
	}
	err := c.request(ctx, http.MethodGet, fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", o, r, n), nil, &v)
	return PullRequest{Owner: o, Repo: r, Number: n, Title: v.Title, Body: v.Description, Author: v.Author.Nickname, URL: v.Links.HTML.Href, HeadSHA: v.Source.Commit.Hash, BaseSHA: v.Destination.Commit.Hash}, err
}
func (c *BitbucketClient) GetPullRequestDiff(ctx context.Context, o, r string, n int) (string, error) {
	var raw []byte
	err := c.request(ctx, http.MethodGet, fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/diff", o, r, n), nil, &raw)
	return string(raw), err
}
func (c *BitbucketClient) GetFileContent(ctx context.Context, o, r, p, ref string) ([]byte, error) {
	var raw []byte
	err := c.request(ctx, http.MethodGet, fmt.Sprintf("/repositories/%s/%s/src/%s/%s", o, r, url.PathEscape(ref), p), nil, &raw)
	return raw, err
}
func (c *BitbucketClient) PublishCheckRun(ctx context.Context, in CheckRunInput) error {
	state := "FAILED"
	if in.Conclusion == "success" {
		state = "SUCCESSFUL"
	}
	return c.request(ctx, http.MethodPost, fmt.Sprintf("/repositories/%s/%s/commit/%s/statuses/build", in.RepoOwner, in.RepoName, in.HeadSHA), map[string]string{"state": state, "key": in.Name, "url": in.DetailsURL, "description": in.Summary}, nil)
}
func (c *BitbucketClient) request(ctx context.Context, m, p string, v any, o any) error {
	return request(ctx, c.client, m, c.baseURL+p, v, o, func(r *http.Request) { r.SetBasicAuth(c.username, c.token) })
}
func request(ctx context.Context, client *http.Client, method, url string, payload, out any, auth func(*http.Request)) error {
	var body io.Reader
	if payload != nil {
		raw, e := json.Marshal(payload)
		if e != nil {
			return e
		}
		body = bytes.NewReader(raw)
	}
	var last error
	for i := 0; i < 2; i++ {
		req, e := http.NewRequestWithContext(ctx, method, url, body)
		if e != nil {
			return e
		}
		auth(req)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, e := client.Do(req)
		if e != nil {
			last = e
			continue
		}
		raw, e := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode >= 500 {
			last = fmt.Errorf("provider status %d", res.StatusCode)
			continue
		}
		if res.StatusCode >= 300 {
			return fmt.Errorf("provider status %d", res.StatusCode)
		}
		if out != nil {
			if target, ok := out.(*[]byte); ok {
				*target = raw
			} else if len(raw) > 0 {
				if e := json.Unmarshal(raw, out); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return last
}
