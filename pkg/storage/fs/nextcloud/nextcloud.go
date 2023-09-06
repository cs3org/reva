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

package nextcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/cs3org/reva/pkg/utils/list"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("nextcloud", New)
}

// StorageDriverConfig is the configuration struct for a NextcloudStorageDriver.
type StorageDriverConfig struct {
	EndPoint     string `mapstructure:"endpoint"` // e.g. "http://nc/apps/sciencemesh/~alice/"
	SharedSecret string `mapstructure:"shared_secret"`
	MockHTTP     bool   `mapstructure:"mock_http"`
}

// StorageDriver implements the storage.FS interface
// and connects with a StorageDriver server as its backend.
type StorageDriver struct {
	endPoint     string
	sharedSecret string
	client       *http.Client
}

// MDFromEFSS is returned by the EFSS to represent a resource.
type MDFromEFSS struct {
	Type int `json:"type"`
	ID   struct {
		OpaqueID string `json:"opaque_id"`
	} `json:"id"`
	Checksum struct {
		Type int    `json:"type"`
		Sum  string `json:"sum"`
	} `json:"checksum"`
	Etag     string `json:"etag"`
	MimeType string `json:"mime_type"`
	Mtime    struct {
		Seconds int `json:"seconds"`
	} `json:"mtime"`
	Path              string `json:"path"`
	Permissions       int    `json:"permissions"`
	Size              int    `json:"size"`
	CanonicalMetadata struct {
		Target any `json:"target"`
	} `json:"canonical_metadata"`
	ArbitraryMetadata struct {
		Metadata struct {
			Placeholder string `json:".placeholder"`
		} `json:"metadata"`
	} `json:"arbitrary_metadata"`
	Owner struct {
		OpaqueID string `json:"opaque_id"`
		Idp      string `json:"idp"`
	} `json:"owner"`
}

// New returns an implementation to of the storage.FS interface that talks to
// a Nextcloud instance over http.
func New(ctx context.Context, m map[string]interface{}) (storage.FS, error) {
	var c StorageDriverConfig
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return NewStorageDriver(&c)
}

// NewStorageDriver returns a new NextcloudStorageDriver.
func NewStorageDriver(c *StorageDriverConfig) (*StorageDriver, error) {
	var client *http.Client
	if c.MockHTTP {
		// called := make([]string, 0)
		// nextcloudServerMock := GetNextcloudServerMock(&called)
		// client, _ = TestingHTTPClient(nextcloudServerMock)

		// This is only used by the integration tests:
		// (unit tests will call SetHTTPClient later):
		called := make([]string, 0)
		h := GetNextcloudServerMock(&called)
		client, _ = TestingHTTPClient(h)
		// FIXME: defer teardown()
	} else {
		if len(c.EndPoint) == 0 {
			return nil, errors.New("Please specify 'endpoint' in '[grpc.services.storageprovider.drivers.nextcloud]'")
		}
		client = &http.Client{}
	}
	return &StorageDriver{
		endPoint:     c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
		sharedSecret: c.SharedSecret,
		client:       client,
	}, nil
}

func getUser(ctx context.Context) (*user.User, error) {
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "nextcloud storage driver: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

// SetHTTPClient sets the HTTP client.
func (nc *StorageDriver) SetHTTPClient(c *http.Client) {
	nc.client = c
}

func (nc *StorageDriver) doRaw(ctx context.Context, req *http.Request) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)

	log.Debug().Str("method", req.Method).Str("url", req.URL.String()).Msg("sending request to EFSS API")
	resp, err := nc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNotFound {
		return nil, fmt.Errorf("unexpected response code %d from EFSS API", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, errtypes.NotFound("")
	}

	return resp.Body, nil
}

func (nc *StorageDriver) prepareRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	user, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	url, _ := url.JoinPath(nc.endPoint, "~"+user.Id.OpaqueId, "/api/storage", path)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}
	req.Header.Set("X-Reva-Secret", nc.sharedSecret)
	return req, nil
}

