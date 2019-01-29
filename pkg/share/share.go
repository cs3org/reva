package share

import (
	"context"

	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/user"
)

const (
	// StateAccepted means the share has been accepted and it can be accessed.
	StateAccepted State = "accepted"
	// StatePending means the share needs to be accepted or rejected.
	StatePending State = "pending"
	// StateRejected means the share has been rejected and is not accessible.
	StateRejected State = "rejected"

	// ACLModeReadOnly means the receiver will only be able to browse and download contents.
	ACLModeReadOnly ACLMode = "read-only"
	// ACLModeReadWrite means the receiver will be able to manipulate the contents (write, delete, rename...)
	ACLModeReadWrite ACLMode = "read-write"

	// ACLTypeUser means the receiver of the share is an individual user.
	ACLTypeUser ACLType = "user"
	// ACLTypeGroup means the receiver of the share is a group of people.
	ACLTypeGroup ACLType = "group"
)

type (
	// ACLMode is the permission for the share.
	ACLMode string

	// ACLType is the type of the share.
	ACLType string

	// State represents the state of the share.
	State string

	// Manager is the interface that manipulates shares.
	Manager interface {
		// Create a new share in fn with the given acl.
		Share(ctx context.Context, u *user.User, md *storage.MD, a *ACL) (*Share, error)

		// GetShare gets the information for a share by the given id.
		GetShare(ctx context.Context, u *user.User, id string) (*Share, error)

		// Unshare deletes the share pointed by id.
		Unshare(ctx context.Context, u *user.User, id string) error

		// UpdateShare updates the mode of the given share.
		UpdateShare(ctx context.Context, u *user.User, id string, mode ACLMode) (*Share, error)

		// ListShares returns the shares created by the user. If forPath is not empty,
		// it returns only shares attached to the given path.
		ListShares(ctx context.Context, u *user.User, md *storage.MD) ([]*Share, error)

		// ListReceivedShares returns the list of shares the user has access.
		ListReceivedShares(ctx context.Context, u *user.User) ([]*Share, error)

		// GetReceivedShare returns the information for the share received with
		// the given id.
		GetReceivedShare(ctx context.Context, u *user.User, id string) (*Share, error)

		// RejectReceivedShare rejects the share by the given id.
		RejectReceivedShare(ctx context.Context, u *user.User, id string) error
	}

	// ACL represents the information about the nature of the share.
	ACL struct {
		// Target is the recipient of the share.
		Target string

		// Mode is the mode for the share.
		Mode ACLMode

		// Type is the type of the share.
		Type ACLType
	}

	// Share represents the information stored in a share.
	Share struct {
		// ID represents the ID of the share.
		ID string
		// Filename points to the source of the share.
		Filename string
		// Owner is the account name owning the share.
		Owner string
		// ACL represents the information about the target of the share.
		ACL *ACL
		// Created represents the creation time in seconds from unix epoch.
		Created uint64
		// Modified represents the modification time in seconds from unix epoch.
		Modified uint64
		// State represents the state of the share.
		State State
	}
)
