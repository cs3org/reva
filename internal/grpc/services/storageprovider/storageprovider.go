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

package storageprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("storageprovider", New)
}

type config struct {
	MountPath           string                            `mapstructure:"mount_path" docs:"/;The path where the file system would be mounted."`
	MountID             string                            `mapstructure:"mount_id" docs:"-;The ID of the mounted file system."`
	Driver              string                            `mapstructure:"driver" docs:"localhome;The storage driver to be used."`
	Drivers             map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:pkg/storage/fs/localhome/localhome.go"`
	TmpFolder           string                            `mapstructure:"tmp_folder" docs:"/var/tmp;Path to temporary folder."`
	DataServerURL       string                            `mapstructure:"data_server_url" docs:"http://localhost/data;The URL for the data server."`
	ExposeDataServer    bool                              `mapstructure:"expose_data_server" docs:"false;Whether to expose data server."` // if true the client will be able to upload/download directly to it
	AvailableXS         map[string]uint32                 `mapstructure:"available_checksums" docs:"nil;List of available checksums."`
	CustomMimeTypesJSON string                            `mapstructure:"custom_mime_types_json" docs:"nil;An optional mapping file with the list of supported custom file extensions and corresponding mime types."`
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "localhome"
	}

	if c.MountPath == "" {
		c.MountPath = "/"
	}

	if c.MountID == "" {
		c.MountID = "00000000-0000-0000-0000-000000000000"
	}

	if c.TmpFolder == "" {
		c.TmpFolder = "/var/tmp/reva/tmp"
	}

	if c.DataServerURL == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			c.DataServerURL = "http://0.0.0.0:19001/data"
		} else {
			c.DataServerURL = fmt.Sprintf("http://%s:19001/data", host)
		}
	}

	// set sane defaults
	if len(c.AvailableXS) == 0 {
		c.AvailableXS = map[string]uint32{"md5": 100, "unset": 1000}
	}
}

type service struct {
	conf               *config
	storage            storage.FS
	mountPath, mountID string
	tmpFolder          string
	dataServerURL      *url.URL
	availableXS        []*provider.ResourceChecksumPriority
}

func (s *service) Close() error {
	return s.storage.Shutdown(context.Background())
}

func (s *service) UnprotectedEndpoints() []string { return []string{} }

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterProviderAPIServer(ss, s)
}

func parseXSTypes(xsTypes map[string]uint32) ([]*provider.ResourceChecksumPriority, error) {
	var types = make([]*provider.ResourceChecksumPriority, 0, len(xsTypes))
	for xs, prio := range xsTypes {
		t := PKG2GRPCXS(xs)
		if t == provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID {
			return nil, errtypes.BadRequest("checksum type is invalid: " + xs)
		}
		xsPrio := &provider.ResourceChecksumPriority{
			Priority: prio,
			Type:     t,
		}
		types = append(types, xsPrio)
	}
	return types, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func registerMimeTypes(mappingFile string) error {
	if mappingFile != "" {
		f, err := os.ReadFile(mappingFile)
		if err != nil {
			return fmt.Errorf("storageprovider: error reading the custom mime types file: +%v", err)
		}
		mimeTypes := map[string]string{}
		err = json.Unmarshal(f, &mimeTypes)
		if err != nil {
			return fmt.Errorf("storageprovider: error unmarshalling the custom mime types file: +%v", err)
		}
		// register all mime types that were read
		for e, m := range mimeTypes {
			mime.RegisterMime(e, m)
		}
	}
	return nil
}

// New creates a new storage provider svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	c.init()

	if err := os.MkdirAll(c.TmpFolder, 0755); err != nil {
		return nil, err
	}

	mountPath := c.MountPath
	mountID := c.MountID

	fs, err := getFS(c)
	if err != nil {
		return nil, err
	}

	// parse data server url
	u, err := url.Parse(c.DataServerURL)
	if err != nil {
		return nil, err
	}

	// validate available checksums
	xsTypes, err := parseXSTypes(c.AvailableXS)
	if err != nil {
		return nil, err
	}

	if len(xsTypes) == 0 {
		return nil, errtypes.NotFound("no available checksum, please set in config")
	}

	// read and register custom mime types if configured
	err = registerMimeTypes(c.CustomMimeTypesJSON)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:          c,
		storage:       fs,
		tmpFolder:     c.TmpFolder,
		mountPath:     mountPath,
		mountID:       mountID,
		dataServerURL: u,
		availableXS:   xsTypes,
	}

	return service, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &provider.SetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error setting arbitrary metadata"),
		}, nil
	}

	if err := s.storage.SetArbitraryMetadata(ctx, newRef, req.ArbitraryMetadata); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting arbitrary metadata")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error setting arbitrary metadata: "+req.Ref.String())
		}
		return &provider.SetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}

	res := &provider.SetArbitraryMetadataResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &provider.UnsetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error unsetting arbitrary metadata"),
		}, nil
	}

	if err := s.storage.UnsetArbitraryMetadata(ctx, newRef, req.ArbitraryMetadataKeys); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when unsetting arbitrary metadata")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error unsetting arbitrary metadata: "+req.Ref.String())
		}
		return &provider.UnsetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}

	res := &provider.UnsetArbitraryMetadataResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

