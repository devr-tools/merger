package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Webhook struct {
	Event        string
	DeliveryID   string
	Signature256 string
	// Payload remains the pull_request payload for backwards compatibility with
	// existing webhook consumers. CheckRun is populated for check_run events.
	Payload  PullRequestWebhookPayload
	CheckRun *CheckRunWebhookPayload
}

type PullRequestWebhookPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	PullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
	} `json:"pull_request"`
}

// CheckRunWebhookPayload is the subset of a GitHub check_run webhook needed to
// associate a completed check with a pull request and its head commit.
type CheckRunWebhookPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	CheckRun struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		HeadSHA    string `json:"head_sha"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		DetailsURL string `json:"details_url"`
		Output     struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
		} `json:"output"`
		App struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"app"`
		PullRequests []struct {
			Number int `json:"number"`
		} `json:"pull_requests"`
	} `json:"check_run"`
}

type WebhookDecoder struct {
	secret string
}

func NewWebhookDecoder(secret string) WebhookDecoder {
	return WebhookDecoder{secret: secret}
}

func (d WebhookDecoder) Decode(r *http.Request) (Webhook, error) {
	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		return Webhook{}, errors.New("missing GitHub event header")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Webhook{}, err
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if err := d.verify(body, signature); err != nil {
		return Webhook{}, err
	}

	webhook := Webhook{
		Event:        event,
		DeliveryID:   r.Header.Get("X-GitHub-Delivery"),
		Signature256: signature,
	}

	switch event {
	case "check_run":
		var payload CheckRunWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return Webhook{}, err
		}
		webhook.CheckRun = &payload
	default:
		var payload PullRequestWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return Webhook{}, err
		}
		webhook.Payload = payload
	}

	return webhook, nil
}

func (d WebhookDecoder) verify(payload []byte, signature string) error {
	if d.secret == "" {
		return nil
	}
	if signature == "" {
		return errors.New("missing GitHub signature header")
	}
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("unsupported GitHub signature format")
	}

	expectedMAC := hmac.New(sha256.New, []byte(d.secret))
	expectedMAC.Write(payload)
	expected := "sha256=" + hex.EncodeToString(expectedMAC.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("invalid GitHub webhook signature")
	}

	return nil
}
