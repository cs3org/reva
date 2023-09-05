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
	"strconv"
	"strings"

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

type StatFromPhp struct {
	Opaque struct {
		Map any `json:"map"`
	} `json:"opaque"`
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
	Token string `json:"token"`
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

// Action describes a REST request to forward to the Nextcloud backend.
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

// SetHTTPClient sets the HTTP client.
func (nc *StorageDriver) SetHTTPClient(c *http.Client) {
	nc.client = c
}

func (nc *StorageDriver) doUpload(ctx context.Context, filePath string, r io.ReadCloser) error {
	log := appctx.GetLogger(ctx)
	user, err := getUser(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error getting user")
		return err
	}

	// See https://github.com/pondersource/nc-sciencemesh/issues/5
	// url := nc.endPoint + "~" + user.Username + "/files/" + filePath
	url := nc.endPoint + "~" + user.Id.OpaqueId + "/api/storage/Upload/home" + filePath
	req, err := http.NewRequest(http.MethodPut, url, r)
	if err != nil {
		log.Error().Err(err).Msg("error creating PUT request")
		return err
	}

	req.Header.Set("X-Reva-Secret", nc.sharedSecret)
	req.Header.Set("Content-Type", "application/octet-stream")
	log.Debug().Msgf("sending PUT to NC/OC at %s", url)
	resp, err := nc.client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("error sending PUT request")
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		log.Error().Int("status", resp.StatusCode).Msg("NC/OC response is not ok")
		return err
	}

	defer resp.Body.Close()
	return nil
}

func (nc *StorageDriver) doDownload(ctx context.Context, filePath string) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)
	user, err := getUser(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error getting user")
		return nil, err
	}
	// See https://github.com/pondersource/nc-sciencemesh/issues/5
	// url := nc.endPoint + "~" + user.Username + "/files/" + filePath
	url := nc.endPoint + "~" + user.Username + "/api/storage/Download/" + filePath
	req, err := http.NewRequest(http.MethodGet, url, strings.NewReader(""))
	if err != nil {
		log.Error().Err(err).Msg("error creating GET request")
		return nil, err
	}

	// See https://github.com/cs3org/reva/issues/4118
	req.Header.Set("X-Reva-Secret", nc.sharedSecret)
	log.Debug().Msgf("sending GET to NC/OC at %s", url)
	resp, err := nc.client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("error sending GET request")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Msg("NC/OC response is not ok")
		return nil, err
	}

	return resp.Body, nil
}

func (nc *StorageDriver) doDownloadRevision(ctx context.Context, filePath string, key string) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)
	user, err := getUser(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error getting user")
		return nil, err
	}
	// See https://github.com/pondersource/nc-sciencemesh/issues/5
	url := nc.endPoint + "~" + user.Username + "/api/storage/DownloadRevision/" + url.QueryEscape(key) + "/" + filePath
	req, err := http.NewRequest(http.MethodGet, url, strings.NewReader(""))
	if err != nil {
		log.Error().Err(err).Msg("error creating GET request")
		return nil, err
	}

	req.Header.Set("X-Reva-Secret", nc.sharedSecret)
	log.Debug().Msgf("sending GET to NC/OC at %s", url)
	resp, err := nc.client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("error sending GET request")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Msg("NC/OC response is not ok")
		return nil, err
	}

	return resp.Body, nil
}

func (nc *StorageDriver) do(ctx context.Context, a Action) (int, []byte, error) {
	log := appctx.GetLogger(ctx)
	user, err := getUser(ctx)
	if err != nil {
		return 0, nil, err
	}
	// See https://github.com/cs3org/reva/issues/2377
	// for discussion of user.Username vs user.Id.OpaqueId
	url := nc.endPoint + "~" + user.Id.OpaqueId + "/api/storage/" + a.verb
	log.Info().Msgf("nc.do req %s %s", url, a.argS)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Reva-Secret", nc.sharedSecret)

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
	log.Info().Msgf("nc.do res %s %s", url, string(body))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNotFound {
		return 0, nil, fmt.Errorf("Unexpected response code from EFSS API: " + strconv.Itoa(resp.StatusCode) + ":" + string(body))
	}
	return resp.StatusCode, body, nil
}

