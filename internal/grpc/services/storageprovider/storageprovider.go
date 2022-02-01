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
	"fmt"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("storageprovider", New)
}

type config struct {
	Driver           string                            `mapstructure:"driver" docs:"localhome;The storage driver to be used."`
	Drivers          map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:pkg/storage/fs/localhome/localhome.go"`
	TmpFolder        string                            `mapstructure:"tmp_folder" docs:"/var/tmp;Path to temporary folder."`
	DataServerURL    string                            `mapstructure:"data_server_url" docs:"http://localhost/data;The URL for the data server."`
	ExposeDataServer bool                              `mapstructure:"expose_data_server" docs:"false;Whether to expose data server."` // if true the client will be able to upload/download directly to it
	AvailableXS      map[string]uint32                 `mapstructure:"available_checksums" docs:"nil;List of available checksums."`
	MimeTypes        map[string]string                 `mapstructure:"mimetypes" docs:"nil;List of supported mime types and corresponding file extensions."`
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "localhome"
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
	if c.MimeTypes == nil || len(c.MimeTypes) == 0 {
		c.MimeTypes = map[string]string{".zmd": "application/compressed-markdown"}
	}

}

type service struct {
	conf          *config
	storage       storage.FS
	tmpFolder     string
	dataServerURL *url.URL
	availableXS   []*provider.ResourceChecksumPriority
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

	registerMimeTypes(c.MimeTypes)

	service := &service{
		conf:          c,
		storage:       fs,
		tmpFolder:     c.TmpFolder,
		dataServerURL: u,
		availableXS:   xsTypes,
	}

	return service, nil
}

func registerMimeTypes(mimes map[string]string) {
	for k, v := range mimes {
		mime.RegisterMime(k, v)
	}
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	if err := s.storage.SetArbitraryMetadata(ctx, req.Ref, req.ArbitraryMetadata); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting arbitrary metadata")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error setting arbitrary metadata: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Msg("failed to set arbitrary metadata")
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
	if err := s.storage.UnsetArbitraryMetadata(ctx, req.Ref, req.ArbitraryMetadataKeys); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when unsetting arbitrary metadata")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error unsetting arbitrary metadata: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Msg("failed to unset arbitrary metadata")
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
	if err := s.storage.SetLock(ctx, req.Ref, req.Lock); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting lock")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, fmt.Sprintf("error setting lock %s: %s", req.Ref.String(), err))
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
	var lock *provider.Lock
	var err error
	if lock, err = s.storage.GetLock(ctx, req.Ref); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when getting lock")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, fmt.Sprintf("error getting lock %s: %s", req.Ref.String(), err))
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
	if err := s.storage.RefreshLock(ctx, req.Ref, req.Lock); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when refreshing lock")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, fmt.Sprintf("error refreshing lock %s: %s", req.Ref.String(), err))
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
	if err := s.storage.Unlock(ctx, req.Ref); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when unlocking")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, fmt.Sprintf("error unlocking %s: %s", req.Ref.String(), err))
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
		// Currently, we only support the simple protocol for GET requests
		// Once we have multiple protocols, this would be moved to the fs layer
		protocol.Protocol = "simple"
		u.Path = path.Join(u.Path, "simple", req.Ref.GetPath())
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
	if req.Ref.GetPath() == "/" {
		return &provider.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, "can't upload to mount path"),
		}, nil
	}

	metadata := map[string]string{}
	var uploadLength int64
	if req.Opaque != nil && req.Opaque.Map != nil {
		if req.Opaque.Map["Upload-Length"] != nil {
			var err error
			uploadLength, err = strconv.ParseInt(string(req.Opaque.Map["Upload-Length"].Value), 10, 64)
			if err != nil {
				log.Error().Err(err).Msg("error parsing upload length")
				return &provider.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, "error parsing upload length"),
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
	uploadIDs, err := s.storage.InitiateUpload(ctx, req.Ref, uploadLength, metadata)
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
			st = status.NewInternal(ctx, "error getting upload id: "+req.Ref.String())
		}
		log.Error().
			Err(err).
			Interface("status", st).
			Msg("failed to initiate upload")
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
		appctx.GetLogger(ctx).Error().
			Err(err).
			Interface("resource_id", req.ResourceId).
			Msg("error getting path by id")
		return &provider.GetPathResponse{
			Status: status.NewInternal(ctx, "error getting path by id"),
		}, nil
	}
	res := &provider.GetPathResponse{
		Path:   fn,
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return nil, errtypes.NotSupported("unused, use the gateway to look up the user home")
}

func (s *service) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	return nil, errtypes.NotSupported("use CreateStorageSpace with type personal")
}

