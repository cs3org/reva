// Copyright 2018-2021 CERN
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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("nextcloud", New)
}

// StorageDriverConfig is the configuration struct for a NextcloudStorageDriver
type StorageDriverConfig struct {
	EndPoint string `mapstructure:"end_point"` // e.g. "http://nc/apps/sciencemesh/~alice/"
	MockHTTP bool   `mapstructure:"mock_http"`
}

// StorageDriver implements the storage.FS interface
// and connects with a StorageDriver server as its backend
type StorageDriver struct {
	endPoint string
	client   *http.Client
}

func parseConfig(m map[string]interface{}) (*StorageDriverConfig, error) {
	c := &StorageDriverConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talks to
// a Nextcloud instance over http.
func New(m map[string]interface{}) (storage.FS, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return NewStorageDriver(conf)
}

// CreateStorageSpace creates a storage space
func (nc *StorageDriver) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, fmt.Errorf("unimplemented: CreateStorageSpace")
}

// NewStorageDriver returns a new NextcloudStorageDriver
func NewStorageDriver(c *StorageDriverConfig) (*StorageDriver, error) {
	var client *http.Client
	if c.MockHTTP {
		nextcloudServerMock := GetNextcloudServerMock()
		client, _ = TestingHTTPClient(nextcloudServerMock)
	} else {
		client = &http.Client{}
	}
	return &StorageDriver{
		endPoint: c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
		client:   client,
	}, nil
}

// Action describes a REST request to forward to the Nextcloud backend
type Action struct {
	verb string
	argS string
}