// GetHome as defined in the storage.FS interface.
func (nc *StorageDriver) GetHome(ctx context.Context) (string, error) {
	_, respBody, err := nc.do(ctx, Action{"GetHome", ""})
	return string(respBody), err
}

// CreateHome as defined in the storage.FS interface.
func (nc *StorageDriver) CreateHome(ctx context.Context) error {
	_, _, err := nc.do(ctx, Action{"CreateHome", ""})
	return err
}

// CreateDir as defined in the storage.FS interface.
func (nc *StorageDriver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}

	_, _, err = nc.do(ctx, Action{"CreateDir", string(bodyStr)})
	return err
}

// TouchFile as defined in the storage.FS interface.
func (nc *StorageDriver) TouchFile(ctx context.Context, ref *provider.Reference) error {
	return fmt.Errorf("unimplemented: TouchFile")
}

// Delete as defined in the storage.FS interface.
func (nc *StorageDriver) Delete(ctx context.Context, ref *provider.Reference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}

	_, _, err = nc.do(ctx, Action{"Delete", string(bodyStr)})
	return err
}

// Move as defined in the storage.FS interface.
func (nc *StorageDriver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	type paramsObj struct {
		OldRef *provider.Reference `json:"oldRef"`
		NewRef *provider.Reference `json:"newRef"`
	}
	bodyObj := &paramsObj{
		OldRef: oldRef,
		NewRef: newRef,
	}
	bodyStr, _ := json.Marshal(bodyObj)

	_, _, err := nc.do(ctx, Action{"Move", string(bodyStr)})
	return err
}

func resInfosFromPhpNode(resp []byte) ([]*provider.ResourceInfo, error) {
	// Parse the JSON struct returned by the PHP SM app into an array of ResourceInfo,
	// translating the permissions from ownCloud DB value to CS3 and ignoring non relevant fields.
	var respArray []StatFromPhp
	err := json.Unmarshal(resp, &respArray)
	if err != nil {
		return nil, err
	}

	var resInfo = make([]*provider.ResourceInfo, len(respArray))
	for i := 0; i < len(respArray); i++ {
		respObj := respArray[i]
		resInfo[i] = &provider.ResourceInfo{
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
			Path:          respObj.Path,
			PermissionSet: conversions.RoleFromOCSPermissions(conversions.Permissions(respObj.Permissions)).CS3ResourcePermissions(),
			Size:          uint64(respObj.Size),
			Owner: &user.UserId{
				Idp:      respObj.Owner.Idp,
				OpaqueId: respObj.Owner.OpaqueID,
				Type:     user.UserType_USER_TYPE_PRIMARY,
			},
		}
	}
	return resInfo, nil
}

// GetMD as defined in the storage.FS interface.
func (nc *StorageDriver) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	type paramsObj struct {
		Ref    *provider.Reference `json:"ref"`
		MdKeys []string            `json:"mdKeys"`
		// MetaData provider.ResourceInfo `json:"metaData"`
	}
	bodyObj := &paramsObj{
		Ref:    ref,
		MdKeys: mdKeys,
	}
	log := appctx.GetLogger(ctx)
	bodyStr, _ := json.Marshal(bodyObj)

	status, body, err := nc.do(ctx, Action{"GetMD", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, errtypes.NotFound("")
	}

	// use the array parsing for the single metadata payload received here
	retValue, err := resInfosFromPhpNode(bytes.Join([][]byte{[]byte("["), body, []byte("]")}, []byte{}))
	if err != nil {
		log.Error().Err(err).Str("output", string(body)).Msg("Failed to parse output")
		return nil, err
	}
	return retValue[0], nil
}