// CreateStorageSpace creates a storage space
func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	resp, err := s.storage.CreateStorageSpace(ctx, req)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "not found when creating space")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.NotSupported:
			// if trying to create a user home fall back to CreateHome
			if u, ok := ctxpkg.ContextGetUser(ctx); ok && req.Type == "personal" && utils.UserEqual(req.GetOwner().Id, u.Id) {
				if err := s.storage.CreateHome(ctx); err != nil {
					st = status.NewInternal(ctx, "error creating home")
				} else {
					st = status.NewOK(ctx)
					// TODO we cannot return a space, but the gateway currently does not expect one...
				}
			} else {
				st = status.NewUnimplemented(ctx, err, "not implemented")
			}
		case errtypes.AlreadyExists:
			st = status.NewAlreadyExists(ctx, err, "already exists")
		default:
			st = status.NewInternal(ctx, "error creating space")
			appctx.GetLogger(ctx).
				Error().
				Err(err).
				Interface("status", st).
				Interface("request", req).
				Msg("failed to create storage space")
		}
		return &provider.CreateStorageSpaceResponse{
			Status: st,
		}, nil
	}

	return resp, nil
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	log := appctx.GetLogger(ctx)

	var includeTrashed bool
	if req.Opaque != nil {
		// let's do as simple as possible until we have proper solution
		_, includeTrashed = req.Opaque.Map["includeTrashed"]
		ctx = context.WithValue(ctx, utils.ContextKeyIncludeTrash{}, includeTrashed)
	}

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
			st = status.NewInternal(ctx, "error listing spaces")
		}
		log.Error().
			Err(err).
			Interface("status", st).
			Interface("filters", req.Filters).
			Msg("failed to list storage spaces")
		return &provider.ListStorageSpacesResponse{
			Status: st,
		}, nil
	}

	for i := range spaces {
		if spaces[i].Id == nil || spaces[i].Id.OpaqueId == "" {
			log.Error().Str("service", "storageprovider").Str("driver", s.conf.Driver).Interface("space", spaces[i]).Msg("space is missing space id and root id")
		}
	}

	return &provider.ListStorageSpacesResponse{
		Status:        status.NewOK(ctx),
		StorageSpaces: spaces,
	}, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	res, err := s.storage.UpdateStorageSpace(ctx, req)
	if err != nil {
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("req", req).
			Msg("failed to update storage space")
		return nil, err
	}
	return res, nil
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	if err := s.storage.DeleteStorageSpace(ctx, req); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "not found when deleting space")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		case errtypes.BadRequest:
			st = status.NewInvalidArg(ctx, err.Error())
		default:
			st = status.NewInternal(ctx, "error deleting space: "+req.Id.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("storage_space_id", req.Id).
			Msg("failed to delete storage space")
		return &provider.DeleteStorageSpaceResponse{
			Status: st,
		}, nil
	}

	res := &provider.DeleteStorageSpaceResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	if err := s.storage.CreateDir(ctx, req.Ref); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating container")
		case errtypes.AlreadyExists:
			st = status.NewAlreadyExists(ctx, err, "container already exists")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error creating container: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to create container")
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
	if err := s.storage.TouchFile(ctx, req.Ref); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when touching the file")
		case errtypes.AlreadyExists:
			st = status.NewAlreadyExists(ctx, err, "file already exists")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error touching file: "+req.Ref.String())
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
	if req.Ref.GetPath() == "/" {
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, "can't delete mount path"),
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

	if err := s.storage.Delete(ctx, req.Ref); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating container")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error deleting file: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to delete")
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
	if err := s.storage.Move(ctx, req.Source, req.Destination); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when moving")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error moving: "+req.Source.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("source_reference", req.Source).
			Interface("target_reference", req.Destination).
			Msg("failed to move")
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

	md, err := s.storage.GetMD(ctx, req.Ref, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when statting")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error statting: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to stat")
		return &provider.StatResponse{
			Status: st,
		}, nil
	}

	res := &provider.StatResponse{
		Status: status.NewOK(ctx),
		Info:   md,
	}
	return res, nil
}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	mds, err := s.storage.ListFolder(ctx, req.Ref, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing container")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error listing container: "+req.Ref.String())
		}
		log.Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to list folder (stream)")
		res := &provider.ListContainerStreamResponse{
			Status: st,
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListContainerStream: error sending response")
			return err
		}
		return nil
	}

	for _, md := range mds {
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
	mds, err := s.storage.ListFolder(ctx, req.Ref, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing container")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error listing container: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to list folder")
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}

	res := &provider.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  mds,
	}
	return res, nil
}