// SetLock puts a lock on the given reference
func (s *service) SetLock(ctx context.Context, req *provider.SetLockRequest) (*provider.SetLockResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &provider.SetLockResponse{
			Status: status.NewInternal(ctx, err, "error setting lock"),
		}, nil
	}

	if err := s.storage.SetLock(ctx, newRef, req.Lock); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting lock")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.BadRequest:
			st = status.NewFailedPrecondition(ctx, err, "reference already locked")
		default:
			st = status.NewInternal(ctx, err, "error setting lock: "+req.Ref.String())
		}
		return &provider.SetLockResponse{
			Status: st,
		}, nil
	}

	res := &provider.SetLockResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

// GetLock returns an existing lock on the given reference
func (s *service) GetLock(ctx context.Context, req *provider.GetLockRequest) (*provider.GetLockResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &provider.GetLockResponse{
			Status: status.NewInternal(ctx, err, "error getting lock"),
		}, nil
	}

	var lock *provider.Lock
	if lock, err = s.storage.GetLock(ctx, newRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "reference or lock not found")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error getting lock: "+req.Ref.String())
		}
		return &provider.GetLockResponse{
			Status: st,
		}, nil
	}

	res := &provider.GetLockResponse{
		Status: status.NewOK(ctx),
		Lock:   lock,
	}
	return res, nil
}

// RefreshLock refreshes an existing lock on the given reference
func (s *service) RefreshLock(ctx context.Context, req *provider.RefreshLockRequest) (*provider.RefreshLockResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &provider.RefreshLockResponse{
			Status: status.NewInternal(ctx, err, "error refreshing lock"),
		}, nil
	}

	if err = s.storage.RefreshLock(ctx, newRef, req.Lock, req.ExistingLockId); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when refreshing lock")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.BadRequest:
			st = status.NewFailedPrecondition(ctx, err, "reference not locked or caller does not hold the lock")
		default:
			st = status.NewInternal(ctx, err, "error refreshing lock: "+req.Ref.String())
		}
		return &provider.RefreshLockResponse{
			Status: st,
		}, nil
	}

	res := &provider.RefreshLockResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