// A convenient method that internally uses doRaw and automatically marshals and unmarshals
// the body request and the body response as json if not nil.
func (nc *StorageDriver) do(ctx context.Context, method, path string, bodyObj, targetObj any) error {
	var body []byte
	var err error
	if bodyObj != nil {
		body, err = json.Marshal(bodyObj)
		if err != nil {
			return errors.Wrap(err, "error marshalling body to json")
		}
	}

	req, err := nc.prepareRequest(ctx, method, path, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := nc.doRaw(ctx, req)
	if err != nil {
		return err
	}
	defer res.Close()

	if targetObj != nil {
		if err := json.NewDecoder(res).Decode(targetObj); err != nil {
			return errors.Wrapf(err, "response %s from EFSS API does not match target type %T", res, targetObj)
		}
	}
	return nil
}

// GetHome as defined in the storage.FS interface.
func (nc *StorageDriver) GetHome(ctx context.Context) (string, error) {
	var path string
	err := nc.do(ctx, http.MethodPost, "GetHome", nil, &path)
	return path, err
}

// CreateHome as defined in the storage.FS interface.
func (nc *StorageDriver) CreateHome(ctx context.Context) error {
	return nc.do(ctx, http.MethodPost, "CreateHome", nil, nil)
}

// CreateDir as defined in the storage.FS interface.
func (nc *StorageDriver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	return nc.do(ctx, http.MethodPost, "CreateDir", ref, nil)
}

// TouchFile as defined in the storage.FS interface.
func (nc *StorageDriver) TouchFile(ctx context.Context, ref *provider.Reference) error {
	return fmt.Errorf("unimplemented: TouchFile")
}

// Delete as defined in the storage.FS interface.
func (nc *StorageDriver) Delete(ctx context.Context, ref *provider.Reference) error {
	return nc.do(ctx, http.MethodPost, "Delete", ref, nil)
}

type MoveRequest struct {
	OldRef *provider.Reference `json:"oldRef"`
	NewRef *provider.Reference `json:"newRef"`
}

// Move as defined in the storage.FS interface.
func (nc *StorageDriver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	return nc.do(ctx, http.MethodPost, "Move", MoveRequest{OldRef: oldRef, NewRef: newRef}, nil)
}

func resInfoFromEFSS(respObj *MDFromEFSS) *provider.ResourceInfo {
	// Parse the JSON struct returned by the PHP SM app into a ResourceInfo,
	// translating the permissions from ownCloud DB value to CS3 and ignoring non relevant fields.
	return &provider.ResourceInfo{
		Id: &provider.ResourceId{
			OpaqueId: respObj.ID.OpaqueID,
		},
		Type: provider.ResourceType(respObj.Type),
		Checksum: &provider.ResourceChecksum{
			Type: provider.ResourceChecksumType(respObj.Checksum.Type),
			Sum:  respObj.Checksum.Sum,
		},
		Etag:     respObj.Etag,
		MimeType: respObj.MimeType,
		Mtime: &typesv1beta1.Timestamp{
			Seconds: uint64(respObj.Mtime.Seconds),
		},
		Path: respObj.Path,
		PermissionSet: conversions.RoleFromOCSPermissions(
			conversions.Permissions(respObj.Permissions)).CS3ResourcePermissions(),
		Size: uint64(respObj.Size),
		Owner: &user.UserId{
			Idp:      respObj.Owner.Idp,
			OpaqueId: respObj.Owner.OpaqueID,
			Type:     user.UserType_USER_TYPE_PRIMARY,
		},
	}
}

type GetMDRequest struct {
	Ref    *provider.Reference `json:"ref"`
	MdKeys []string            `json:"mdKeys"`
}

// GetMD as defined in the storage.FS interface.
func (nc *StorageDriver) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	var target MDFromEFSS
	err := nc.do(ctx, http.MethodPost, "GetMD", GetMDRequest{Ref: ref, MdKeys: mdKeys}, &target)
	if err != nil {
		return nil, err
	}
	return resInfoFromEFSS(&target), nil
}

// ListFolder as defined in the storage.FS interface.
func (nc *StorageDriver) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	var l []*MDFromEFSS
	err := nc.do(ctx, http.MethodPost, "ListFolder", GetMDRequest{Ref: ref, MdKeys: mdKeys}, &l)
	if err != nil {
		return nil, err
	}

	return list.Map(l, resInfoFromEFSS), nil
}

type InitiateUploadRequest struct {
	Ref          *provider.Reference `json:"ref"`
	UploadLength int64               `json:"uploadLength"`
	Metadata     map[string]string   `json:"metadata"`
}

// InitiateUpload as defined in the storage.FS interface.
func (nc *StorageDriver) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	var res map[string]string
	err := nc.do(ctx, http.MethodPost, "InitiateUpload", InitiateUploadRequest{Ref: ref, UploadLength: uploadLength, Metadata: metadata}, &res)
	return res, err
}

