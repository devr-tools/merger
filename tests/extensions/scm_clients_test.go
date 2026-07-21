package extensions_test

import (
	"context"
	"github.com/devr-tools/merger/pkg/extensions"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitLabClientAuthRetryAndStatus(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Error("missing token")
		}
		if calls == 1 {
			w.WriteHeader(502)
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(201)
			return
		}
		_, _ = w.Write([]byte(`{"title":"p","sha":"head","diff_refs":{"base_sha":"base"}}`))
	}))
	defer s.Close()
	c, e := extensions.NewGitLabClient(extensions.GitLabConfig{BaseURL: s.URL, Token: "token"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.GetPullRequest(context.Background(), "team", "repo", 1)
	if e != nil || p.HeadSHA != "head" {
		t.Fatalf("%#v %v", p, e)
	}
	if calls != 2 {
		t.Fatalf("expected retry, got %d", calls)
	}
	if e = c.PublishCheckRun(context.Background(), extensions.CheckRunInput{RepoOwner: "team", RepoName: "repo", HeadSHA: "head", Name: "gate", Conclusion: "success"}); e != nil {
		t.Fatal(e)
	}
}
func TestBitbucketClientAuthDiffAndStatus(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "user" || p != "token" {
			t.Error("missing basic auth")
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(201)
			return
		}
		_, _ = w.Write([]byte("diff --git a/a b/a\n"))
	}))
	defer s.Close()
	c, e := extensions.NewBitbucketClient(extensions.BitbucketConfig{BaseURL: s.URL, Username: "user", Token: "token"})
	if e != nil {
		t.Fatal(e)
	}
	d, e := c.GetPullRequestDiff(context.Background(), "team", "repo", 1)
	if e != nil || d == "" {
		t.Fatalf("%q %v", d, e)
	}
	if e = c.PublishCheckRun(context.Background(), extensions.CheckRunInput{RepoOwner: "team", RepoName: "repo", HeadSHA: "head", Name: "gate", Conclusion: "success"}); e != nil {
		t.Fatal(e)
	}
}
