package access

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Authenticator resolves an authorization value into a principal.
type Authenticator interface {
	Authenticate(string) (Principal, error)
}

// DisabledAuthenticator grants local administrator access when authentication
// is explicitly disabled. It lets transports use one authentication path in
// local development and secured deployments.
type DisabledAuthenticator struct{}

func NewDisabledAuthenticator() DisabledAuthenticator {
	return DisabledAuthenticator{}
}

func (DisabledAuthenticator) Authenticate(string) (Principal, error) {
	return Principal{Subject: "local", Roles: []Role{RoleAdmin}}, nil
}

// StaticToken describes one principal whose bearer token is supplied through
// an environment variable. TokenEnv is retained only for startup diagnostics;
// the environment value itself is stored only as a digest.
type StaticToken struct {
	Subject  string
	TokenEnv string
	Roles    []Role
}

type staticCredential struct {
	digest    [sha256.Size]byte
	principal Principal
}

// StaticTokenAuthenticator authenticates bearer tokens resolved from the
// process environment at construction time.
type StaticTokenAuthenticator struct {
	credentials []staticCredential
}

// NewStaticTokenAuthenticator resolves configured tokens from the environment.
// It fails closed when an entry is invalid, missing, or resolves to a token
// already assigned to another principal.
func NewStaticTokenAuthenticator(tokens []StaticToken) (*StaticTokenAuthenticator, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("at least one static token is required")
	}

	authenticator := &StaticTokenAuthenticator{
		credentials: make([]staticCredential, 0, len(tokens)),
	}
	subjects := make(map[string]struct{}, len(tokens))
	environments := make(map[string]struct{}, len(tokens))
	digests := make(map[[sha256.Size]byte]struct{}, len(tokens))

	for index, entry := range tokens {
		subject := strings.TrimSpace(entry.Subject)
		if subject == "" {
			return nil, fmt.Errorf("static token entry %d has an empty subject", index)
		}
		subjectKey := strings.ToLower(subject)
		if _, duplicate := subjects[subjectKey]; duplicate {
			return nil, fmt.Errorf("duplicate static token subject %q", subject)
		}
		subjects[subjectKey] = struct{}{}

		tokenEnv := strings.TrimSpace(entry.TokenEnv)
		if tokenEnv == "" {
			return nil, fmt.Errorf("static token entry %d has an empty token environment variable", index)
		}
		if _, duplicate := environments[tokenEnv]; duplicate {
			return nil, fmt.Errorf("duplicate static token environment variable %q", tokenEnv)
		}
		environments[tokenEnv] = struct{}{}

		roles, err := validatedRoles(entry.Roles)
		if err != nil {
			return nil, fmt.Errorf("static token entry %d: %w", index, err)
		}

		token, found := os.LookupEnv(tokenEnv)
		if !found || token == "" {
			return nil, fmt.Errorf("static token environment variable %q is not set or is empty", tokenEnv)
		}
		if strings.TrimSpace(token) != token || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
			return nil, fmt.Errorf("static token environment variable %q contains whitespace", tokenEnv)
		}

		digest := sha256.Sum256([]byte(token))
		if _, duplicate := digests[digest]; duplicate {
			return nil, fmt.Errorf("static token environment variable %q duplicates another token", tokenEnv)
		}
		digests[digest] = struct{}{}

		authenticator.credentials = append(authenticator.credentials, staticCredential{
			digest: digest,
			principal: Principal{
				Subject: subject,
				Roles:   roles,
			},
		})
	}

	return authenticator, nil
}

// Authenticate validates an HTTP Authorization header containing a Bearer
// token. Every configured digest is compared before a result is returned.
func (a *StaticTokenAuthenticator) Authenticate(authorization string) (Principal, error) {
	token, err := bearerToken(authorization)
	if err != nil {
		return Principal{}, err
	}

	candidate := sha256.Sum256([]byte(token))
	match := 0
	for index, credential := range a.credentials {
		equal := subtle.ConstantTimeCompare(candidate[:], credential.digest[:])
		match = subtle.ConstantTimeSelect(equal, index+1, match)
	}
	if match == 0 {
		return Principal{}, ErrUnauthenticated
	}

	principal := a.credentials[match-1].principal
	principal.Roles = append([]Role(nil), principal.Roles...)
	return principal, nil
}

func bearerToken(authorization string) (string, error) {
	header := strings.TrimSpace(authorization)
	scheme, token, found := strings.Cut(header, " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return "", ErrUnauthenticated
	}
	return token, nil
}

func validatedRoles(roles []Role) ([]Role, error) {
	if len(roles) == 0 {
		return nil, fmt.Errorf("at least one role is required")
	}
	validated := make([]Role, 0, len(roles))
	seen := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		if !IsSupportedRole(role) {
			return nil, fmt.Errorf("unsupported role %q", role)
		}
		if _, duplicate := seen[role]; duplicate {
			return nil, fmt.Errorf("duplicate role %q", role)
		}
		seen[role] = struct{}{}
		validated = append(validated, role)
	}
	return validated, nil
}
