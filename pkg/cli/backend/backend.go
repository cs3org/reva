// Copyright 2018-2026 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

// Package backend defines the user plane of the CLI: the metadata calls
// (Backend), which have one implementation per transport, and the data
// transfers (DataPlane), which have one implementation for all transports.
// The data model is the CS3 messages, also over HTTPS.
//
// A method returns a *StatusError for a CS3 status that is not OK, and an
// error that matches ErrNotAvailable for what its transport cannot do.
package backend

import (
	"context"
	"io"
	"net/http"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
)

// The names of the transports.
const (
	GRPC = "grpc"
	HTTP = "http"
)

// Backend is the metadata plane. The runtime gives it the reva token, so a
// backend never reads token files.
type Backend interface {
	// Transport is the name of the transport, for the messages.
	Transport() string
	Files() Files
	Shares() Shares
	Links() Links
	Spaces() Spaces
	Trash() Trash
	Versions() Versions
	Locks() Locks
	OCM() OCM
	Tokens() Tokens
	Users() Users
	// Locator tells the data plane where the transfers go.
	Locator() Locator
	// ServerTime is the clock of the server, to find the skew of the local
	// clock.
	ServerTime(ctx context.Context) (time.Time, error)
}

// Files are the calls on files and folders.
type Files interface {
	Stat(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error)
	List(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error)
	// Walk calls fn for ref and, for a folder, for all that is below it. A
	// folder comes before its contents.
	Walk(ctx context.Context, ref *provider.Reference, fn func(*provider.ResourceInfo) error) error
	// Mkdir creates the folder ref. With parents, it also creates the missing
	// parents, and a folder that exists is not an error.
	Mkdir(ctx context.Context, ref *provider.Reference, parents bool) error
	// Move moves src to dst. Without overwrite, a dst that exists is an error
	// with CODE_ALREADY_EXISTS.
	Move(ctx context.Context, src, dst *provider.Reference, overwrite bool) error
	// Copy copies src to dst, with the same rule as Move for dst.
	Copy(ctx context.Context, src, dst *provider.Reference, overwrite bool) error
	Remove(ctx context.Context, ref *provider.Reference) error
	Touch(ctx context.Context, ref *provider.Reference) error
	// ResolveSpace returns the root of the space that alias names: a space
	// id, an alias, a project name, or home.
	ResolveSpace(ctx context.Context, alias string) (*provider.Reference, error)
}

// Shares are the shares with users and groups. An id is the opaque id of
// the share, as List shows it.
type Shares interface {
	// List returns the shares that the caller manages. A resource that is not
	// nil keeps only its shares.
	List(ctx context.Context, resource *provider.ResourceId) ([]*collaboration.Share, error)
	Get(ctx context.Context, id string) (*collaboration.Share, error)
	Create(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.Share, error)
	Update(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.Share, error)
	Remove(ctx context.Context, id string) error
	// Received returns the shares that the caller received.
	Received(ctx context.Context) ([]*collaboration.ReceivedShare, error)
	UpdateReceived(ctx context.Context, id string, state collaboration.ShareState) (*collaboration.ReceivedShare, error)
	HideReceived(ctx context.Context, id string) error
}

// Links are the public links.
type Links interface {
	// List returns the links that the caller created. A resource that is not
	// nil keeps only its links.
	List(ctx context.Context, resource *provider.ResourceId) ([]*link.PublicShare, error)
	Get(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error)
	Create(ctx context.Context, req *link.CreatePublicShareRequest) (*link.PublicShare, error)
	Update(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.PublicShare, error)
	Remove(ctx context.Context, ref *link.PublicShareReference) error
}

// Spaces are the storage spaces of the caller.
type Spaces interface {
	List(ctx context.Context) ([]*provider.StorageSpace, error)
	// Quota returns the quota of the space of ref.
	Quota(ctx context.Context, ref *provider.Reference) (*Quota, error)
}

// Quota is the quota of a space, in bytes.
type Quota struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Remaining uint64 `json:"remaining"`
}

// Trash is the recycle bin. A nil ref is the home space of the caller.
type Trash interface {
	// List returns the deleted items. A zero from or to is no limit.
	List(ctx context.Context, ref *provider.Reference, from, to time.Time) ([]*provider.RecycleItem, error)
	// Restore puts the item key back where it was.
	Restore(ctx context.Context, ref *provider.Reference, key string) error
	Purge(ctx context.Context, ref *provider.Reference, key string) error
}

