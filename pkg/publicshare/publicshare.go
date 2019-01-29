package publicshare

import (
	"context"

	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/user"
)

const (
	// ACLModeReadOnly specifies that the share is read-only.
	ACLModeReadOnly ACLMode = "read-only"

	// ACLModeReadWrite specifies that the share is read-writable.
	ACLModeReadWrite ACLMode = "read-write"

	// ACLTypeDirectory specifies that the share points to a directory.
	ACLTypeDirectory ACLType = "directory"

	// ACLTypeFile specifies that the share points to a file.
	ACLTypeFile ACLType = "file"
)

type (
	// Manager manipulates public shares.
	Manager interface {
		CreatePublicShare(ctx context.Context, u *user.User, md *storage.MD, a *ACL) (*PublicShare, error)
		UpdatePublicShare(ctx context.Context, u *user.User, id string, up *UpdatePolicy, a *ACL) (*PublicShare, error)
		GetPublicShare(ctx context.Context, u *user.User, id string) (*PublicShare, error)
		ListPublicShares(ctx context.Context, u *user.User, md *storage.MD) ([]*PublicShare, error)
		RevokePublicShare(ctx context.Context, u *user.User, id string) error

		GetPublicShareByToken(ctx context.Context, token string) (*PublicShare, error)
	}

	// PublicShare represents a public share.
	PublicShare struct {
		ID          string
		Token       string
		Filename    string
		Modified    uint64
		Owner       string
		DisplayName string
		ACL         *ACL
	}

	// ACL is the the acl to use when creating or updating public shares.
	ACL struct {
		Password   string
		Expiration uint64
		SetMode    bool
		Mode       ACLMode
		Type       ACLType
	}

	// UpdatePolicy specifies which attributes to update when calling UpdateACL.
	UpdatePolicy struct {
		SetPassword   bool
		SetExpiration bool
		SetMode       bool
	}

	// ACLMode represents the mode for the share (read-only, read-write, ...)
	ACLMode string

	// ACLType represents the type of file the share points to (file, directory, ...)
	ACLType string
)

/*
AuthenticatePublicShare(ctx context.Context, token, password string) (*PublicShare, error)
	IsPublicShareProtected(ctx context.Context, token string) (bool, error)
*/
