package access

import "context"

// Role identifies a capability granted to an authenticated principal.
type Role string

const (
	RoleReader         Role = "reader"
	RoleEvidenceWriter Role = "evidence_writer"
	RoleAdmin          Role = "admin"
)

// Principal is the authenticated identity associated with a request.
type Principal struct {
	Subject string `json:"subject"`
	Roles   []Role `json:"roles"`
}

// HasRole reports whether the principal was explicitly granted role.
func (p Principal) HasRole(role Role) bool {
	for _, candidate := range p.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// IsSupportedRole reports whether role is understood by merger.
func IsSupportedRole(role Role) bool {
	switch role {
	case RoleReader, RoleEvidenceWriter, RoleAdmin:
		return true
	default:
		return false
	}
}

type principalContextKey struct{}

// WithPrincipal returns a context carrying principal. A defensive copy of the
// roles prevents callers from mutating authorization state through a slice.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	principal.Roles = append([]Role(nil), principal.Roles...)
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated principal, when present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	if !ok {
		return Principal{}, false
	}
	principal.Roles = append([]Role(nil), principal.Roles...)
	return principal, true
}
