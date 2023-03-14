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

package ocm

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

func init() {
	registry.Register("ocmreceived", New)
}

type driver struct {
	c       *config
	gateway gateway.GatewayAPIClient
}

type config struct {
	GatewaySVC string
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

func shareInfoFromPath(path string) (*ocmpb.ShareId, string) {
	// the path is of the type /share_id[/rel_path]
	shareID, rel := router.ShiftPath(path)
	return &ocmpb.ShareId{OpaqueId: shareID}, rel
}

func shareInfoFromReference(ref *provider.Reference) (*ocmpb.ShareId, string) {
	if ref.ResourceId == nil {
		return shareInfoFromPath(ref.Path)
	}

	s := strings.SplitN(ref.ResourceId.OpaqueId, ":", 2)
	shareID := &ocmpb.ShareId{OpaqueId: s[0]}
	var path string
	if len(s) == 2 {
		path = s[1]
	}
	path = filepath.Join(path, ref.Path)

	return shareID, path
}

func (d *driver) getWebDAVFromShare(ctx context.Context, shareID *ocmpb.ShareId) (*ocmpb.ReceivedShare, string, string, error) {
	// TODO: we may want to cache the share
	res, err := d.gateway.GetReceivedOCMShare(ctx, &ocmpb.GetReceivedOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: shareID,
			},
		},
	})
	if err != nil {
		return nil, "", "", err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, "", "", errtypes.NotFound("share not found")
		}
		return nil, "", "", errtypes.InternalError(res.Status.Message)
	}

	dav, ok := getWebDAVProtocol(res.Share.Protocols)
	if !ok {
		return nil, "", "", errtypes.NotFound("share does not contain a WebDAV endpoint")
	}

	return res.Share, dav.Uri, dav.SharedSecret, nil
}

func getWebDAVProtocol(protocols []*ocmpb.Protocol) (*ocmpb.WebDAVProtocol, bool) {
	for _, p := range protocols {
		if dav, ok := p.Term.(*ocmpb.Protocol_WebdavOptions); ok {
			return dav.WebdavOptions, true
		}
	}
	return nil, false
}

func (d *driver) webdavClient(ctx context.Context, ref *provider.Reference) (*gowebdav.Client, *ocmpb.ReceivedShare, string, error) {
	id, rel := shareInfoFromReference(ref)

	share, endpoint, secret, err := d.getWebDAVFromShare(ctx, id)
	if err != nil {
		return nil, nil, "", err
	}

	endpoint, err = url.PathUnescape(endpoint)
	if err != nil {
		return nil, nil, "", err
	}

	// FIXME: it's still not clear from the OCM APIs how to use the shared secret
	// will use as a token in the bearer authentication as this is the reva implementation
	c := gowebdav.NewClient(endpoint, "", "")
	c.SetHeader("Authorization", "Bearer "+secret)

	return c, share, rel, nil
}

func (d *driver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return err
	}
	return client.MkdirAll(rel, 0)
}

func (d *driver) Delete(ctx context.Context, ref *provider.Reference) error {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return err
	}
	return client.RemoveAll(rel)
}

func (d *driver) TouchFile(ctx context.Context, ref *provider.Reference) error {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return err
	}
	return client.Write(rel, []byte{}, 0)
}

func (d *driver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	client, _, relOld, err := d.webdavClient(ctx, oldRef)
	if err != nil {
		return err
	}
	_, relNew := shareInfoFromReference(newRef)

	return client.Rename(relOld, relNew, false)
}

func getResourceInfo(shareID *ocmpb.ShareId, relPath string) *provider.ResourceId {
	return &provider.ResourceId{
		OpaqueId: fmt.Sprintf("%s:%s", shareID.OpaqueId, relPath),
	}
}

func getPathFromShareIDAndRelPath(shareID *ocmpb.ShareId, relPath string) string {
	return filepath.Join("/", shareID.OpaqueId, relPath)
}

func convertStatToResourceInfo(f fs.FileInfo, share *ocmpb.ReceivedShare, relPath string) *provider.ResourceInfo {
	t := provider.ResourceType_RESOURCE_TYPE_FILE
	if f.IsDir() {
		t = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}

	var name string
	if share.ResourceType == provider.ResourceType_RESOURCE_TYPE_FILE {
		name = share.Name
	} else {
		name = f.Name()
	}

	webdav, _ := getWebDAVProtocol(share.Protocols)

	return &provider.ResourceInfo{
		Type:     t,
		Id:       getResourceInfo(share.Id, relPath),
		MimeType: mime.Detect(f.IsDir(), f.Name()),
		Path:     getPathFromShareIDAndRelPath(share.Id, relPath),
		Name:     name,
		Size:     uint64(f.Size()),
		Mtime: &typepb.Timestamp{
			Seconds: uint64(f.ModTime().Unix()),
		},
		Owner:         share.Creator,
		PermissionSet: webdav.Permissions.Permissions,
		Checksum: &provider.ResourceChecksum{
			Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID,
		},
	}
}

func (d *driver) GetMD(ctx context.Context, ref *provider.Reference, _ []string) (*provider.ResourceInfo, error) {
	client, share, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, err
	}

	info, err := client.Stat(rel)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, errtypes.NotFound(ref.GetPath())
		}
		return nil, err
	}

	return convertStatToResourceInfo(info, share, rel), nil
}

func (d *driver) ListFolder(ctx context.Context, ref *provider.Reference, _ []string) ([]*provider.ResourceInfo, error) {
	client, share, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, err
	}

	list, err := client.ReadDir(rel)
	if err != nil {
		return nil, err
	}

	res := make([]*provider.ResourceInfo, 0, len(list))
	for _, r := range list {
		res = append(res, convertStatToResourceInfo(r, share, filepath.Join(rel, r.Name())))
	}
	return res, nil
}

func (d *driver) InitiateUpload(ctx context.Context, ref *provider.Reference, _ int64, _ map[string]string) (map[string]string, error) {
	shareID, rel := shareInfoFromReference(ref)
	p := getPathFromShareIDAndRelPath(shareID, rel)

	return map[string]string{
		"simple": p,
	}, nil
}

func (d *driver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return err
	}

	client.SetHeader(ocdav.HeaderUploadLength, "-1")
	return client.WriteStream(rel, r, 0)
}

func (d *driver) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, err
	}

	return client.ReadStream(rel)
}

func (d *driver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	shareID, rel := shareInfoFromReference(&provider.Reference{
		ResourceId: id,
	})
	return getPathFromShareIDAndRelPath(shareID, rel), nil
}

func (d *driver) Shutdown(ctx context.Context) error {
	return nil
}

func (d *driver) CreateHome(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("operation not supported")
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