// Unlock removes an existing lock from the given reference
func (s *service) Unlock(ctx context.Context, req *provider.UnlockRequest) (*provider.UnlockResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &provider.UnlockResponse{
			Status: status.NewInternal(ctx, err, "error on unlocking"),
		}, nil
	}

	if err = s.storage.Unlock(ctx, newRef, req.Lock); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when unlocking")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.BadRequest:
			st = status.NewFailedPrecondition(ctx, err, "reference not locked")
		default:
			st = status.NewInternal(ctx, err, "error unlocking: "+req.Ref.String())
		}
		return &provider.UnlockResponse{
			Status: st,
		}, nil
	}

	res := &provider.UnlockResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	// TODO(labkode): maybe add some checks before download starts? eg. check permissions?
	// TODO(labkode): maybe add short-lived token?
	// We now simply point the client to the data server.
	// For example, https://data-server.example.org/home/docs/myfile.txt
	// or ownclouds://data-server.example.org/home/docs/myfile.txt
	log := appctx.GetLogger(ctx)
	u := *s.dataServerURL
	log.Info().Str("data-server", u.String()).Interface("ref", req.Ref).Msg("file download")

	protocol := &provider.FileDownloadProtocol{Expose: s.conf.ExposeDataServer}

	if utils.IsRelativeReference(req.Ref) {
		protocol.Protocol = "spaces"
		u.Path = path.Join(u.Path, "spaces", req.Ref.ResourceId.StorageId+"!"+req.Ref.ResourceId.OpaqueId, req.Ref.Path)
	} else {
		newRef, err := s.unwrap(ctx, req.Ref)
		if err != nil {
			return &provider.InitiateFileDownloadResponse{
				Status: status.NewInternal(ctx, err, "error unwrapping path"),
			}, nil
		}
		// Currently, we only support the simple protocol for GET requests
		// Once we have multiple protocols, this would be moved to the fs layer
		protocol.Protocol = "simple"
		u.Path = path.Join(u.Path, "simple", newRef.GetPath())
	}

	protocol.DownloadEndpoint = u.String()

	return &provider.InitiateFileDownloadResponse{
		Protocols: []*provider.FileDownloadProtocol{protocol},
		Status:    status.NewOK(ctx),
	}, nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	// TODO(labkode): same considerations as download
	log := appctx.GetLogger(ctx)
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	if newRef.GetPath() == "/" {
		return &provider.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, errtypes.BadRequest("can't upload to mount path"), "can't upload to mount path"),
		}, nil
	}

	metadata := map[string]string{}
	var uploadLength int64
	if req.Opaque != nil && req.Opaque.Map != nil {
		if req.Opaque.Map["Upload-Length"] != nil {
			var err error
			uploadLength, err = strconv.ParseInt(string(req.Opaque.Map["Upload-Length"].Value), 10, 64)
			if err != nil {
				return &provider.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, err, "error parsing upload length"),
				}, nil
			}
		}
		// TUS forward Upload-Checksum header as checksum, uses '[type] [hash]' format
		if req.Opaque.Map["Upload-Checksum"] != nil {
			metadata["checksum"] = string(req.Opaque.Map["Upload-Checksum"].Value)
		}
		// ownCloud mtime to set for the uploaded file
		if req.Opaque.Map["X-OC-Mtime"] != nil {
			metadata["mtime"] = string(req.Opaque.Map["X-OC-Mtime"].Value)
		}
	}
	uploadIDs, err := s.storage.InitiateUpload(ctx, newRef, uploadLength, metadata)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when initiating upload")
		case errtypes.IsBadRequest, errtypes.IsChecksumMismatch:
			st = status.NewInvalidArg(ctx, err.Error())
			// TODO TUS uses a custom ChecksumMismatch 460 http status which is in an unassigned range in
			// https://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml
			// maybe 409 conflict is good enough
			// someone is proposing `419 Checksum Error`, see https://stackoverflow.com/a/35665694
			// - it is also unassigned
			// - ends in 9 as the 409 conflict
			// - is near the 4xx errors about conditions: 415 Unsupported Media Type, 416 Range Not Satisfiable or 417 Expectation Failed
			// owncloud only expects a 400 Bad request so InvalidArg is good enough for now
			// seealso errtypes.StatusChecksumMismatch
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.InsufficientStorage:
			st = status.NewInsufficientStorage(ctx, err, "insufficient storage")
		default:
			st = status.NewInternal(ctx, err, "error getting upload id: "+req.Ref.String())
		}
		return &provider.InitiateFileUploadResponse{
			Status: st,
		}, nil
	}

	protocols := make([]*provider.FileUploadProtocol, len(uploadIDs))
	var i int
	for protocol, ID := range uploadIDs {
		u := *s.dataServerURL
		u.Path = path.Join(u.Path, protocol, ID)
		protocols[i] = &provider.FileUploadProtocol{
			Protocol:           protocol,
			UploadEndpoint:     u.String(),
			AvailableChecksums: s.availableXS,
			Expose:             s.conf.ExposeDataServer,
		}
		i++
		log.Info().Str("data-server", u.String()).
			Str("fn", req.Ref.GetPath()).
			Str("xs", fmt.Sprintf("%+v", s.conf.AvailableXS)).
			Msg("file upload")
	}

	res := &provider.InitiateFileUploadResponse{
		Protocols: protocols,
		Status:    status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	// TODO(labkode): check that the storage ID is the same as the storage provider id.
	fn, err := s.storage.GetPathByID(ctx, req.ResourceId)
	if err != nil {
		return &provider.GetPathResponse{
			Status: status.NewInternal(ctx, err, "error getting path by id"),
		}, nil
	}

	fn = path.Join(s.mountPath, path.Clean(fn))
	res := &provider.GetPathResponse{
		Path:   fn,
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	home := path.Join(s.mountPath)

	res := &provider.GetHomeResponse{
		Status: status.NewOK(ctx),
		Path:   home,
	}

	return res, nil
}

func (s *service) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	log := appctx.GetLogger(ctx)
	if err := s.storage.CreateHome(ctx); err != nil {
		st := status.NewInternal(ctx, err, "error creating home")
		log.Err(err).Msg("storageprovider: error calling CreateHome of storage driver")
		return &provider.CreateHomeResponse{
			Status: st,
		}, nil
	}

	res := &provider.CreateHomeResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

// CreateStorageSpace creates a storage space
func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	resp, err := s.storage.CreateStorageSpace(ctx, req)
	if err != nil {
		return nil, err
	}

	resp.StorageSpace.Root = &provider.ResourceId{StorageId: s.mountID, OpaqueId: resp.StorageSpace.Id.OpaqueId}
	resp.StorageSpace.Id = &provider.StorageSpaceId{OpaqueId: s.mountID + "!" + resp.StorageSpace.Id.OpaqueId}
	return resp, nil
}

