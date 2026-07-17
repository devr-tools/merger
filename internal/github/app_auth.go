package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type InstallationTokenSource interface {
	Token(context.Context, int64) (string, error)
}

type AppAuthenticator struct {
	appID      string
	privateKey *rsa.PrivateKey
	client     *Client

	mu     sync.Mutex
	tokens map[int64]installationToken
}

type installationToken struct {
	value     string
	expiresAt time.Time
}

func NewAppAuthenticator(appID, privateKeyPath string, client *Client) (*AppAuthenticator, error) {
	raw, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("failed to decode GitHub app private key PEM")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if pkcs8Err != nil {
			return nil, err
		}

		typedKey, ok := pkcs8Key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("github app private key is not RSA")
		}
		key = typedKey
	}

	return &AppAuthenticator{
		appID:      appID,
		privateKey: key,
		client:     client,
		tokens:     make(map[int64]installationToken),
	}, nil
}

func (a *AppAuthenticator) Token(ctx context.Context, installationID int64) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	cached := a.tokens[installationID]
	if cached.value != "" && time.Now().UTC().Before(cached.expiresAt.Add(-1*time.Minute)) {
		return cached.value, nil
	}

	jwtToken, err := a.appJWT()
	if err != nil {
		return "", err
	}

	body, err := a.client.postWithBearer(
		ctx,
		fmt.Sprintf("/app/installations/%d/access_tokens", installationID),
		jwtToken,
		nil,
	)
	if err != nil {
		return "", err
	}

	var response struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}

	a.tokens[installationID] = installationToken{
		value:     response.Token,
		expiresAt: response.ExpiresAt,
	}
	return response.Token, nil
}

func (a *AppAuthenticator) appJWT() (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	payload := map[string]any{
		"iat": time.Now().UTC().Add(-30 * time.Second).Unix(),
		"exp": time.Now().UTC().Add(9 * time.Minute).Unix(),
		"iss": a.appID,
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	unsigned := encodeSegment(headerBytes) + "." + encodeSegment(payloadBytes)
	hash := sha256.Sum256([]byte(unsigned))

	signature, err := rsa.SignPKCS1v15(rand.Reader, a.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}

	return unsigned + "." + encodeSegment(signature), nil
}

func encodeSegment(raw []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(raw), "=")
}
