package access

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
	ErrUnsupportedRole = errors.New("unsupported role")
)

// Authorize checks whether principal may perform an operation requiring role.
// Administrators are authorized for every supported role.
func Authorize(principal Principal, role Role) error {
	if strings.TrimSpace(principal.Subject) == "" {
		return ErrUnauthenticated
	}
	if !IsSupportedRole(role) {
		return fmt.Errorf("%w: %q", ErrUnsupportedRole, role)
	}
	if principal.HasRole(RoleAdmin) || principal.HasRole(role) {
		return nil
	}
	return fmt.Errorf("%w: subject %q requires role %q", ErrForbidden, principal.Subject, role)
}

// RequireRole authorizes the principal stored in ctx for role.
func RequireRole(ctx context.Context, role Role) error {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return ErrUnauthenticated
	}
	return Authorize(principal, role)
}