// ListFolder as defined in the storage.FS interface.
func (nc *StorageDriver) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	type paramsObj struct {
		Ref    *provider.Reference `json:"ref"`
		MdKeys []string            `json:"mdKeys"`
	}
	bodyObj := &paramsObj{
		Ref:    ref,
		MdKeys: mdKeys,
	}
	log := appctx.GetLogger(ctx)
	bodyStr, err := json.Marshal(bodyObj)
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

	retValue, err := resInfosFromPhpNode(body)
	if err != nil {
		log.Error().Err(err).Str("output", string(body)).Msg("Failed to parse output")
		return nil, err
	}
	return retValue, nil
}

// InitiateUpload as defined in the storage.FS interface.
func (nc *StorageDriver) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	type paramsObj struct {
		Ref          *provider.Reference `json:"ref"`
		UploadLength int64               `json:"uploadLength"`
		Metadata     map[string]string   `json:"metadata"`
	}
	bodyObj := &paramsObj{
		Ref:          ref,
		UploadLength: uploadLength,
		Metadata:     metadata,
	}
	bodyStr, _ := json.Marshal(bodyObj)
	log := appctx.GetLogger(ctx)

	_, respBody, err := nc.do(ctx, Action{"InitiateUpload", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	respMap := make(map[string]string)
	err = json.Unmarshal(respBody, &respMap)
	if err != nil {
		log.Error().Err(err).Str("output", string(respBody)).Msg("Failed to parse output")
		return nil, err
	}
	return respMap, nil
}

// Upload as defined in the storage.FS interface.
func (nc *StorageDriver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	return nc.doUpload(ctx, ref.Path, r)
}

// Download as defined in the storage.FS interface.
func (nc *StorageDriver) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	return nc.doDownload(ctx, ref.Path)
}

// ListRevisions as defined in the storage.FS interface.
func (nc *StorageDriver) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	bodyStr, _ := json.Marshal(ref)
	_, respBody, err := nc.do(ctx, Action{"ListRevisions", string(bodyStr)})

	if err != nil {
		return nil, err
	}
	var respMapArr []provider.FileVersion
	err = json.Unmarshal(respBody, &respMapArr)
	if err != nil {
		return nil, err
	}
	revs := make([]*provider.FileVersion, len(respMapArr))
	for i := 0; i < len(respMapArr); i++ {
		revs[i] = &respMapArr[i]
	}
	return revs, err
}

// DownloadRevision as defined in the storage.FS interface.
func (nc *StorageDriver) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	return nc.doDownloadRevision(ctx, ref.Path, key)
}