func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	revs, err := s.storage.ListRevisions(ctx, req.Ref)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing file versions")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error listing file versions: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to list file versions")
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
	if err := s.storage.RestoreRevision(ctx, req.Ref, req.Key); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when restoring file versions")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error restoring version: "+req.Ref.String())
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Str("key", req.Key).
			Msg("failed to restore file version")
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

	key, itemPath := router.ShiftPath(req.Key)
	items, err := s.storage.ListRecycle(ctx, req.Ref, key, itemPath)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing recycle stream")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error listing recycle stream")
		}
		log.Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Str("key", req.Key).
			Msg("failed to list recycle (stream)")
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
	key, itemPath := router.ShiftPath(req.Key)
	items, err := s.storage.ListRecycle(ctx, req.Ref, key, itemPath)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing recycle")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error listing recycle")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Str("key", req.Key).
			Msg("failed to list recycle")
		return &provider.ListRecycleResponse{
			Status: st,
		}, nil
	}

	res := &provider.ListRecycleResponse{
		Status:       status.NewOK(ctx),
		RecycleItems: items,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	key, itemPath := router.ShiftPath(req.Key)
	if err := s.storage.RestoreRecycleItem(ctx, req.Ref, key, itemPath, req.RestoreRef); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when restoring recycle bin item")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error restoring recycle bin item")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Str("key", req.Key).
			Msg("failed to restore recycle item")
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
	// if a key was sent as opaque id purge only that item
	key, itemPath := router.ShiftPath(req.Key)
	if key != "" {
		if err := s.storage.PurgeRecycleItem(ctx, req.Ref, key, itemPath); err != nil {
			st := status.NewStatusFromErrType(ctx, "error purging recycle item", err)
			appctx.GetLogger(ctx).
				Error().
				Err(err).
				Interface("status", st).
				Interface("reference", req.Ref).
				Str("key", req.Key).
				Msg("failed to purge recycle item")
			return &provider.PurgeRecycleResponse{
				Status: st,
			}, nil
		}
	} else if err := s.storage.EmptyRecycle(ctx, req.Ref); err != nil {
		// otherwise try emptying the whole recycle bin
		st := status.NewStatusFromErrType(ctx, "error emptying recycle", err)
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Str("key", req.Key).
			Msg("failed to empty recycle")
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
	grants, err := s.storage.ListGrants(ctx, req.Ref)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when listing grants")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error listing grants")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to list grants")
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
	// check grantee type is valid
	if req.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_INVALID {
		return &provider.DenyGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	err := s.storage.DenyGrant(ctx, req.Ref, req.Grantee)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.NotSupported:
			// ignore - setting storage grants is optional
			return &provider.DenyGrantResponse{
				Status: status.NewOK(ctx),
			}, nil
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting grants")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error setting grants")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to deny grant")
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
	// TODO: update CS3 APIs
	if req.Opaque != nil {
		_, spacegrant := req.Opaque.Map["spacegrant"]
		if spacegrant {
			ctx = context.WithValue(ctx, utils.SpaceGrant, struct{}{})
		}

	}

	// check grantee type is valid
	if req.Grant.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_INVALID {
		return &provider.AddGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	err := s.storage.AddGrant(ctx, req.Ref, req.Grant)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.NotSupported:
			// ignore - setting storage grants is optional
			return &provider.AddGrantResponse{
				Status: status.NewOK(ctx),
			}, nil
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when setting grants")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error setting grants")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to add grant")
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

	if err := s.storage.UpdateGrant(ctx, req.Ref, req.Grant); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.NotSupported:
			// ignore - setting storage grants is optional
			return &provider.UpdateGrantResponse{
				Status: status.NewOK(ctx),
			}, nil
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when updating grant")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error updating grant")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to update grant")
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

	if err := s.storage.RemoveGrant(ctx, req.Ref, req.Grant); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when removing grant")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error removing grant")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to remove grant")
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
		log.Error().Err(err).Msg("invalid target uri")
		return &provider.CreateReferenceResponse{
			Status: status.NewInvalidArg(ctx, "target uri is invalid: "+err.Error()),
		}, nil
	}

	if err := s.storage.CreateReference(ctx, req.Ref.GetPath(), u); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating reference")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error creating reference")
		}
		log.Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to create reference")
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
	total, used, err := s.storage.GetQuota(ctx, req.Ref)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when getting quota")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, "error getting quota")
		}
		appctx.GetLogger(ctx).
			Error().
			Err(err).
			Interface("status", st).
			Interface("reference", req.Ref).
			Msg("failed to get quota")
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
