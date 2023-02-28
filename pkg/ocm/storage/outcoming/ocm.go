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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
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

func parseConfig(c map[string]interface{}) (*config, error) {
	var conf config
	err := mapstructure.Decode(c, &conf)
	return &conf, err
}

func (c *config) init() {
	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
}

// New creates an OCM storage driver.
func New(c map[string]interface{}) (storage.FS, error) {
	conf, err := parseConfig(c)
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding config")
	}
	conf.init()

	gateway, err := pool.GetGatewayServiceClient(pool.Endpoint(conf.GatewaySVC))
	if err != nil {
		return nil, err
	}

	d := &driver{
		c:       conf,
		gateway: gateway,
	}

	return d, nil
}

func (d *driver) resolveToken(ctx context.Context, token string) (*provider.ResourceId, *ocmv1beta1.Share, error) {
	shareRes, err := d.gateway.GetOCMShare(ctx, &ocmv1beta1.GetOCMShareRequest{
		Ref: &ocmv1beta1.ShareReference{
			Spec: &ocmv1beta1.ShareReference_Token{
				Token: token,
			},
		},
	})

	switch {
	case err != nil:
		return nil, nil, err
	case shareRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound(token)
	case shareRes.Status.Code != rpcv1beta1.Code_CODE_OK:
		return nil, nil, errtypes.InternalError(shareRes.Status.Message)
	}

	return shareRes.Share.ResourceId, shareRes.Share, nil
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

func (d *driver) resolvePath(ctx context.Context, path string) (*provider.Reference, *ocmv1beta1.Share, error) {
	// path is of type /token/<rel-path>
	tkn, rel := router.ShiftPath(path)
	rel = makeRelative(rel)

	resId, share, err := d.resolveToken(ctx, tkn)
	if err != nil {
		return nil, nil, err
	}

	info, err := d.stat(ctx, &provider.Reference{ResourceId: resId})
	if err != nil {
		return nil, nil, err
	}

	fmt.Println("********************* info from stat in resolving path", info.Path)

	p := filepath.Join(info.Path, rel)

	return &provider.Reference{
		Path: p,
	}, share, nil
}

func makeRelative(path string) string {
	if strings.HasPrefix(path, "/") {
		return "." + path
	}
	return path
}

func (d *driver) translateOCMShareToCS3Ref(ctx context.Context, ref *provider.Reference) (*provider.Reference, *ocmv1beta1.Share, error) {
	if ref.ResourceId == nil {
		return d.resolvePath(ctx, ref.Path)
	}

	s := strings.SplitN(ref.ResourceId.OpaqueId, ":", 2)
	tkn := s[0]
	var path string
	if len(s) == 2 {
		path = s[1]
	}
	path = filepath.Join(path, ref.Path)
	path = makeRelative(path)

	resID, share, err := d.resolveToken(ctx, tkn)
	if err != nil {
		return nil, nil, err
	}

	return &provider.Reference{
		ResourceId: resID,
		Path:       path,
	}, share, nil
}

func (d *driver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	newRef, _, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return err
	}

	res, err := d.gateway.CreateContainer(ctx, &provider.CreateContainerRequest{Ref: newRef})
	switch {
	case err != nil:
		return err
	case res.Status.Code != rpcv1beta1.Code_CODE_OK:
		// TODO: better error handling
		return errtypes.InternalError(res.Status.Message)
	}

	return nil
}

func (d *driver) TouchFile(ctx context.Context, ref *provider.Reference) error {
	newRef, _, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return err
	}

	res, err := d.gateway.TouchFile(ctx, &provider.TouchFileRequest{Ref: newRef})
	switch {
	case err != nil:
		return err
	case res.Status.Code != rpcv1beta1.Code_CODE_OK:
		// TODO: better error handling
		return errtypes.InternalError(res.Status.Message)
	}

	return nil
}

func (d *driver) Delete(ctx context.Context, ref *provider.Reference) error {
	newRef, _, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return err
	}

	res, err := d.gateway.Delete(ctx, &provider.DeleteRequest{Ref: newRef})
	switch {
	case err != nil:
		return err
	case res.Status.Code != rpcv1beta1.Code_CODE_OK:
		// TODO: better error handling
		return errtypes.InternalError(res.Status.Message)
	}

	return nil
}