// RestoreRevision as defined in the storage.FS interface.
func (nc *StorageDriver) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	type paramsObj struct {
		Ref *provider.Reference `json:"ref"`
		Key string              `json:"key"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		Key: key,
	}
	bodyStr, _ := json.Marshal(bodyObj)

	_, _, err := nc.do(ctx, Action{"RestoreRevision", string(bodyStr)})
	return err
}

// ListRecycle as defined in the storage.FS interface.
func (nc *StorageDriver) ListRecycle(ctx context.Context, basePath, key string, relativePath string) ([]*provider.RecycleItem, error) {
	type paramsObj struct {
		Key  string `json:"key"`
		Path string `json:"path"`
	}
	bodyObj := &paramsObj{
		Key:  key,
		Path: relativePath,
	}
	bodyStr, _ := json.Marshal(bodyObj)

	_, respBody, err := nc.do(ctx, Action{"ListRecycle", string(bodyStr)})

	if err != nil {
		return nil, err
	}
	var respMapArr []provider.RecycleItem
	err = json.Unmarshal(respBody, &respMapArr)
	if err != nil {
		return nil, err
	}
	items := make([]*provider.RecycleItem, len(respMapArr))
	for i := 0; i < len(respMapArr); i++ {
		items[i] = &respMapArr[i]
	}
	return items, nil
}

// RestoreRecycleItem as defined in the storage.FS interface.
func (nc *StorageDriver) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	type paramsObj struct {
		Key        string              `json:"key"`
		Path       string              `json:"path"`
		RestoreRef *provider.Reference `json:"restoreRef"`
	}
	bodyObj := &paramsObj{
		Key:        key,
		Path:       relativePath,
		RestoreRef: restoreRef,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"RestoreRecycleItem", string(bodyStr)})

	return err
}

// PurgeRecycleItem as defined in the storage.FS interface.
func (nc *StorageDriver) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	type paramsObj struct {
		Key  string `json:"key"`
		Path string `json:"path"`
	}
	bodyObj := &paramsObj{
		Key:  key,
		Path: relativePath,
	}
	bodyStr, _ := json.Marshal(bodyObj)

	_, _, err := nc.do(ctx, Action{"PurgeRecycleItem", string(bodyStr)})
	return err
}

// EmptyRecycle as defined in the storage.FS interface.
func (nc *StorageDriver) EmptyRecycle(ctx context.Context) error {
	_, _, err := nc.do(ctx, Action{"EmptyRecycle", ""})
	return err
}

// GetPathByID as defined in the storage.FS interface.
func (nc *StorageDriver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	bodyStr, _ := json.Marshal(id)
	_, respBody, err := nc.do(ctx, Action{"GetPathByID", string(bodyStr)})
	return string(respBody), err
}

// AddGrant as defined in the storage.FS interface.
func (nc *StorageDriver) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	type paramsObj struct {
		Ref *provider.Reference `json:"ref"`
		G   *provider.Grant     `json:"g"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		G:   g,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"AddGrant", string(bodyStr)})
	return err
}

// DenyGrant as defined in the storage.FS interface.
func (nc *StorageDriver) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	type paramsObj struct {
		Ref *provider.Reference `json:"ref"`
		G   *provider.Grantee   `json:"g"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		G:   g,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"DenyGrant", string(bodyStr)})
	return err
}

// RemoveGrant as defined in the storage.FS interface.
func (nc *StorageDriver) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	type paramsObj struct {
		Ref *provider.Reference `json:"ref"`
		G   *provider.Grant     `json:"g"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		G:   g,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"RemoveGrant", string(bodyStr)})
	return err
}

