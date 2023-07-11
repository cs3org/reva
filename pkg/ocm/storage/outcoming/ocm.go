// Copyright 2018-2023 CERN
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

package outcoming

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

func init() {
	registry.Register("ocmoutcoming", New)
}

type driver struct {
	c       *config
	gateway gateway.GatewayAPIClient
}

type config struct {
	GatewaySVC    string `mapstructure:"gatewaysvc"`
	MachineSecret string `mapstructure:"machine_secret"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
}

// New creates an OCM storage driver.
func New(ctx context.Context, m map[string]interface{}) (storage.FS, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	gateway, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySVC))
	if err != nil {
		return nil, err
	}

	d := &driver{
		c:       &c,
		gateway: gateway,
	}

	return d, nil
}

func (d *driver) resolveToken(ctx context.Context, token string) (*ocmv1beta1.Share, error) {
	shareRes, err := d.gateway.GetOCMShare(ctx, &ocmv1beta1.GetOCMShareRequest{
		Ref: &ocmv1beta1.ShareReference{
			Spec: &ocmv1beta1.ShareReference_Token{
				Token: token,
			},
		},
	})

	switch {
	case err != nil:
		return nil, err
	case shareRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(token)
	case shareRes.Status.Code != rpcv1beta1.Code_CODE_OK:
		return nil, errtypes.InternalError(shareRes.Status.Message)
	}

	return shareRes.Share, nil
}

func (d *driver) stat(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	statRes, err := d.gateway.Stat(ctx, &provider.StatRequest{Ref: ref})
	switch {
	case err != nil:
		return nil, err
	case statRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(ref.String())
	case statRes.Status.Code != rpcv1beta1.Code_CODE_OK:
		return nil, errtypes.InternalError(statRes.Status.Message)
	}

	return statRes.Info, nil
}

func makeRelative(path string) string {
	if strings.HasPrefix(path, "/") {
		return "." + path
	}
	return path
}

func (d *driver) shareAndRelativePathFromRef(ctx context.Context, ref *provider.Reference) (*ocmv1beta1.Share, string, error) {
	var (
		token string
		path  string
	)
	if ref.ResourceId == nil {
		// path is of type /token/<rel-path>
		token, path = rhttp.ShiftPath(ref.Path)
	} else {
		// opaque id is of type token:rel.path
		s := strings.SplitN(ref.ResourceId.OpaqueId, ":", 2)
		token = s[0]
		if len(s) == 2 {
			path = s[1]
		}
		path = filepath.Join(path, ref.Path)
	}
	path = makeRelative(path)

	share, err := d.resolveToken(ctx, token)
	if err != nil {
		return nil, "", err
	}
	return share, path, nil
}

func (d *driver) translateOCMShareResourceToCS3Ref(ctx context.Context, resID *provider.ResourceId, rel string) (*provider.Reference, error) {
	info, err := d.stat(ctx, &provider.Reference{ResourceId: resID})
	if err != nil {
		return nil, err
	}
	return &provider.Reference{
		Path: filepath.Join(info.Path, rel),
	}, nil
}

func (d *driver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, ref *provider.Reference) error {
		res, err := d.gateway.CreateContainer(ctx, &provider.CreateContainerRequest{Ref: ref})
		switch {
		case err != nil:
			return err
		case res.Status.Code != rpcv1beta1.Code_CODE_OK:
			// TODO: better error handling
			return errtypes.InternalError(res.Status.Message)
		}
		return nil
	})
}

func (d *driver) TouchFile(ctx context.Context, ref *provider.Reference) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, ref *provider.Reference) error {
		res, err := d.gateway.TouchFile(ctx, &provider.TouchFileRequest{Ref: ref})
		switch {
		case err != nil:
			return err
		case res.Status.Code != rpcv1beta1.Code_CODE_OK:
			// TODO: better error handling
			return errtypes.InternalError(res.Status.Message)
		}
		return nil
	})
}

func (d *driver) Delete(ctx context.Context, ref *provider.Reference) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, ref *provider.Reference) error {
		res, err := d.gateway.Delete(ctx, &provider.DeleteRequest{Ref: ref})
		switch {
		case err != nil:
			return err
		case res.Status.Code != rpcv1beta1.Code_CODE_OK:
			// TODO: better error handling
			return errtypes.InternalError(res.Status.Message)
		}
		return nil
	})
}

func (d *driver) Move(ctx context.Context, from, to *provider.Reference) error {
	return errtypes.NotSupported("not yet implemented")
}

func (d *driver) opFromUser(ctx context.Context, userID *userv1beta1.UserId, f func(ctx context.Context) error) error {
	userRes, err := d.gateway.GetUser(ctx, &userv1beta1.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return err
	}
	if userRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		return errors.New(userRes.Status.Message)
	}

	authRes, err := d.gateway.Authenticate(context.TODO(), &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     userRes.User.Username,
		ClientSecret: d.c.MachineSecret,
	})
	if err != nil {
		return err
	}
	if authRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		return errors.New(authRes.Status.Message)
	}

	ownerCtx := context.TODO()
	ownerCtx = ctxpkg.ContextSetToken(ownerCtx, authRes.Token)
	ownerCtx = ctxpkg.ContextSetUser(ownerCtx, authRes.User)
	ownerCtx = metadata.AppendToOutgoingContext(ownerCtx, ctxpkg.TokenHeader, authRes.Token)

	return f(ownerCtx)
}

func (d *driver) unwrappedOpFromShareCreator(ctx context.Context, share *ocmv1beta1.Share, rel string, f func(ctx context.Context, ref *provider.Reference) error) error {
	return d.opFromUser(ctx, share.Creator, func(ctx context.Context) error {
		newRef, err := d.translateOCMShareResourceToCS3Ref(ctx, share.ResourceId, rel)
		if err != nil {
			return err
		}
		return f(ctx, newRef)
	})
}

func (d *driver) GetMD(ctx context.Context, ref *provider.Reference, _ []string) (*provider.ResourceInfo, error) {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	var info *provider.ResourceInfo
	if err := d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		info, err = d.stat(ctx, newRef)
		if err != nil {
			return err
		}
		return d.augmentResourceInfo(ctx, info, share)
	}); err != nil {
		return nil, err
	}

	return info, nil
}

func (d *driver) augmentResourceInfo(ctx context.Context, info *provider.ResourceInfo, share *ocmv1beta1.Share) error {
	// prevent leaking internal paths
	shareInfo, err := d.stat(ctx, &provider.Reference{ResourceId: share.ResourceId})
	if err != nil {
		return err
	}
	fixResourceInfo(info, shareInfo, share, getPermissionsFromShare(share))
	return nil
}

func getPermissionsFromShare(share *ocmv1beta1.Share) *provider.ResourcePermissions {
	for _, m := range share.AccessMethods {
		switch v := m.Term.(type) {
		case *ocmv1beta1.AccessMethod_WebdavOptions:
			return v.WebdavOptions.Permissions
		case *ocmv1beta1.AccessMethod_WebappOptions:
			mode := v.WebappOptions.ViewMode
			if mode == providerv1beta1.ViewMode_VIEW_MODE_READ_WRITE {
				return conversions.NewEditorRole().CS3ResourcePermissions()
			}
			return conversions.NewViewerRole().CS3ResourcePermissions()
		}
	}
	return nil
}

func fixResourceInfo(info, shareInfo *provider.ResourceInfo, share *ocmv1beta1.Share, perms *provider.ResourcePermissions) {
	// fix path
	relPath := makeRelative(strings.TrimPrefix(info.Path, shareInfo.Path))
	info.Path = filepath.Join("/", share.Token, relPath)

	// to enable collaborative apps, the fileid must be the same
	// of the proxied storage

	// fix permissions
	info.PermissionSet = perms
}

func (d *driver) ListFolder(ctx context.Context, ref *provider.Reference, _ []string) ([]*provider.ResourceInfo, error) {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	var infos []*provider.ResourceInfo
	if err := d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		lstRes, err := d.gateway.ListContainer(ctx, &provider.ListContainerRequest{Ref: newRef})
		switch {
		case err != nil:
			return err
		case lstRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case lstRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(lstRes.Status.Message)
		}
		infos = lstRes.Infos

		shareInfo, err := d.stat(ctx, &provider.Reference{ResourceId: share.ResourceId})
		if err != nil {
			return err
		}

		perms := getPermissionsFromShare(share)
		for _, info := range infos {
			fixResourceInfo(info, shareInfo, share, perms)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return infos, nil
}

func exposedPathFromReference(ref *provider.Reference) string {
	if ref.ResourceId == nil {
		return ref.Path
	}

	s := strings.SplitN(ref.ResourceId.StorageId, ":", 2)
	tkn := s[0]
	var rel string
	if len(s) == 2 {
		rel = s[1]
	}
	return filepath.Join("/", tkn, rel, ref.Path)
}

func (d *driver) InitiateUpload(ctx context.Context, ref *provider.Reference, _ int64, _ map[string]string) (map[string]string, error) {
	p := exposedPathFromReference(ref)
	return map[string]string{
		"simple": p,
	}, nil
}

func getUploadProtocol(protocols []*gateway.FileUploadProtocol, protocol string) (string, string, bool) {
	for _, p := range protocols {
		if p.Protocol == protocol {
			return p.UploadEndpoint, p.Token, true
		}
	}
	return "", "", false
}

func (d *driver) Upload(ctx context.Context, ref *provider.Reference, content io.ReadCloser) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		initRes, err := d.gateway.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{Ref: newRef})
		switch {
		case err != nil:
			return err
		case initRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(initRes.Status.Message)
		}

		endpoint, token, ok := getUploadProtocol(initRes.Protocols, "simple")
		if !ok {
			return errtypes.InternalError("simple upload not supported")
		}

		httpReq, err := rhttp.NewRequest(ctx, http.MethodPut, endpoint, content)
		if err != nil {
			return errors.Wrap(err, "error creating new request")
		}

		httpReq.Header.Set(datagateway.TokenTransportHeader, token)

		httpRes, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return errors.Wrap(err, "error doing put request")
		}
		defer httpRes.Body.Close()

		if httpRes.StatusCode != http.StatusOK {
			return errors.Errorf("error doing put request: %s", httpRes.Status)
		}

		return nil
	})
}

func getDownloadProtocol(protocols []*gateway.FileDownloadProtocol, lst []string) (string, string, bool) {
	for _, p := range protocols {
		for _, prot := range lst {
			if p.Protocol == prot {
				return p.DownloadEndpoint, p.Token, true
			}
		}
	}
	return "", "", false
}

func (d *driver) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	var r io.ReadCloser
	if err := d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		initRes, err := d.gateway.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{Ref: newRef})
		switch {
		case err != nil:
			return err
		case initRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case initRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(initRes.Status.Message)
		}

		endpoint, token, ok := getDownloadProtocol(initRes.Protocols, []string{"simple", "spaces"})
		if !ok {
			return errtypes.InternalError("simple download not supported")
		}

		httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		httpReq.Header.Set(datagateway.TokenTransportHeader, token)

		httpRes, err := http.DefaultClient.Do(httpReq) //nolint:golint,bodyclose
		if err != nil {
			return err
		}

		if httpRes.StatusCode != http.StatusOK {
			return errors.New(httpRes.Status)
		}
		r = httpRes.Body
		return nil
	}); err != nil {
		return nil, err
	}

	return r, nil
}

func (d *driver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	info, err := d.GetMD(ctx, &provider.Reference{ResourceId: id}, nil)
	if err != nil {
		return "", err
	}
	return info.Path, nil
}

func (d *driver) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		lockRes, err := d.gateway.SetLock(ctx, &provider.SetLockRequest{
			Ref:  newRef,
			Lock: lock,
		})
		switch {
		case err != nil:
			return err
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_FAILED_PRECONDITION:
			return errtypes.BadRequest(lockRes.Status.Message)
		case lockRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(lockRes.Status.Message)
		}
		return nil
	})
}

func (d *driver) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	var lock *provider.Lock
	if err := d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		lockRes, err := d.gateway.GetLock(ctx, &provider.GetLockRequest{Ref: newRef})
		switch {
		case err != nil:
			return err
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case lockRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(lockRes.Status.Message)
		}

		lock = lockRes.Lock
		return nil
	}); err != nil {
		return nil, err
	}
	return lock, nil
}

func (d *driver) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		lockRes, err := d.gateway.RefreshLock(ctx, &provider.RefreshLockRequest{
			Ref:            newRef,
			ExistingLockId: existingLockID,
			Lock:           lock,
		})
		switch {
		case err != nil:
			return err
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_FAILED_PRECONDITION:
			return errtypes.BadRequest(lockRes.Status.Message)
		case lockRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(lockRes.Status.Message)
		}
		return nil
	})
}

func (d *driver) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		lockRes, err := d.gateway.Unlock(ctx, &provider.UnlockRequest{
			Ref:  newRef,
			Lock: lock,
		})
		switch {
		case err != nil:
			return err
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case lockRes.Status.Code == rpcv1beta1.Code_CODE_FAILED_PRECONDITION:
			return errtypes.BadRequest(lockRes.Status.Message)
		case lockRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(lockRes.Status.Message)
		}
		return nil
	})
}

func (d *driver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		res, err := d.gateway.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
			Ref:               newRef,
			ArbitraryMetadata: md,
		})
		switch {
		case err != nil:
			return err
		case res.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case res.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(res.Status.Message)
		}
		return nil
	})
}

func (d *driver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	share, rel, err := d.shareAndRelativePathFromRef(ctx, ref)
	if err != nil {
		return err
	}

	return d.unwrappedOpFromShareCreator(ctx, share, rel, func(ctx context.Context, newRef *provider.Reference) error {
		res, err := d.gateway.UnsetArbitraryMetadata(ctx, &provider.UnsetArbitraryMetadataRequest{
			Ref:                   newRef,
			ArbitraryMetadataKeys: keys,
		})
		switch {
		case err != nil:
			return err
		case res.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case res.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(res.Status.Message)
		}
		return nil
	})
}

func (d *driver) Shutdown(ctx context.Context) error {
	return nil
}

func (d *driver) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("operation not supported")
}

func (d *driver) CreateHome(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("operation not supported")
}
func (d *driver) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListRecycle(ctx context.Context, basePath, key, relativePath string) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) GetQuota(ctx context.Context, ref *provider.Reference) ( /*TotalBytes*/ uint64 /*UsedBytes*/, uint64, error) {
	return 0, 0, errtypes.NotSupported("operation not supported")
}

func (d *driver) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("operation not supported")
}
