package outgoing

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

type OutgoingUserStatus string

const (
	OutgoingUserStatusGracePeriod OutgoingUserStatus = "graceperiod"
	OutgoingUserStatusArchiving   OutgoingUserStatus = "archiving"
)

func (ous OutgoingUserStatus) AsString() string { return string(ous) }

// Manager is the interface for managing outgoing users.
type Manager interface {
	// AddOutgoingUser adds a user to the outgoing users table
	AddOutgoingUser(ctx context.Context, user *userpb.UserId, status OutgoingUserStatus) error
	// GetOutgoingUser retrieves a user's status from the outgoing users table
	GetOutgoingUser(ctx context.Context, user *userpb.UserId) (OutgoingUserStatus, error)
	// UpdateOutgoingUserStatus updates the status of an outgoing user
	UpdateOutgoingUserStatus(ctx context.Context, user *userpb.UserId, status OutgoingUserStatus) error
	// RemoveOutgoingUser removes a user from the outgoing users table
	RemoveOutgoingUser(ctx context.Context, user *userpb.UserId) error
	// ListOutgoingUsers lists all outgoing users, optionally filtered by status
	ListOutgoingUsers(ctx context.Context, status *OutgoingUserStatus) ([]OutgoingUserInfo, error)
}

// OutgoingUserInfo contains information about an outgoing user
type OutgoingUserInfo struct {
	User   *userpb.UserId
	Status OutgoingUserStatus
}
