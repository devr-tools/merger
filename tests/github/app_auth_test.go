package github_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/devr-tools/merger/internal/github"
)

func TestClientCachesTokensByInstallation(t *testing.T) {
	var mu sync.Mutex
	tokenRequests := make(map[string]int)
	requestTokens := make(map[string][]string)

	client := newTestClient(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/app/installations/101/access_tokens":
			tokenRequests[r.URL.Path]++
			return jsonResponse(t, map[string]any{
				"token":      "installation-101-token",
				"expires_at": "2099-01-01T00:00:00Z",
			}), nil
		case "/app/installations/202/access_tokens":
			tokenRequests[r.URL.Path]++
			return jsonResponse(t, map[string]any{
				"token":      "installation-202-token",
				"expires_at": "2099-01-01T00:00:00Z",
			}), nil
		case "/repos/acme/installation-101/check-runs":
			requestTokens[r.URL.Path] = append(requestTokens[r.URL.Path], r.Header.Get("Authorization"))
			return jsonResponse(t, map[string]string{"id": "101"}), nil
		case "/repos/acme/installation-202/check-runs":
			requestTokens[r.URL.Path] = append(requestTokens[r.URL.Path], r.Header.Get("Authorization"))
			return jsonResponse(t, map[string]string{"id": "202"}), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil
		}
	}))

	installations := []struct {
		id   int64
		repo string
	}{
		{id: 101, repo: "installation-101"},
		{id: 202, repo: "installation-202"},
		{id: 101, repo: "installation-101"},
		{id: 202, repo: "installation-202"},
	}
	for _, installation := range installations {
		service := client.ForInstallation(installation.id)
		err := service.PublishCheckRun(context.Background(), checkRunInput(installation.repo))
		if err != nil {
			t.Fatalf("publish check run for installation %d: %v", installation.id, err)
		}
	}

	for _, id := range []int64{101, 202} {
		tokenPath := fmt.Sprintf("/app/installations/%d/access_tokens", id)
		if got := tokenRequests[tokenPath]; got != 1 {
			t.Fatalf("unexpected token requests for installation %d: got %d want 1", id, got)
		}

		repoPath := fmt.Sprintf("/repos/acme/installation-%d/check-runs", id)
		want := fmt.Sprintf("Bearer installation-%d-token", id)
		if got := len(requestTokens[repoPath]); got != 2 {
			t.Fatalf("unexpected check run requests for installation %d: got %d want 2", id, got)
		}
		for requestIndex, got := range requestTokens[repoPath] {
			if got != want {
				t.Fatalf("unexpected authorization for installation %d request %d: got %q want %q", id, requestIndex, got, want)
			}
		}
	}
}

func checkRunInput(repo string) github.CheckRunInput {
	return github.CheckRunInput{
		RepoOwner:  "acme",
		RepoName:   repo,
		HeadSHA:    "abc123",
		Name:       "merger/risk",
		Status:     "completed",
		Conclusion: "success",
		Summary:    "Low blast radius",
	}
}