func hasNodeID(s *provider.StorageSpace) bool {
	return s != nil && s.Root != nil && s.Root.OpaqueId != ""
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	log := appctx.GetLogger(ctx)

	spaces, err := s.storage.ListStorageSpaces(ctx, req.Filters)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "not found when listing spaces")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.NotSupported:
			st = status.NewUnimplemented(ctx, err, "not implemented")
		default:
			st = status.NewInternal(ctx, err, "error listing spaces")
		}
		return &provider.ListStorageSpacesResponse{
			Status: st,
		}, nil
	}

	for i := range spaces {
		if hasNodeID(spaces[i]) {
			// fill in storagespace id if it is not set
			if spaces[i].Id == nil || spaces[i].Id.OpaqueId == "" {
				spaces[i].Id = &provider.StorageSpaceId{OpaqueId: s.mountID + "!" + spaces[i].Root.OpaqueId}
			}
			// fill in storage id if it is not set
			if spaces[i].Root.StorageId == "" {
				spaces[i].Root.StorageId = s.mountID
			}
		} else if spaces[i].Id == nil || spaces[i].Id.OpaqueId == "" {
			log.Warn().Str("service", "storageprovider").Str("driver", s.conf.Driver).Interface("space", spaces[i]).Msg("space is missing space id and root id")
		}
	}

	return &provider.ListStorageSpacesResponse{
		Status:        status.NewOK(ctx),
		StorageSpaces: spaces,
	}, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return s.storage.UpdateStorageSpace(ctx, req)
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return &provider.DeleteStorageSpaceResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("DeleteStorageSpace not implemented"), "DeleteStorageSpace not implemented"),
	}, nil
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	if err := s.storage.CreateDir(ctx, newRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating container")
		case errtypes.AlreadyExists:
			st = status.NewAlreadyExists(ctx, err, "container already exists")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error creating container: "+req.Ref.String())
		}
		return &provider.CreateContainerResponse{
			Status: st,
		}, nil
	}

	res := &provider.CreateContainerResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) TouchFile(ctx context.Context, req *provider.TouchFileRequest) (*provider.TouchFileResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.TouchFileResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	if err := s.storage.TouchFile(ctx, newRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when touching the file")
		case errtypes.AlreadyExists:
			st = status.NewAlreadyExists(ctx, err, "file already exists")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error touching file: "+req.Ref.String())
		}
		return &provider.TouchFileResponse{
			Status: st,
		}, nil
	}

	res := &provider.TouchFileResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	if newRef.GetPath() == "/" {
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, errtypes.BadRequest("can't delete mount path"), "can't delete mount path"),
		}, nil
	}

	// check DeleteRequest for any known opaque properties.
	if req.Opaque != nil {
		_, ok := req.Opaque.Map["deleting_shared_resource"]
		if ok {
			// it is a binary key; its existence signals true. Although, do not assume.
			ctx = context.WithValue(ctx, appctx.DeletingSharedResource, true)
		}
	}

	if err := s.storage.Delete(ctx, newRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating container")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error deleting file: "+req.Ref.String())
		}
		return &provider.DeleteResponse{
			Status: st,
		}, nil
	}

	res := &provider.DeleteResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	sourceRef, err := s.unwrap(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping source path"),
		}, nil
	}
	targetRef, err := s.unwrap(ctx, req.Destination)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping destination path"),
		}, nil
	}

	if err := s.storage.Move(ctx, sourceRef, targetRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when moving")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error moving: "+sourceRef.String())
		}
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	res := &provider.MoveResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(ctx, "stat")
	defer span.End()

	span.SetAttributes(attribute.KeyValue{
		Key:   "reference",
		Value: attribute.StringValue(req.Ref.String()),
	})

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		// The path might be a virtual view; handle that case
		if utils.IsAbsolutePathReference(req.Ref) && strings.HasPrefix(s.mountPath, req.Ref.Path) {
			return s.statVirtualView(ctx, req.Ref)
		}
	}

	md, err := s.storage.GetMD(ctx, newRef, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when statting")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error statting: "+req.Ref.String())
		}
		return &provider.StatResponse{
			Status: st,
		}, nil
	}

	if err := s.wrap(ctx, md, utils.IsAbsoluteReference(req.Ref)); err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "error wrapping path"),
		}, nil
	}
	res := &provider.StatResponse{
		Status: status.NewOK(ctx),
		Info:   md,
	}
	return res, nil
}