// Upload as defined in the storage.FS interface.
func (nc *StorageDriver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	req, err := nc.prepareRequest(ctx, http.MethodPut, filepath.Join("/Upload/home", ref.Path), r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	b, err := nc.doRaw(ctx, req)
	if err != nil {
		return err
	}
	defer b.Close()
	_, _ = io.ReadAll(b)
	return nil
}

// Download as defined in the storage.FS interface.
func (nc *StorageDriver) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	req, err := nc.prepareRequest(ctx, http.MethodGet, filepath.Join("/Download", ref.Path), nil)
	if err != nil {
		return nil, err
	}
	return nc.doRaw(ctx, req)
}

// ListRevisions as defined in the storage.FS interface.
func (nc *StorageDriver) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	var versions []*provider.FileVersion
	err := nc.do(ctx, http.MethodPost, "ListRevisions", ref, &versions)
	return versions, err
}

// DownloadRevision as defined in the storage.FS interface.
func (nc *StorageDriver) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	req, err := nc.prepareRequest(ctx, http.MethodGet, filepath.Join("/DownloadRevision/", key, ref.Path), nil)
	if err != nil {
		return nil, err
	}
	return nc.doRaw(ctx, req)
}

type RestoreRevisionRequest struct {
	Ref *provider.Reference `json:"ref"`
	Key string              `json:"key"`
}

// RestoreRevision as defined in the storage.FS interface.
func (nc *StorageDriver) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	return nc.do(ctx, http.MethodPost, "RestoreRevision", RestoreRevisionRequest{Ref: ref, Key: key}, nil)
}

type ListRecycleRequest struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

// ListRecycle as defined in the storage.FS interface.
func (nc *StorageDriver) ListRecycle(ctx context.Context, basePath, key, relativePath string) ([]*provider.RecycleItem, error) {
	var items []*provider.RecycleItem
	err := nc.do(ctx, http.MethodPost, "ListRecycle", ListRecycleRequest{Key: key, Path: relativePath}, &items)
	return items, err
}

type RestoreRecycleItemRequest struct {
	Key        string              `json:"key"`
	Path       string              `json:"path"`
	RestoreRef *provider.Reference `json:"restoreRef"`
}

// RestoreRecycleItem as defined in the storage.FS interface.
func (nc *StorageDriver) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return nc.do(ctx, http.MethodPost, "RestoreRecycleItem", RestoreRecycleItemRequest{Key: key, Path: relativePath, RestoreRef: restoreRef}, nil)
}

type PurgeRecycleItemRequest struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

// PurgeRecycleItem as defined in the storage.FS interface.
func (nc *StorageDriver) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return nc.do(ctx, http.MethodPost, "PurgeRecycleItem", PurgeRecycleItemRequest{Key: key, Path: relativePath}, nil)
}

// EmptyRecycle as defined in the storage.FS interface.
func (nc *StorageDriver) EmptyRecycle(ctx context.Context) error {
	return nc.do(ctx, http.MethodPost, "EmptyRecycle", nil, nil)
}

// GetPathByID as defined in the storage.FS interface.
func (nc *StorageDriver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	d, _ := json.Marshal(id)
	req, err := nc.prepareRequest(ctx, http.MethodPost, "GetPathByID", bytes.NewBuffer(d))
	if err != nil {
		return "", err
	}

	body, err := nc.doRaw(ctx, req)
	if err != nil {
		return "", err
	}
	defer body.Close()

	b, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type GrantRequest struct {
	Ref *provider.Reference `json:"ref"`
	G   *provider.Grant     `json:"g"`
}

// AddGrant as defined in the storage.FS interface.
func (nc *StorageDriver) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return nc.do(ctx, http.MethodPost, "AddGrant", GrantRequest{Ref: ref, G: g}, nil)
}

// DenyGrant as defined in the storage.FS interface.
func (nc *StorageDriver) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	return nc.do(ctx, http.MethodPost, "DenyGrant", GrantRequest{Ref: ref, G: &provider.Grant{Grantee: g}}, nil)
}

// RemoveGrant as defined in the storage.FS interface.
func (nc *StorageDriver) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return nc.do(ctx, http.MethodPost, "RemoveGrant", GrantRequest{Ref: ref, G: g}, nil)
}

// UpdateGrant as defined in the storage.FS interface.
func (nc *StorageDriver) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return nc.do(ctx, http.MethodPost, "UpdateGrant", GrantRequest{Ref: ref, G: g}, nil)
}

