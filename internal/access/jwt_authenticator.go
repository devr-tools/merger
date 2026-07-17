package access

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	JWTAlgorithmHS256 = "HS256"
	JWTAlgorithmRS256 = "RS256"
)

type JWTConfig struct {
	Algorithm     string
	Issuer        string
	Audience      string
	SubjectClaim  string
	RolesClaim    string
	SecretEnv     string
	PublicKeyPath string
	RoleBindings  []JWTClaimBinding
}

type JWTClaimBinding struct {
	ClaimValue string
	Roles      []Role
}

type jwtVerifier interface {
	Verify(signingInput, signature []byte) error
}

type hmacSHA256Verifier struct {
	secret []byte
}

func (v hmacSHA256Verifier) Verify(signingInput, signature []byte) error {
	mac := hmac.New(sha256.New, v.secret)
	if _, err := mac.Write(signingInput); err != nil {
		return err
	}
	expected := mac.Sum(nil)
	if !hmac.Equal(signature, expected) {
		return ErrUnauthenticated
	}
	return nil
}

type rsaSHA256Verifier struct {
	key *rsa.PublicKey
}

func (v rsaSHA256Verifier) Verify(signingInput, signature []byte) error {
	sum := sha256.Sum256(signingInput)
	if err := rsa.VerifyPKCS1v15(v.key, crypto.SHA256, sum[:], signature); err != nil {
		return ErrUnauthenticated
	}
	return nil
}

type JWTAuthenticator struct {
	algorithm    string
	issuer       string
	audience     string
	subjectClaim string
	rolesClaim   string
	bindings     map[string][]Role
	verifier     jwtVerifier
	now          func() time.Time
}

func NewJWTAuthenticator(cfg JWTConfig) (*JWTAuthenticator, error) {
	algorithm := strings.ToUpper(strings.TrimSpace(cfg.Algorithm))
	if algorithm != JWTAlgorithmHS256 && algorithm != JWTAlgorithmRS256 {
		return nil, fmt.Errorf("unsupported jwt algorithm %q", cfg.Algorithm)
	}
	issuer := strings.TrimSpace(cfg.Issuer)
	if issuer == "" {
		return nil, fmt.Errorf("jwt issuer must not be empty")
	}
	audience := strings.TrimSpace(cfg.Audience)
	if audience == "" {
		return nil, fmt.Errorf("jwt audience must not be empty")
	}

	bindings := make(map[string][]Role, len(cfg.RoleBindings))
	for index, binding := range cfg.RoleBindings {
		claimValue := strings.TrimSpace(binding.ClaimValue)
		if claimValue == "" {
			return nil, fmt.Errorf("jwt role binding %d has an empty claim value", index)
		}
		if _, duplicate := bindings[claimValue]; duplicate {
			return nil, fmt.Errorf("duplicate jwt claim value %q", claimValue)
		}
		roles, err := validatedRoles(binding.Roles)
		if err != nil {
			return nil, fmt.Errorf("jwt role binding %d: %w", index, err)
		}
		bindings[claimValue] = roles
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("at least one jwt role binding is required")
	}

	verifier, err := newJWTVerifier(algorithm, cfg)
	if err != nil {
		return nil, err
	}

	subjectClaim := strings.TrimSpace(cfg.SubjectClaim)
	if subjectClaim == "" {
		subjectClaim = "sub"
	}
	rolesClaim := strings.TrimSpace(cfg.RolesClaim)
	if rolesClaim == "" {
		rolesClaim = "roles"
	}

	return &JWTAuthenticator{
		algorithm:    algorithm,
		issuer:       issuer,
		audience:     audience,
		subjectClaim: subjectClaim,
		rolesClaim:   rolesClaim,
		bindings:     bindings,
		verifier:     verifier,
		now:          time.Now,
	}, nil
}

func newJWTVerifier(algorithm string, cfg JWTConfig) (jwtVerifier, error) {
	switch algorithm {
	case JWTAlgorithmHS256:
		secretEnv := strings.TrimSpace(cfg.SecretEnv)
		if secretEnv == "" {
			return nil, fmt.Errorf("jwt secret environment variable must not be empty")
		}
		secret, found := os.LookupEnv(secretEnv)
		if !found || secret == "" {
			return nil, fmt.Errorf("jwt secret environment variable %q is not set or is empty", secretEnv)
		}
		return hmacSHA256Verifier{secret: []byte(secret)}, nil
	case JWTAlgorithmRS256:
		publicKeyPath := strings.TrimSpace(cfg.PublicKeyPath)
		if publicKeyPath == "" {
			return nil, fmt.Errorf("jwt public key path must not be empty")
		}
		raw, err := os.ReadFile(publicKeyPath)
		if err != nil {
			return nil, err
		}
		key, err := parseRSAPublicKeyPEM(raw)
		if err != nil {
			return nil, err
		}
		return rsaSHA256Verifier{key: key}, nil
	default:
		return nil, fmt.Errorf("unsupported jwt algorithm %q", algorithm)
	}
}