func (s *service) statVirtualView(ctx context.Context, ref *provider.Reference) (*provider.StatResponse, error) {
	// The reference in the request encompasses this provider
	// So we need to stat root, and update the required path
	md, err := s.storage.GetMD(ctx, &provider.Reference{Path: "/"}, []string{})
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when statting")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error statting root")
		}
		return &provider.StatResponse{
			Status: st,
		}, nil
	}

	if err := s.wrap(ctx, md, true); err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "error wrapping path"),
		}, nil
	}

	// Don't expose the underlying path
	md.Path = ref.Path

	return &provider.StatResponse{
		Status: status.NewOK(ctx),
		Info:   md,
	}, nil
}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		res := &provider.ListContainerStreamResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListContainerStream: error sending response")
			return err
		}
		return nil
	}

	mds, err := s.storage.ListFolder(ctx, newRef, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing container")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing container: "+req.Ref.String())
		}
		res := &provider.ListContainerStreamResponse{
			Status: st,
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListContainerStream: error sending response")
			return err
		}
		return nil
	}

	prefixMountpoint := utils.IsAbsoluteReference(req.Ref)
	for _, md := range mds {
		if err := s.wrap(ctx, md, prefixMountpoint); err != nil {
			res := &provider.ListContainerStreamResponse{
				Status: status.NewInternal(ctx, err, "error wrapping path"),
			}
			if err := ss.Send(res); err != nil {
				log.Error().Err(err).Msg("ListContainerStream: error sending response")
				return err
			}
			return nil
		}
		res := &provider.ListContainerStreamResponse{
			Info:   md,
			Status: status.NewOK(ctx),
		}

		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListContainerStream: error sending response")
			return err
		}
	}
	return nil
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		// The path might be a virtual view; handle that case
		if utils.IsAbsolutePathReference(req.Ref) && strings.HasPrefix(s.mountPath, req.Ref.Path) {
			return s.listVirtualView(ctx, req.Ref)
		}

		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	mds, err := s.storage.ListFolder(ctx, newRef, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing container")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing container: "+req.Ref.String())
		}
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}

	var infos = make([]*provider.ResourceInfo, 0, len(mds))
	prefixMountpoint := utils.IsAbsoluteReference(req.Ref)
	for _, md := range mds {
		if err := s.wrap(ctx, md, prefixMountpoint); err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "error wrapping path"),
			}, nil
		}
		infos = append(infos, md)
	}
	res := &provider.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}
	return res, nil
}