func (d *driver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	resolvedOldRef, _, err := d.translateOCMShareToCS3Ref(ctx, oldRef)
	if err != nil {
		return err
	}

	resolvedNewRef, _, err := d.translateOCMShareToCS3Ref(ctx, newRef)
	if err != nil {
		return err
	}

	res, err := d.gateway.Move(ctx, &provider.MoveRequest{Source: resolvedOldRef, Destination: resolvedNewRef})
	switch {
	case err != nil:
		return err
	case res.Status.Code != rpcv1beta1.Code_CODE_OK:
		// TODO: better error handling
		return errtypes.InternalError(res.Status.Message)
	}

	return nil
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

	fmt.Println("****************** OP FROM USER =", userRes.User)

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

func (d *driver) GetMD(ctx context.Context, ref *provider.Reference, _ []string) (*provider.ResourceInfo, error) {
	fmt.Println("*********************** ref=", ref)
	newRef, share, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return nil, err
	}
	fmt.Println("*********************** new ref=", newRef)

	var info *provider.ResourceInfo
	if err := d.opFromUser(ctx, share.Creator, func(userCtx context.Context) error {
		info, err = d.stat(userCtx, newRef)
		fmt.Println("********************* stat from user = ", info, err)
		return err
	}); err != nil {
		return nil, err
	}

	if err := d.augmentResourceInfo(ctx, info, share); err != nil {
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
	fixResourceInfo(info, shareInfo, share)
	return nil
}

func fixResourceInfo(info, shareInfo *provider.ResourceInfo, share *ocmv1beta1.Share) {
	// fix path
	relPath := makeRelative(strings.TrimPrefix(info.Path, shareInfo.Path))
	info.Path = filepath.Join("/", share.Token, relPath)

	// fix resource id
	info.Id = &provider.ResourceId{
		StorageId: fmt.Sprintf("%s:%s", share.Token, relPath),
	}
	// TODO: we should filter the the permissions also
}

func (d *driver) ListFolder(ctx context.Context, ref *provider.Reference, _ []string) ([]*provider.ResourceInfo, error) {
	newRef, share, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return nil, err
	}

	var infos []*provider.ResourceInfo
	if err := d.opFromUser(ctx, share.Creator, func(userCtx context.Context) error {
		lstRes, err := d.gateway.ListContainer(userCtx, &provider.ListContainerRequest{Ref: newRef})
		switch {
		case err != nil:
			return err
		case lstRes.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
			return errtypes.NotFound(ref.String())
		case lstRes.Status.Code != rpcv1beta1.Code_CODE_OK:
			return errtypes.InternalError(lstRes.Status.Message)
		}
		infos = lstRes.Infos
		return nil
	}); err != nil {
		return nil, err
	}

	shareInfo, err := d.stat(ctx, &provider.Reference{ResourceId: share.ResourceId})
	if err != nil {
		return nil, err
	}

	for _, info := range infos {
		fixResourceInfo(info, shareInfo, share)
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
	newRef, share, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return err
	}

	return d.opFromUser(ctx, share.Creator, func(userCtx context.Context) error {
		initRes, err := d.gateway.InitiateFileUpload(userCtx, &provider.InitiateFileUploadRequest{Ref: newRef})
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

		httpReq, err := rhttp.NewRequest(userCtx, http.MethodPut, endpoint, content)
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
	newRef, share, err := d.translateOCMShareToCS3Ref(ctx, ref)
	if err != nil {
		return nil, err
	}

	var r io.ReadCloser

	if err := d.opFromUser(ctx, share.Creator, func(userCtx context.Context) error {
		initRes, err := d.gateway.InitiateFileDownload(userCtx, &provider.InitiateFileDownloadRequest{Ref: newRef})
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

		httpReq, err := rhttp.NewRequest(userCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		httpReq.Header.Set(datagateway.TokenTransportHeader, token)

		httpRes, err := http.DefaultClient.Do(httpReq)
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

func (d *driver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
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