func parseRSAPublicKeyPEM(raw []byte) (*rsa.PublicKey, error) {
	for len(raw) > 0 {
		block, rest := pem.Decode(raw)
		if block == nil {
			break
		}
		raw = rest

		switch block.Type {
		case "PUBLIC KEY":
			parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			key, ok := parsed.(*rsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("jwt public key is not RSA")
			}
			return key, nil
		case "RSA PUBLIC KEY":
			return x509.ParsePKCS1PublicKey(block.Bytes)
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			key, ok := cert.PublicKey.(*rsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("jwt certificate public key is not RSA")
			}
			return key, nil
		}
	}
	return nil, fmt.Errorf("no RSA public key found in JWT public key file")
}

func (a *JWTAuthenticator) Authenticate(authorization string) (Principal, error) {
	token, err := bearerToken(authorization)
	if err != nil {
		return Principal{}, err
	}

	headerSegment, claimsSegment, signatureSegment, err := splitJWT(token)
	if err != nil {
		return Principal{}, err
	}

	signingInput := []byte(headerSegment + "." + claimsSegment)
	signature, err := base64.RawURLEncoding.DecodeString(signatureSegment)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(headerSegment)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return Principal{}, ErrUnauthenticated
	}
	if !strings.EqualFold(strings.TrimSpace(header.Algorithm), a.algorithm) {
		return Principal{}, ErrUnauthenticated
	}
	if err := a.verifier.Verify(signingInput, signature); err != nil {
		return Principal{}, err
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsSegment)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	claims, err := decodeJWTClaims(claimsJSON)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	if err := a.validateClaims(claims); err != nil {
		return Principal{}, err
	}

	subject, ok := stringClaim(claims[a.subjectClaim])
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Principal{}, ErrUnauthenticated
	}

	claimValues, err := claimValues(claims[a.rolesClaim])
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}

	roles := make([]Role, 0, len(claimValues))
	seen := make(map[Role]struct{}, len(claimValues))
	for _, value := range claimValues {
		for _, role := range a.bindings[value] {
			if _, duplicate := seen[role]; duplicate {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
	}

	return Principal{
		Subject: subject,
		Roles:   roles,
	}, nil
}

func splitJWT(token string) (string, string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", "", ErrUnauthenticated
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", "", "", ErrUnauthenticated
		}
	}
	return parts[0], parts[1], parts[2], nil
}

func decodeJWTClaims(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var claims map[string]any
	if err := decoder.Decode(&claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func (a *JWTAuthenticator) validateClaims(claims map[string]any) error {
	issuer, ok := stringClaim(claims["iss"])
	if !ok || issuer != a.issuer {
		return ErrUnauthenticated
	}
	if !audienceMatches(claims["aud"], a.audience) {
		return ErrUnauthenticated
	}
	expiresAt, ok := numericDate(claims["exp"])
	if !ok || !a.now().Before(expiresAt) {
		return ErrUnauthenticated
	}
	if notBefore, ok := numericDate(claims["nbf"]); ok && a.now().Before(notBefore) {
		return ErrUnauthenticated
	}
	return nil
}

func stringClaim(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

func audienceMatches(value any, audience string) bool {
	switch typed := value.(type) {
	case string:
		return typed == audience
	case []any:
		for _, candidate := range typed {
			text, ok := candidate.(string)
			if ok && text == audience {
				return true
			}
		}
	}
	return false
}

func numericDate(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case json.Number:
		seconds, err := typed.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return time.Unix(seconds, 0), true
	case float64:
		return time.Unix(int64(typed), 0), true
	default:
		return time.Time{}, false
	}
}

func claimValues(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case string:
		return splitClaimString(typed), nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, ErrUnauthenticated
			}
			values = append(values, strings.TrimSpace(text))
		}
		return values, nil
	default:
		return nil, ErrUnauthenticated
	}
}

func splitClaimString(value string) []string {
	if strings.Contains(value, ",") {
		parts := strings.Split(value, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				values = append(values, trimmed)
			}
		}
		return values
	}
	return strings.Fields(value)
}