func (s *service) listVirtualView(ctx context.Context, ref *provider.Reference) (*provider.ListContainerResponse, error) {
	// The reference in the request encompasses this provider
	// So we need to list root, merge the responses and return only the immediate children
	mds, err := s.storage.ListFolder(ctx, &provider.Reference{Path: "/"}, []string{})
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing root")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing root")
		}
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}

	nestedInfos := make(map[string]*provider.ResourceInfo)
	infos := make([]*provider.ResourceInfo, 0, len(mds))

	for _, info := range mds {
		// Get the path prefixed with the mount point
		if err := s.wrap(ctx, info, true); err != nil {
			continue
		}

		// If info is an immediate child of the path in request, just use that
		if path.Dir(info.Path) == path.Clean(ref.Path) {
			infos = append(infos, info)
			continue
		}

		// info is a nested resource, so link it to its parent closest to the path in request
		rel, err := filepath.Rel(ref.Path, info.Path)
		if err != nil {
			continue
		}
		parent := path.Join(ref.Path, strings.Split(rel, "/")[0])

		if p, ok := nestedInfos[parent]; ok {
			p.Size += info.Size
			if utils.TSToUnixNano(info.Mtime) > utils.TSToUnixNano(p.Mtime) {
				p.Mtime = info.Mtime
				p.Etag = info.Etag
			}
			if p.Etag == "" && p.Etag != info.Etag {
				p.Etag = info.Etag
			}
		} else {
			nestedInfos[parent] = &provider.ResourceInfo{
				Path: parent,
				Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				Id: &provider.ResourceId{
					OpaqueId: uuid.New().String(),
				},
				Size:     info.Size,
				Mtime:    info.Mtime,
				Etag:     info.Etag,
				MimeType: "httpd/unix-directory",
			}
		}
	}

	for _, info := range nestedInfos {
		infos = append(infos, info)
	}

	return &provider.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}, nil
}

func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	revs, err := s.storage.ListRevisions(ctx, newRef)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing file versions")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing file versions: "+req.Ref.String())
		}
		return &provider.ListFileVersionsResponse{
			Status: st,
		}, nil
	}

	sort.Sort(descendingMtime(revs))

	res := &provider.ListFileVersionsResponse{
		Status:   status.NewOK(ctx),
		Versions: revs,
	}
	return res, nil
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.RestoreRevision(ctx, newRef, req.Key); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when restoring file versions")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error restoring version: "+req.Ref.String())
		}
		return &provider.RestoreFileVersionResponse{
			Status: st,
		}, nil
	}

	res := &provider.RestoreFileVersionResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListRecycleStream(req *provider.ListRecycleStreamRequest, ss provider.ProviderAPI_ListRecycleStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	ref, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return err
	}

	key, itemPath := router.ShiftPath(req.Key)
	items, err := s.storage.ListRecycle(ctx, ref.GetPath(), key, itemPath)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing recycle stream")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing recycle stream")
		}
		res := &provider.ListRecycleStreamResponse{
			Status: st,
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListRecycleStream: error sending response")
			return err
		}
		return nil
	}

	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	for _, item := range items {
		res := &provider.ListRecycleStreamResponse{
			RecycleItem: item,
			Status:      status.NewOK(ctx),
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListRecycleStream: error sending response")
			return err
		}
	}
	return nil
}

