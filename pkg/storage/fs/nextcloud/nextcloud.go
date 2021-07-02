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
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/tests/helpers"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("nextcloud", New)
}

// NextcloudStorageDriverConfig
// Configuration for a NextcloudStorageDriver
//

type NextcloudStorageDriverConfig struct {
	EndPoint string `mapstructure:"end_point"` // e.g. "http://nc/apps/sciencemesh/~alice/"
	MockHTTP bool   `mapstructure:"mock_http"`
}

// NextcloudStorageDriver
// Implementation of storage.FS that connects with a NextcloudStorageDriver server as its backend
type NextcloudStorageDriver struct {
	endPoint string
	client   *http.Client
}

func parseConfig(m map[string]interface{}) (*NextcloudStorageDriverConfig, error) {
	c := &NextcloudStorageDriverConfig{}
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

	return NewNextcloudStorageDriver(conf)
}

// NewNextcloudStorageDriver returns a new NextcloudStorageDriver
func NewNextcloudStorageDriver(c *NextcloudStorageDriverConfig) (*NextcloudStorageDriver, error) {
	var client *http.Client
	if c.MockHTTP {
		nextcloudServerMock := GetNextcloudServerMock()
		client, _ = helpers.TestingHTTPClient(nextcloudServerMock)
	} else {
		client = &http.Client{}
	}
	return &NextcloudStorageDriver{
		endPoint: c.EndPoint, // e.g. "http://nc/apps/sciencemesh/~alice/"
		client:   client,
	}, nil
}

// Action describes a REST request to forward to the Nextcloud backend
type Action struct {
	verb string
	argS string
}

// SetHTTPClient sets the HTTP client
func (nc *NextcloudStorageDriver) SetHTTPClient(c *http.Client) {
	nc.client = c
}

func (nc *NextcloudStorageDriver) doUpload(r io.ReadCloser) error {
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

func (nc *NextcloudStorageDriver) do(a Action, endPoint string) (int, []byte, error) {
	url := endPoint + a.verb
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := nc.client.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

// GetHome as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) GetHome(ctx context.Context) (string, error) {
	_, respBody, err := nc.do(Action{"GetHome", ""}, nc.endPoint)
	return string(respBody), err
}

// CreateHome as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) CreateHome(ctx context.Context) error {
	_, _, err := nc.do(Action{"CreateHome", ""}, nc.endPoint)
	return err
}

// CreateDir as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) CreateDir(ctx context.Context, fn string) error {
	data := make(map[string]string)
	data["path"] = fn
	bodyStr, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, _, err = nc.do(Action{"CreateDir", string(bodyStr)}, nc.endPoint)
	return err
}

// Delete as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) Delete(ctx context.Context, ref *provider.Reference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	_, _, err = nc.do(Action{"Delete", string(bodyStr)}, nc.endPoint)
	return err
}

// Move as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	data := make(map[string]string)
	data["from"] = oldRef.Path
	data["to"] = newRef.Path
	bodyStr, _ := json.Marshal(data)
	_, _, err := nc.do(Action{"Move", string(bodyStr)}, nc.endPoint)
	return err
}

// GetMD as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}
	status, body, err := nc.do(Action{"GetMD", string(bodyStr)}, nc.endPoint)
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
func (nc *NextcloudStorageDriver) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}
	status, body, err := nc.do(Action{"ListFolder", string(bodyStr)}, nc.endPoint)
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
func (nc *NextcloudStorageDriver) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	bodyStr, _ := json.Marshal(ref)
	_, respBody, err := nc.do(Action{"InitiateUpload", string(bodyStr)}, nc.endPoint)
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
func (nc *NextcloudStorageDriver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	bodyStr, _ := json.Marshal(ref)
	err := nc.doUpload(r)
	if err != nil {
		return err
	}
	_, _, err = nc.do(Action{"Upload", string(bodyStr)}, nc.endPoint)
	return err
}

// Download as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"Download", string(bodyStr)}, nc.endPoint)
	return nil, err
}

// ListRevisions as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	bodyStr, _ := json.Marshal(ref)
	_, respBody, err := nc.do(Action{"ListRevisions", string(bodyStr)}, nc.endPoint)
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
func (nc *NextcloudStorageDriver) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"DownloadRevision", string(bodyStr)}, nc.endPoint)
	return nil, err
}

// RestoreRevision as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"RestoreRevision", string(bodyStr)}, nc.endPoint)
	return err
}

// ListRecycle as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	_, respBody, err := nc.do(Action{"ListRecycle", ""}, nc.endPoint)
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
func (nc *NextcloudStorageDriver) RestoreRecycleItem(ctx context.Context, key string, restoreRef *provider.Reference) error {
	bodyStr, _ := json.Marshal(restoreRef)
	_, _, err := nc.do(Action{"RestoreRecycleItem", string(bodyStr)}, nc.endPoint)
	return err
}

// PurgeRecycleItem as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) PurgeRecycleItem(ctx context.Context, key string) error {
	bodyStr, _ := json.Marshal(key)
	_, _, err := nc.do(Action{"PurgeRecycleItem", string(bodyStr)}, nc.endPoint)
	return err
}

// EmptyRecycle as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) EmptyRecycle(ctx context.Context) error {
	_, _, err := nc.do(Action{"EmptyRecycle", ""}, nc.endPoint)
	return err
}

// GetPathByID as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	bodyStr, _ := json.Marshal(id)
	_, respBody, err := nc.do(Action{"GetPathByID", string(bodyStr)}, nc.endPoint)
	return string(respBody), err
}

// AddGrant as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"AddGrant", string(bodyStr)}, nc.endPoint)
	return err
}

// RemoveGrant as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"RemoveGrant", string(bodyStr)}, nc.endPoint)
	return err
}

// UpdateGrant as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"UpdateGrant", string(bodyStr)}, nc.endPoint)
	return err
}

// ListGrants as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	bodyStr, _ := json.Marshal(ref)
	_, respBody, err := nc.do(Action{"ListGrants", string(bodyStr)}, nc.endPoint)
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
func (nc *NextcloudStorageDriver) GetQuota(ctx context.Context) (uint64, uint64, error) {
	_, _, err := nc.do(Action{"GetQuota", ""}, nc.endPoint)
	return 0, 0, err
}

// CreateReference as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	_, _, err := nc.do(Action{"CreateReference", fmt.Sprintf(`{"path":"%s"}`, path)}, nc.endPoint)
	return err
}

// Shutdown as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) Shutdown(ctx context.Context) error {
	_, _, err := nc.do(Action{"Shutdown", ""}, nc.endPoint)
	return err
}

// SetArbitraryMetadata as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	bodyStr, _ := json.Marshal(md)
	_, _, err := nc.do(Action{"SetArbitraryMetadata", string(bodyStr)}, nc.endPoint)
	return err
}

// UnsetArbitrarymetadata as defined in the storage.FS interface
func (nc *NextcloudStorageDriver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	bodyStr, _ := json.Marshal(ref)
	_, _, err := nc.do(Action{"UnsetArbitraryMetadata", string(bodyStr)}, nc.endPoint)
	return err
}