func getUser(ctx context.Context) (*user.User, error) {
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "nextcloud storage driver: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

// SetHTTPClient sets the HTTP client
func (nc *StorageDriver) SetHTTPClient(c *http.Client) {
	nc.client = c
}

func (nc *StorageDriver) doUpload(r io.ReadCloser) error {
	filePath := "test.txt"

	// initialize http client
	client := &http.Client{}
	url := nc.endPoint + "Upload/" + filePath
	req, err := http.NewRequest(http.MethodPut, url, r)
	if err != nil {
		panic(err)
	}

	// set the request header Content-Type for the upload
	// FIXME: get the actual content type from somewhere
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	return err
}

func (nc *StorageDriver) do(ctx context.Context, a Action) (int, []byte, error) {
	log := appctx.GetLogger(ctx)
	user, err := getUser(ctx)
	if err != nil {
		return 0, nil, err
	}
	url := nc.endPoint + "~" + user.Username + "/api/" + a.verb
	log.Info().Msgf("nc.do %s", url)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := nc.client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	log.Info().Msgf("nc.do response %d %s", resp.StatusCode, body)
	return resp.StatusCode, body, nil
}

// GetHome as defined in the storage.FS interface
func (nc *StorageDriver) GetHome(ctx context.Context) (string, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("GetHome")

	_, respBody, err := nc.do(ctx, Action{"GetHome", ""})
	return string(respBody), err
}

// CreateHome as defined in the storage.FS interface
func (nc *StorageDriver) CreateHome(ctx context.Context) error {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("CreateHome")

	_, _, err := nc.do(ctx, Action{"CreateHome", ""})
	return err
}

// CreateDir as defined in the storage.FS interface
func (nc *StorageDriver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("CreateDir %s", bodyStr)

	_, _, err = nc.do(ctx, Action{"CreateDir", string(bodyStr)})
	return err
}

// Delete as defined in the storage.FS interface
func (nc *StorageDriver) Delete(ctx context.Context, ref *provider.Reference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("Delete %s", bodyStr)

	_, _, err = nc.do(ctx, Action{"Delete", string(bodyStr)})
	return err
}

// Move as defined in the storage.FS interface
func (nc *StorageDriver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	data := make(map[string]string)
	data["from"] = oldRef.Path
	data["to"] = newRef.Path
	bodyStr, _ := json.Marshal(data)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("Move %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"Move", string(bodyStr)})
	return err
}

// GetMD as defined in the storage.FS interface
func (nc *StorageDriver) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	bodyStr, err := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("GetMD %s", bodyStr)

	if err != nil {
		return nil, err
	}
	status, body, err := nc.do(ctx, Action{"GetMD", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, errtypes.NotFound("")
	}
	var respMap map[string]interface{}
	err = json.Unmarshal(body, &respMap)
	if err != nil {
		return nil, err
	}
	size := int(respMap["size"].(float64))
	mdMap, ok := respMap["metadata"].(map[string]interface{})
	mdMapString := make(map[string]string)
	if ok {
		for key, value := range mdMap {
			mdMapString[key] = value.(string)
		}
	}
	md := &provider.ResourceInfo{
		Opaque:            &types.Opaque{},
		Type:              provider.ResourceType_RESOURCE_TYPE_FILE,
		Id:                &provider.ResourceId{OpaqueId: "fileid-" + url.QueryEscape(ref.Path)},
		Checksum:          &provider.ResourceChecksum{},
		Etag:              "some-etag",
		MimeType:          "application/octet-stream",
		Mtime:             &types.Timestamp{Seconds: 1234567890},
		Path:              ref.Path,
		PermissionSet:     &provider.ResourcePermissions{},
		Size:              uint64(size),
		Owner:             nil,
		Target:            "",
		CanonicalMetadata: &provider.CanonicalMetadata{},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata:             mdMapString,
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []byte{},
			XXX_sizecache:        0,
		},
		XXX_NoUnkeyedLiteral: struct{}{},
		XXX_unrecognized:     []byte{},
		XXX_sizecache:        0,
	}

	return md, nil
}

// ListFolder as defined in the storage.FS interface
func (nc *StorageDriver) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	bodyStr, err := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("LisfFolder %s", bodyStr)
	if err != nil {
		return nil, err
	}
	status, body, err := nc.do(ctx, Action{"ListFolder", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, errtypes.NotFound("")
	}
	var bodyArr []string
	err = json.Unmarshal(body, &bodyArr)
	var infos = make([]*provider.ResourceInfo, len(bodyArr))
	for i := 0; i < len(bodyArr); i++ {
		infos[i] = &provider.ResourceInfo{
			Opaque:               &types.Opaque{},
			Type:                 provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			Id:                   &provider.ResourceId{OpaqueId: "fileid-" + url.QueryEscape(bodyArr[i])},
			Checksum:             &provider.ResourceChecksum{},
			Etag:                 "some-etag",
			MimeType:             "application/octet-stream",
			Mtime:                &types.Timestamp{Seconds: 1234567890},
			Path:                 "/subdir", // FIXME: bodyArr[i],
			PermissionSet:        &provider.ResourcePermissions{},
			Size:                 0,
			Owner:                &user.UserId{OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"},
			Target:               "",
			CanonicalMetadata:    &provider.CanonicalMetadata{},
			ArbitraryMetadata:    nil,
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []byte{},
			XXX_sizecache:        0,
		}
	}
	return infos, err
}

// InitiateUpload as defined in the storage.FS interface
func (nc *StorageDriver) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("InitiateUpload %s", bodyStr)

	_, respBody, err := nc.do(ctx, Action{"InitiateUpload", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	respMap := make(map[string]string)
	err = json.Unmarshal(respBody, &respMap)
	if err != nil {
		return nil, err
	}
	return respMap, err
}

// Upload as defined in the storage.FS interface
func (nc *StorageDriver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("Upload %s", bodyStr)

	err := nc.doUpload(r)
	if err != nil {
		return err
	}
	_, _, err = nc.do(ctx, Action{"Upload", string(bodyStr)})
	return err
}

// Download as defined in the storage.FS interface
func (nc *StorageDriver) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("Download %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"Download", string(bodyStr)})
	return nil, err
}

// ListRevisions as defined in the storage.FS interface
func (nc *StorageDriver) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("ListRevisions %s", bodyStr)

	_, respBody, err := nc.do(ctx, Action{"ListRevisions", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	var m []int
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return nil, err
	}
	revs := make([]*provider.FileVersion, len(m))
	for i := 0; i < len(m); i++ {
		revs[i] = &provider.FileVersion{
			Opaque:               &types.Opaque{},
			Key:                  fmt.Sprint(i),
			Size:                 uint64(m[i]),
			Mtime:                0,
			Etag:                 "",
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []byte{},
			XXX_sizecache:        0,
		}
	}
	return revs, err
}

// DownloadRevision as defined in the storage.FS interface
func (nc *StorageDriver) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("DownloadRevision %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"DownloadRevision", string(bodyStr)})
	return nil, err
}

// RestoreRevision as defined in the storage.FS interface
func (nc *StorageDriver) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("RestoreRevision %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"RestoreRevision", string(bodyStr)})
	return err
}

// ListRecycle as defined in the storage.FS interface
func (nc *StorageDriver) ListRecycle(ctx context.Context, key string, path string) ([]*provider.RecycleItem, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("ListRecycle")
	_, respBody, err := nc.do(ctx, Action{"ListRecycle", ""})

	if err != nil {
		return nil, err
	}
	var m []string
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return nil, err
	}
	items := make([]*provider.RecycleItem, len(m))
	for i := 0; i < len(m); i++ {
		items[i] = &provider.RecycleItem{
			Opaque: &types.Opaque{},
			Type:   0,
			Key:    "",
			Ref: &provider.Reference{
				ResourceId:           &provider.ResourceId{},
				Path:                 m[i],
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     []byte{},
				XXX_sizecache:        0,
			},
			Size:                 0,
			DeletionTime:         &types.Timestamp{},
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []byte{},
			XXX_sizecache:        0,
		}
	}
	return items, err
}

// RestoreRecycleItem as defined in the storage.FS interface
func (nc *StorageDriver) RestoreRecycleItem(ctx context.Context, key string, path string, restoreRef *provider.Reference) error {
	bodyStr, _ := json.Marshal(restoreRef)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("RestoreRecycleItem %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"RestoreRecycleItem", string(bodyStr)})
	return err
}

// PurgeRecycleItem as defined in the storage.FS interface
func (nc *StorageDriver) PurgeRecycleItem(ctx context.Context, key string, path string) error {
	bodyStr, _ := json.Marshal(key)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("PurgeRecycleItem %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"PurgeRecycleItem", string(bodyStr)})
	return err
}

// EmptyRecycle as defined in the storage.FS interface
func (nc *StorageDriver) EmptyRecycle(ctx context.Context) error {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("EmptyRecycle")

	_, _, err := nc.do(ctx, Action{"EmptyRecycle", ""})
	return err
}

// GetPathByID as defined in the storage.FS interface
func (nc *StorageDriver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	bodyStr, _ := json.Marshal(id)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("GetPathByID %s", bodyStr)

	_, respBody, err := nc.do(ctx, Action{"GetPathByID", string(bodyStr)})
	return string(respBody), err
}

// AddGrant as defined in the storage.FS interface
func (nc *StorageDriver) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("AggGrant %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"AddGrant", string(bodyStr)})
	return err
}

// RemoveGrant as defined in the storage.FS interface
func (nc *StorageDriver) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("RemoveGrant %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"RemoveGrant", string(bodyStr)})
	return err
}

// DenyGrant as defined in the storage.FS interface
func (nc *StorageDriver) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("DenyGrant %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"DenyGrant", string(bodyStr)})
	return err
}

// UpdateGrant as defined in the storage.FS interface
func (nc *StorageDriver) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("UpdateGrant %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"UpdateGrant", string(bodyStr)})
	return err
}

// ListGrants as defined in the storage.FS interface
func (nc *StorageDriver) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("ListGrants %s", bodyStr)

	_, respBody, err := nc.do(ctx, Action{"ListGrants", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	var m []map[string]bool
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return nil, err
	}
	grants := make([]*provider.Grant, len(m))
	for i := 0; i < len(m); i++ {
		var perms = &provider.ResourcePermissions{
			AddGrant:             false,
			CreateContainer:      false,
			Delete:               false,
			GetPath:              false,
			GetQuota:             false,
			InitiateFileDownload: false,
			InitiateFileUpload:   false,
			ListGrants:           false,
			ListContainer:        false,
			ListFileVersions:     false,
			ListRecycle:          false,
			Move:                 false,
			RemoveGrant:          false,
			PurgeRecycle:         false,
			RestoreFileVersion:   false,
			RestoreRecycleItem:   false,
			Stat:                 false,
			UpdateGrant:          false,
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []byte{},
			XXX_sizecache:        0,
		}
		for key, element := range m[i] {
			if key == "stat" {
				perms.Stat = element
			}
			if key == "move" {
				perms.Move = element
			}
			if key == "delete" {
				perms.Delete = element
			}
		}
		grants[i] = &provider.Grant{
			Grantee: &provider.Grantee{
				Type:                 provider.GranteeType_GRANTEE_TYPE_USER,
				Id:                   nil,
				Opaque:               &types.Opaque{},
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     []byte{},
				XXX_sizecache:        0,
			},
			Permissions:          perms,
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []byte{},
			XXX_sizecache:        0,
		}
	}
	return grants, err
}

// GetQuota as defined in the storage.FS interface
func (nc *StorageDriver) GetQuota(ctx context.Context) (uint64, uint64, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("GetQuota")

	_, _, err := nc.do(ctx, Action{"GetQuota", ""})
	return 0, 0, err
}

// CreateReference as defined in the storage.FS interface
func (nc *StorageDriver) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("CreateReference %s", path)

	_, _, err := nc.do(ctx, Action{"CreateReference", fmt.Sprintf(`{"path":"%s"}`, path)})
	return err
}

// Shutdown as defined in the storage.FS interface
func (nc *StorageDriver) Shutdown(ctx context.Context) error {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("Shutdown")

	_, _, err := nc.do(ctx, Action{"Shutdown", ""})
	return err
}

// SetArbitraryMetadata as defined in the storage.FS interface
func (nc *StorageDriver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	bodyStr, _ := json.Marshal(md)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("SetArbitraryMetadata %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"SetArbitraryMetadata", string(bodyStr)})
	return err
}

// UnsetArbitraryMetadata as defined in the storage.FS interface
func (nc *StorageDriver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("UnsetArbitraryMetadata %s", bodyStr)

	_, _, err := nc.do(ctx, Action{"UnsetArbitraryMetadata", string(bodyStr)})
	return err
}

// ListStorageSpaces :as defined in the storage.FS interface
func (nc *StorageDriver) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("ListStorageSpaces")

	_, _, err := nc.do(ctx, Action{"ListStorageSpaces", ""})
	return nil, err
}