func (s *service) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	ref, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}
	key, itemPath := router.ShiftPath(req.Key)
	items, err := s.storage.ListRecycle(ctx, ref.GetPath(), key, itemPath)
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing recycle")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing recycle")
		}
		return &provider.ListRecycleResponse{
			Status: st,
		}, nil
	}

	prefixMountpoint := utils.IsAbsoluteReference(req.Ref)
	for _, md := range items {
		if err := s.wrapReference(ctx, md.Ref, prefixMountpoint); err != nil {
			return &provider.ListRecycleResponse{
				Status: status.NewInternal(ctx, err, "error wrapping path"),
			}, nil
		}
	}

	res := &provider.ListRecycleResponse{
		Status:       status.NewOK(ctx),
		RecycleItems: items,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	ref, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}
	key, itemPath := router.ShiftPath(req.Key)
	if err := s.storage.RestoreRecycleItem(ctx, ref.GetPath(), key, itemPath, req.RestoreRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when restoring recycle bin item")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error restoring recycle bin item")
		}
		return &provider.RestoreRecycleItemResponse{
			Status: st,
		}, nil
	}

	res := &provider.RestoreRecycleItemResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	ref, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}
	// if a key was sent as opaque id purge only that item
	key, itemPath := router.ShiftPath(req.Key)
	if key != "" {
		if err := s.storage.PurgeRecycleItem(ctx, ref.GetPath(), key, itemPath); err != nil {
			var st *rpc.Status
			switch err.(type) {
			case errtypes.IsNotFound:
				st = status.NewNotFound(ctx, "path not found when purging recycle item")
			case errtypes.PermissionDenied:
				st = status.NewPermissionDenied(ctx, err, "permission denied")
			default:
				st = status.NewInternal(ctx, err, "error purging recycle item")
			}
			return &provider.PurgeRecycleResponse{
				Status: st,
			}, nil
		}
	} else if err := s.storage.EmptyRecycle(ctx); err != nil {
		// otherwise try emptying the whole recycle bin
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when purging recycle bin")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error purging recycle bin")
		}
		return &provider.PurgeRecycleResponse{
			Status: st,
		}, nil
	}

	res := &provider.PurgeRecycleResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListGrants(ctx context.Context, req *provider.ListGrantsRequest) (*provider.ListGrantsResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.ListGrantsResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	grants, err := s.storage.ListGrants(ctx, newRef)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing grants")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error listing grants")
		}
		return &provider.ListGrantsResponse{
			Status: st,
		}, nil
	}

	res := &provider.ListGrantsResponse{
		Status: status.NewOK(ctx),
		Grants: grants,
	}
	return res, nil
}

