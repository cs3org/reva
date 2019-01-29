package storage

import (
	"context"
	"io"
)

// ACLMode represents the mode for the ACL (read, write, ...).
type ACLMode uint32

// ACLType represents the type of the ACL (user, group, ...).
type ACLType uint32

const (
	// ACLModeInvalid specifies an invalid permission.
	ACLModeInvalid = ACLMode(0) // default is invalid.
	// ACLModeReadOnly specifies read permissions.
	ACLModeReadOnly = ACLMode(1) // 1
	// ACLModeWrite specifies write-permissions.
	ACLModeReadWrite = ACLMode(2) // 2

	// ACLTypeInvalid specifies that the acl is invalid
	ACLTypeInvalid ACLType = ACLType(0)
	// ACLTypeUser specifies that the acl is set for an individual user.
	ACLTypeUser ACLType = ACLType(1)
	// ACLTypeGroup specifies that the acl is set for a group.
	ACLTypeGroup ACLType = ACLType(2)
)

// FS is the interface to implement access to the storage.
type FS interface {
	CreateDir(ctx context.Context, fn string) error
	Delete(ctx context.Context, fn string) error
	Move(ctx context.Context, old, new string) error
	GetMD(ctx context.Context, fn string) (*MD, error)
	ListFolder(ctx context.Context, fn string) ([]*MD, error)
	Upload(ctx context.Context, fn string, r io.ReadCloser) error
	Download(ctx context.Context, fn string) (io.ReadCloser, error)
	ListRevisions(ctx context.Context, fn string) ([]*Revision, error)
	DownloadRevision(ctx context.Context, fn, key string) (io.ReadCloser, error)
	RestoreRevision(ctx context.Context, fn, key string) error
	ListRecycle(ctx context.Context, fn string) ([]*RecycleItem, error)
	RestoreRecycleItem(ctx context.Context, fn, key string) error
	EmptyRecycle(ctx context.Context, fn string) error
	GetPathByID(ctx context.Context, id string) (string, error)
	SetACL(ctx context.Context, fn string, a *ACL) error
	UnsetACL(ctx context.Context, fn string, a *ACL) error
	UpdateACL(ctx context.Context, fn string, a *ACL) error
	ListACLs(ctx context.Context, fn string) ([]*ACL, error)
	GetACL(ctx context.Context, fn string, aclType ACLType, target string) (*ACL, error)
	GetQuota(ctx context.Context, fn string) (int, int, error)
}

// MD represents the metadata about a file/directory.
type MD struct {
	ID          string
	Path        string
	Size        uint64
	Mtime       uint64
	IsDir       bool
	Etag        string
	Checksum    string
	Mime        string
	Permissions *Permissions
	Sys         map[string]interface{}
}

type Permissions struct {
	Read, Write, Share bool
}

// ACL represents an ACL to persist on the storage.
type ACL struct {
	Target string
	Type   ACLType
	Mode   ACLMode
}

// RecycleItem represents an entry in the recycle bin of the user.
type RecycleItem struct {
	RestorePath string
	RestoreKey  string
	Size        uint64
	DelMtime    uint64
	IsDir       bool
}

// Revision represents a version of the file in the past.
type Revision struct {
	RevKey string
	Size   uint64
	Mtime  uint64
	IsDir  bool
}

// Broker is the interface that storage brokers implement
// for discovering storage providers
type Broker interface {
	FindProvider(ctx context.Context, fn string) (*ProviderInfo, error)
}

// ProviderInfo contains the information
// about a StorageProvider
type ProviderInfo struct {
	Location string
}

// FSTable contains descriptive information about the various file systems.
// It follows the same logic as unix fstab.
type FSTable interface {
	AddMount(m Mount) error
	ListMounts() ([]Mount, error)
	RemoveMount(m Mount) error
	GetMount(dir string) (Mount, error)
}

// Mount contains the information on how to mount a filesystem.
type Mount interface {
	GetName() string
	GetDir() string
	GetFS() FS
	GetOptions() *MountOptions
}

// MountOptions are the options for the mount.
type MountOptions struct {
	ForceReadOnly bool
}