// UpdateGrant as defined in the storage.FS interface.
func (nc *StorageDriver) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	type paramsObj struct {
		Ref *provider.Reference `json:"ref"`
		G   *provider.Grant     `json:"g"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		G:   g,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"UpdateGrant", string(bodyStr)})
	return err
}

// ListGrants as defined in the storage.FS interface.
func (nc *StorageDriver) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	bodyStr, _ := json.Marshal(ref)
	log := appctx.GetLogger(ctx)

	_, respBody, err := nc.do(ctx, Action{"ListGrants", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	// To avoid this error:
	// json: cannot unmarshal object into Go struct field Grantee.grantee.Id of type providerv1beta1.isGrantee_Id
	// To test:
	// bodyStr, _ := json.Marshal(provider.Grant{
	// 	 Grantee: &provider.Grantee{
	// 		 Type: provider.GranteeType_GRANTEE_TYPE_USER,
	// 		 Id: &provider.Grantee_UserId{
	// 			 UserId: &user.UserId{
	// 				 Idp:      "some-idp",
	// 				 OpaqueId: "some-opaque-id",
	// 				 Type:     user.UserType_USER_TYPE_PRIMARY,
	// 			 },
	// 		 },
	// 	 },
	// 	 Permissions: &provider.ResourcePermissions{},
	// })
	// JSON example:
	// [{"grantee":{"Id":{"UserId":{"idp":"some-idp","opaque_id":"some-opaque-id","type":1}}},"permissions":{"add_grant":true,"create_container":true,"delete":true,"get_path":true,"get_quota":true,"initiate_file_download":true,"initiate_file_upload":true,"list_grants":true}}]
	var respMapArr []map[string]interface{}
	err = json.Unmarshal(respBody, &respMapArr)
	if err != nil {
		log.Error().Err(err).Str("output", string(respBody)).Msg("Failed to parse output")
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
func (nc *StorageDriver) GetQuota(ctx context.Context, ref *provider.Reference) (uint64, uint64, error) {
	_, respBody, err := nc.do(ctx, Action{"GetQuota", ""})
	if err != nil {
		return 0, 0, err
	}

	var respMap map[string]interface{}
	err = json.Unmarshal(respBody, &respMap)
	if err != nil {
		return 0, 0, err
	}
	return uint64(respMap["totalBytes"].(float64)), uint64(respMap["usedBytes"].(float64)), nil
}

// CreateReference as defined in the storage.FS interface.
func (nc *StorageDriver) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	type paramsObj struct {
		Path string `json:"path"`
		URL  string `json:"url"`
	}
	bodyObj := &paramsObj{
		Path: path,
		URL:  targetURI.String(),
	}
	bodyStr, _ := json.Marshal(bodyObj)

	_, _, err := nc.do(ctx, Action{"CreateReference", string(bodyStr)})
	return err
}

// Shutdown as defined in the storage.FS interface.
func (nc *StorageDriver) Shutdown(ctx context.Context) error {
	_, _, err := nc.do(ctx, Action{"Shutdown", ""})
	return err
}

// SetArbitraryMetadata as defined in the storage.FS interface.
func (nc *StorageDriver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	type paramsObj struct {
		Ref *provider.Reference         `json:"ref"`
		Md  *provider.ArbitraryMetadata `json:"md"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		Md:  md,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"SetArbitraryMetadata", string(bodyStr)})
	return err
}

// UnsetArbitraryMetadata as defined in the storage.FS interface.
func (nc *StorageDriver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	type paramsObj struct {
		Ref  *provider.Reference `json:"ref"`
		Keys []string            `json:"keys"`
	}
	bodyObj := &paramsObj{
		Ref:  ref,
		Keys: keys,
	}

	bodyStr, _ := json.Marshal(bodyObj)
	_, _, err := nc.do(ctx, Action{"UnsetArbitraryMetadata", string(bodyStr)})
	return err
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
	log := appctx.GetLogger(ctx)
	bodyStr, _ := json.Marshal(f)
	_, respBody, err := nc.do(ctx, Action{"ListStorageSpaces", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	// https://github.com/cs3org/go-cs3apis/blob/970eec3/cs3/storage/provider/v1beta1/resources.pb.go#L1341-L1366
	var respMapArr []provider.StorageSpace
	err = json.Unmarshal(respBody, &respMapArr)
	if err != nil {
		log.Error().Err(err).Str("output", string(respBody)).Msg("Failed to parse output")
		return nil, err
	}
	var spaces = make([]*provider.StorageSpace, len(respMapArr))
	for i := 0; i < len(respMapArr); i++ {
		spaces[i] = &respMapArr[i]
	}
	return spaces, err
}

// CreateStorageSpace creates a storage space.
func (nc *StorageDriver) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	bodyStr, _ := json.Marshal(req)
	_, respBody, err := nc.do(ctx, Action{"CreateStorageSpace", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	var respObj provider.CreateStorageSpaceResponse
	err = json.Unmarshal(respBody, &respObj)
	if err != nil {
		return nil, err
	}
	return &respObj, nil
}

// UpdateStorageSpace updates a storage space.
func (nc *StorageDriver) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	bodyStr, _ := json.Marshal(req)
	_, respBody, err := nc.do(ctx, Action{"UpdateStorageSpace", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	var respObj provider.UpdateStorageSpaceResponse
	err = json.Unmarshal(respBody, &respObj)
	if err != nil {
		return nil, err
	}
	return &respObj, nil
}