func (s *service) DenyGrant(ctx context.Context, req *provider.DenyGrantRequest) (*provider.DenyGrantResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.DenyGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	// check grantee type is valid
	if req.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_INVALID {
		return &provider.DenyGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	err = s.storage.DenyGrant(ctx, newRef, req.Grantee)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting grants")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error setting grants")
		}
		return &provider.DenyGrantResponse{
			Status: st,
		}, nil
	}

	res := &provider.DenyGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) AddGrant(ctx context.Context, req *provider.AddGrantRequest) (*provider.AddGrantResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.AddGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	// check grantee type is valid
	if req.Grant.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_INVALID {
		return &provider.AddGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	err = s.storage.AddGrant(ctx, newRef, req.Grant)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting grants")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error setting grants")
		}
		return &provider.AddGrantResponse{
			Status: st,
		}, nil
	}

	res := &provider.AddGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) UpdateGrant(ctx context.Context, req *provider.UpdateGrantRequest) (*provider.UpdateGrantResponse, error) {
	// check grantee type is valid
	if req.Grant.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_INVALID {
		return &provider.UpdateGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.UpdateGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.UpdateGrant(ctx, newRef, req.Grant); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when updating grant")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error updating grant")
		}
		return &provider.UpdateGrantResponse{
			Status: st,
		}, nil
	}

	res := &provider.UpdateGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) RemoveGrant(ctx context.Context, req *provider.RemoveGrantRequest) (*provider.RemoveGrantResponse, error) {
	// check targetType is valid
	if req.Grant.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_INVALID {
		return &provider.RemoveGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.RemoveGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.RemoveGrant(ctx, newRef, req.Grant); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when removing grant")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error removing grant")
		}
		return &provider.RemoveGrantResponse{
			Status: st,
		}, nil
	}

	res := &provider.RemoveGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) CreateReference(ctx context.Context, req *provider.CreateReferenceRequest) (*provider.CreateReferenceResponse, error) {
	log := appctx.GetLogger(ctx)

	// parse uri is valid
	u, err := url.Parse(req.TargetUri)
	if err != nil {
		log.Err(err).Msg("invalid target uri")
		return &provider.CreateReferenceResponse{
			Status: status.NewInvalidArg(ctx, "target uri is invalid: "+err.Error()),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.CreateReferenceResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.CreateReference(ctx, newRef.GetPath(), u); err != nil {
		log.Err(err).Msg("error calling CreateReference")
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating reference")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error creating reference")
		}
		return &provider.CreateReferenceResponse{
			Status: st,
		}, nil
	}

	return &provider.CreateReferenceResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return &provider.CreateSymlinkResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CreateSymlink not implemented"), "CreateSymlink not implemented"),
	}, nil
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	total, used, err := s.storage.GetQuota(ctx, newRef)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when getting quota")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error getting quota")
		}
		return &provider.GetQuotaResponse{
			Status: st,
		}, nil
	}

	res := &provider.GetQuotaResponse{
		Status:     status.NewOK(ctx),
		TotalBytes: total,
		UsedBytes:  used,
	}
	return res, nil
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func (s *service) unwrap(ctx context.Context, ref *provider.Reference) (*provider.Reference, error) {
	// all references with an id can be passed on to the driver
	// there are two cases:
	// 1. absolute id references (resource_id is set, path is empty)
	// 2. relative references (resource_id is set, path starts with a `.`)
	if ref.GetResourceId() != nil {
		return ref, nil
	}

	if !strings.HasPrefix(ref.GetPath(), "/") {
		// abort, absolute path references must start with a `/`
		return nil, errtypes.BadRequest("ref is invalid: " + ref.String())
	}

	// TODO move mount path trimming to the gateway
	fn, err := s.trimMountPrefix(ref.GetPath())
	if err != nil {
		return nil, err
	}
	return &provider.Reference{Path: fn}, nil
}

func (s *service) trimMountPrefix(fn string) (string, error) {
	if strings.HasPrefix(fn, s.mountPath) {
		return path.Join("/", strings.TrimPrefix(fn, s.mountPath)), nil
	}
	return "", errtypes.BadRequest(fmt.Sprintf("path=%q does not belong to this storage provider mount path=%q", fn, s.mountPath))
}

func (s *service) wrap(ctx context.Context, ri *provider.ResourceInfo, prefixMountpoint bool) error {
	if ri.Id.StorageId == "" {
		// For wrapper drivers, the storage ID might already be set. In that case, skip setting it
		ri.Id.StorageId = s.mountID
	}
	if prefixMountpoint {
		// TODO move mount path prefixing to the gateway
		ri.Path = path.Join(s.mountPath, ri.Path)
	}
	return nil
}

func (s *service) wrapReference(ctx context.Context, ref *provider.Reference, prefixMountpoint bool) error {
	if ref.ResourceId != nil && ref.ResourceId.StorageId == "" {
		// For wrapper drivers, the storage ID might already be set. In that case, skip setting it
		ref.ResourceId.StorageId = s.mountID
	}
	if prefixMountpoint {
		// TODO move mount path prefixing to the gateway
		ref.Path = path.Join(s.mountPath, ref.Path)
	}
	return nil
}

type descendingMtime []*provider.FileVersion

func (v descendingMtime) Len() int {
	return len(v)
}

func (v descendingMtime) Less(i, j int) bool {
	return v[i].Mtime >= v[j].Mtime
}

func (v descendingMtime) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}
