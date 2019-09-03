package memory

import (
	"context"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	v0alpha "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	pbtypes "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new memory manager
func New(c map[string]interface{}) (publicshare.Manager, error) {
	return &manager{}, nil
}

type manager struct{}

// TODO(refs) implement application logic.
func (m *manager) CreatePublicShare(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo, g *publicshareproviderv0alphapb.Grant) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}

func (m *manager) UpdatePublicShare(ctx context.Context, u *authv0alphapb.User, ref *publicshareproviderv0alphapb.PublicShareReference, g *publicshareproviderv0alphapb.Grant) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}

func (m *manager) GetPublicShare(ctx context.Context, u *authv0alphapb.User, ref *publicshareproviderv0alphapb.PublicShareReference) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}

func (m *manager) ListPublicShares(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo) ([]*publicshareproviderv0alphapb.PublicShare, error) {
	shares := []*publicshareproviderv0alphapb.PublicShare{
		&publicshareproviderv0alphapb.PublicShare{
			Id: &publicshareproviderv0alphapb.PublicShareId{
				OpaqueId: "some_publicly_shared_id",
			},
			Token:       "my_token",
			ResourceId:  &v0alpha.ResourceId{},
			Permissions: &publicshareproviderv0alphapb.PublicSharePermissions{},
			Owner:       &pbtypes.UserId{},
			Creator:     &pbtypes.UserId{},
			Ctime:       &pbtypes.Timestamp{},
			Expiration:  &pbtypes.Timestamp{},
			Mtime:       &pbtypes.Timestamp{},
			DisplayName: "some_public_share",
		},
	}
	return shares, nil
}

func (m *manager) RevokePublicShare(ctx context.Context, u *authv0alphapb.User, id string) error {
	return nil
}

func (m *manager) GetPublicShareByToken(ctx context.Context, token string) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}