// ListGrants as defined in the storage.FS interface.
func (nc *StorageDriver) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	var respMapArr []map[string]any
	err := nc.do(ctx, http.MethodPost, "ListGrants", ref, &respMapArr)
	if err != nil {
		return nil, err
	}

	grants := make([]*provider.Grant, len(respMapArr))
	for i := 0; i < len(respMapArr); i++ {
		granteeMap := respMapArr[i]["grantee"].(map[string]interface{})
		granteeIDMap := granteeMap["Id"].(map[string]interface{})
		granteeIDUserIDMap := granteeIDMap["UserId"].(map[string]interface{})

		// if (granteeMap["Id"])
		permsMap := respMapArr[i]["permissions"].(map[string]interface{})
		grants[i] = &provider.Grant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER, // FIXME: support groups too
				Id: &provider.Grantee_UserId{
					UserId: &user.UserId{
						Idp:      granteeIDUserIDMap["idp"].(string),
						OpaqueId: granteeIDUserIDMap["opaque_id"].(string),
						Type:     user.UserType(granteeIDUserIDMap["type"].(float64)),
					},
				},
			},
			Permissions: &provider.ResourcePermissions{
				AddGrant:             permsMap["add_grant"].(bool),
				CreateContainer:      permsMap["create_container"].(bool),
				Delete:               permsMap["delete"].(bool),
				GetPath:              permsMap["get_path"].(bool),
				GetQuota:             permsMap["get_quota"].(bool),
				InitiateFileDownload: permsMap["initiate_file_download"].(bool),
				InitiateFileUpload:   permsMap["initiate_file_upload"].(bool),
				ListGrants:           permsMap["list_grants"].(bool),
				ListContainer:        permsMap["list_container"].(bool),
				ListFileVersions:     permsMap["list_file_versions"].(bool),
				ListRecycle:          permsMap["list_recycle"].(bool),
				Move:                 permsMap["move"].(bool),
				RemoveGrant:          permsMap["remove_grant"].(bool),
				PurgeRecycle:         permsMap["purge_recycle"].(bool),
				RestoreFileVersion:   permsMap["restore_file_version"].(bool),
				RestoreRecycleItem:   permsMap["restore_recycle_item"].(bool),
				Stat:                 permsMap["stat"].(bool),
				UpdateGrant:          permsMap["update_grant"].(bool),
			},
		}
	}
	return grants, nil
}

// GetQuota as defined in the storage.FS interface.
func (nc *StorageDriver) GetQuota(ctx context.Context, _ *provider.Reference) (uint64, uint64, error) {
	var quotaRes struct {
		TotalBytes uint64 `json:"totalBytes"`
		UsedBytes  uint64 `json:"usedBytes"`
	}
	err := nc.do(ctx, http.MethodPost, "GetQuota", nil, &quotaRes)
	if err != nil {
		return 0, 0, err
	}
	return quotaRes.TotalBytes, quotaRes.UsedBytes, nil
}

type CreateReferenceRequest struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

// CreateReference as defined in the storage.FS interface.
func (nc *StorageDriver) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return nc.do(ctx, http.MethodPost, "CreateReference", CreateReferenceRequest{Path: path, URL: targetURI.String()}, nil)
}

// Shutdown as defined in the storage.FS interface. Obviously we don't shutdown the EFSS...
func (nc *StorageDriver) Shutdown(ctx context.Context) error {
	return nil
}

type SetArbitraryMetadataRequest struct {
	Ref *provider.Reference         `json:"ref"`
	Md  *provider.ArbitraryMetadata `json:"md"`
}

// SetArbitraryMetadata as defined in the storage.FS interface.
func (nc *StorageDriver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return nc.do(ctx, http.MethodPost, "SetArbitraryMetadata", SetArbitraryMetadataRequest{Ref: ref, Md: md}, nil)
}

type UnsetArbitraryMetadataRequest struct {
	Ref  *provider.Reference `json:"ref"`
	Keys []string            `json:"keys"`
}

// UnsetArbitraryMetadata as defined in the storage.FS interface.
func (nc *StorageDriver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return nc.do(ctx, http.MethodPost, "UnsetArbitraryMetadata", UnsetArbitraryMetadataRequest{Ref: ref, Keys: keys}, nil)
}

// GetLock returns an existing lock on the given reference.
func (nc *StorageDriver) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

// SetLock puts a lock on the given reference.
func (nc *StorageDriver) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("unimplemented")
}

// RefreshLock refreshes an existing lock on the given reference.
func (nc *StorageDriver) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
	return errtypes.NotSupported("unimplemented")
}

// Unlock removes an existing lock from the given reference.
func (nc *StorageDriver) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("unimplemented")
}

// ListStorageSpaces as defined in the storage.FS interface.
func (nc *StorageDriver) ListStorageSpaces(ctx context.Context, f []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

// CreateStorageSpace creates a storage space.
func (nc *StorageDriver) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

// UpdateStorageSpace updates a storage space.
func (nc *StorageDriver) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("unimplemented")
}
