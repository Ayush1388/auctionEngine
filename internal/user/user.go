package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	ActivatedAt  *time.Time

	// Role is "user" or "admin" (role-based access control). It is copied
	// into access tokens, so a change takes effect at the next login or
	// refresh, at most one access-token lifetime later.
	Role string
}

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)