// Versions are the versions of a file.
type Versions interface {
	List(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	Restore(ctx context.Context, ref *provider.Reference, key string) error
}

// Locks are the locks of a file.
type Locks interface {
	Get(ctx context.Context, ref *provider.Reference) (*provider.Lock, error)
	Set(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error
	Refresh(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error
	Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error
}

// OCM is sharing with users of other servers (Open Cloud Mesh). An id is
// the opaque id of an OCM share.
type OCM interface {
	CreateInvite(ctx context.Context, description string) (*invitepb.InviteToken, error)
	Invites(ctx context.Context) ([]*invitepb.InviteToken, error)
	// AcceptInvite accepts the invitation token that the provider at domain
	// gave.
	AcceptInvite(ctx context.Context, token, domain string) error
	// Contacts returns the remote users that the caller can share with.
	Contacts(ctx context.Context, filter string) ([]*userpb.User, error)
	RemoveContact(ctx context.Context, id *userpb.UserId) error
	Providers(ctx context.Context) ([]*ocmprovider.ProviderInfo, error)
	// CreateShare shares ref with the contact grantee. grantee.Idp is the
	// domain of the provider of the contact.
	CreateShare(ctx context.Context, ref *provider.Reference, grantee *userpb.UserId, methods []*ocm.AccessMethod) (*ocm.Share, error)
	// Shares returns the OCM shares that the caller manages. A resource that
	// is not nil keeps only its shares.
	Shares(ctx context.Context, resource *provider.ResourceId) ([]*ocm.Share, error)
	GetShare(ctx context.Context, id string) (*ocm.Share, error)
	UpdateShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) error
	RemoveShare(ctx context.Context, id string) error
	Received(ctx context.Context) ([]*ocm.ReceivedShare, error)
	GetReceived(ctx context.Context, id string) (*ocm.ReceivedShare, error)
	UpdateReceived(ctx context.Context, id string, state ocm.ShareState) error
}

// Tokens are the app tokens (app passwords) of the caller. The id differs
// per transport: the password hash over gRPC, the client id over HTTPS.
type Tokens interface {
	List(ctx context.Context) ([]*authapp.AppPassword, error)
	// Create returns the new token. It is the only answer with the secret.
	Create(ctx context.Context, req *authapp.GenerateAppPasswordRequest) (*authapp.AppPassword, error)
	Revoke(ctx context.Context, id string) error
}

// Users are the user lookups.
type Users interface {
	// Me returns the user of the token.
	Me(ctx context.Context) (*userpb.User, error)
	// ByClaim finds a user by a claim, for example username or mail.
	ByClaim(ctx context.Context, claim, value string) (*userpb.User, error)
}

// Locator gives the data plane the targets of the transfers. Over gRPC they
// come from InitiateFileDownload and InitiateFileUpload, over HTTPS from the
// WebDAV URLs.
type Locator interface {
	DownloadTarget(ctx context.Context, ref *provider.Reference) (*Target, error)
	// UploadTarget returns where to send size bytes for ref. An ifMatch that
	// is not empty is the etag that ref must still have.
	UploadTarget(ctx context.Context, ref *provider.Reference, size int64, ifMatch string) (*Target, error)
	OpenInApp(ctx context.Context, ref *provider.Reference, opts AppOptions) (*appprovider.OpenInAppURL, error)
}

// Target is one HTTP transfer.
type Target struct {
	URL string
	// Header holds the credentials of the transfer.
	Header http.Header
	// Checksums are the checksum types that the target accepts with an
	// upload, in the xs and xs_type query parameters. nil means that the
	// target takes no checksum.
	Checksums []*provider.ResourceChecksumPriority
}

// AppOptions select how OpenInApp opens a file.
type AppOptions struct {
	// App is the name of the app. Empty is the default app of the type.
	App      string
	ViewMode gateway.OpenInAppRequest_ViewMode
	// Insecure and SkipVerify go to the app provider, for its connection to
	// the app.
	Insecure, SkipVerify bool
}

// DataPlane moves the content of files. It has one implementation for all
// transports (package data): only the Locator differs.
type DataPlane interface {
	// Download returns the content of ref from offset, and the number of
	// bytes that it has, or -1 if that is not known.
	Download(ctx context.Context, ref *provider.Reference, offset int64) (io.ReadCloser, int64, error)
	// Upload writes ref in one request. open can be called more than once,
	// because a checksum reads the content before the transfer. checksum is
	// the type to send: adler32, md5, sha1, negotiate for the preference of
	// the server, or empty for none.
	Upload(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string) error
	// UploadIfUnchanged is Upload without a checksum, only while ref still
	// has the etag. Else the error has CODE_FAILED_PRECONDITION.
	UploadIfUnchanged(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, etag string) error
	// UploadStream writes size bytes from r, which can be read only once. It
	// is never retried.
	UploadStream(ctx context.Context, ref *provider.Reference, r io.Reader, size int64) error
	// UploadResumable writes ref with TUS, in parts that a new call can
	// continue.
	UploadResumable(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string) error
	// Archive returns refs as one archive, in format zip or tar.
	Archive(ctx context.Context, refs []*provider.Reference, format string) (io.ReadCloser, error)
	OpenInApp(ctx context.Context, ref *provider.Reference, opts AppOptions) (*appprovider.OpenInAppURL, error)
}

// The optional abilities: a backend that has one implements its interface.
// When the backend of a command does not, the runtime uses the HTTPS backend
// for that call if an endpoint is known.

// Searcher finds files by name (the WebDAV REPORT).
type Searcher interface {
	Search(ctx context.Context, ref *provider.Reference, opts SearchOptions) ([]*provider.ResourceInfo, error)
}

// SearchOptions are the options of Search.
type SearchOptions struct {
	// Pattern is a part of the name.
	Pattern string
	// Limit is the maximum number of results. Zero is the server default.
	Limit int
}

// GrantLister reads the raw CS3 grants of a resource.
type GrantLister interface {
	ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
}

// PreferenceStore reads and writes the preferences of the caller.
type PreferenceStore interface {
	GetPreference(ctx context.Context, namespace, key string) (string, error)
	SetPreference(ctx context.Context, namespace, key, value string) error
}

// TransferManager controls the data transfers of OCM shares.
type TransferManager interface {
	// ListTransfers returns the transfers, only those of the OCM share
	// shareID when it is not empty.
	ListTransfers(ctx context.Context, shareID string) ([]*datatx.TxInfo, error)
	TransferStatus(ctx context.Context, id string) (*datatx.TxInfo, error)
	CancelTransfer(ctx context.Context, id string) (*datatx.TxInfo, error)
	RetryTransfer(ctx context.Context, id string) (*datatx.TxInfo, error)
}
