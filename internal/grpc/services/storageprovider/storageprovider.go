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
	// "encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	// link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("storageprovider", New)
}

type config struct {
	MountPath        string                            `mapstructure:"mount_path" docs:"/;The path where the file system would be mounted."`
	MountID          string                            `mapstructure:"mount_id" docs:"-;The ID of the mounted file system."`
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
			return nil, fmt.Errorf("checksum type is invalid: %s", xs)
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
		return nil, fmt.Errorf("no available checksum, please set in config")
	}

	registerMimeTypes(c.MimeTypes)

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

func registerMimeTypes(mimes map[string]string) {
	for k, v := range mimes {
		mime.RegisterMime(k, v)
	}
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

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	// TODO(labkode): maybe add some checks before download starts? eg. check permissions?
	// TODO(labkode): maybe add short-lived token?
	// We now simply point the client to the data server.
	// For example, https://data-server.example.org/home/docs/myfile.txt
	// or ownclouds://data-server.example.org/home/docs/myfile.txt
	log := appctx.GetLogger(ctx)
	u := *s.dataServerURL
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	// Currently, we only support the simple protocol for GET requests
	// Once we have multiple protocols, this would be moved to the fs layer
	u.Path = path.Join(u.Path, "simple", newRef.GetPath())

	log.Info().Str("data-server", u.String()).Str("fn", req.Ref.GetPath()).Msg("file download")
	res := &provider.InitiateFileDownloadResponse{
		Protocols: []*provider.FileDownloadProtocol{
			&provider.FileDownloadProtocol{
				Protocol:         "simple",
				DownloadEndpoint: u.String(),
				Expose:           s.conf.ExposeDataServer,
			},
		},
		Status: status.NewOK(ctx),
	}
	return res, nil
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
			Status: status.NewInternal(ctx, errors.New("can't upload to mount path"), "can't upload to mount path"),
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
			// TODO TUS uses a custom ChecksumMismatch 460 http status which is in an unnasigned range in
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

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return &provider.CreateStorageSpaceResponse{
		Status: status.NewUnimplemented(ctx, errors.New("CreateStorageSpace not implemented"), "CreateStorageSpace not implemented"),
	}, nil
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	return &provider.ListStorageSpacesResponse{
		Status: status.NewUnimplemented(ctx, errors.New("ListStorageSpaces not implemented"), "ListStorageSpaces not implemented"),
	}, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return &provider.UpdateStorageSpaceResponse{
		Status: status.NewUnimplemented(ctx, errors.New("UpdateStorageSpace not implemented"), "UpdateStorageSpace not implemented"),
	}, nil
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return &provider.DeleteStorageSpaceResponse{
		Status: status.NewUnimplemented(ctx, errors.New("DeleteStorageSpace not implemented"), "DeleteStorageSpace not implemented"),
	}, nil
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.CreateDir(ctx, newRef.GetPath()); err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when creating container")
		case errtypes.AlreadyExists:
			st = status.NewInternal(ctx, err, "error: container already exists")
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

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	if newRef.GetPath() == "/" {
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, errors.New("can't delete mount path"), "can't delete mount path"),
		}, nil
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
	ctx, span := trace.StartSpan(ctx, "Stat")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	md, err := s.storage.GetMD(ctx, newRef, req.ArbitraryMetadataKeys)
	if err != nil {
		var st *rpc.Status
		switch err.(type) {
		case errtypes.IsNotFound:
			st = status.NewNotFound(ctx, "path not found when stating")
		case errtypes.PermissionDenied:
			st = status.NewPermissionDenied(ctx, err, "permission denied")
		default:
			st = status.NewInternal(ctx, err, "error stating: "+req.Ref.String())
		}
		return &provider.StatResponse{
			Status: st,
		}, nil
	}

	if err := s.wrap(ctx, md); err != nil {
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

	for _, md := range mds {
		if err := s.wrap(ctx, md); err != nil {
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
	for _, md := range mds {
		if err := s.wrap(ctx, md); err != nil {
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

	items, err := s.storage.ListRecycle(ctx)
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
	items, err := s.storage.ListRecycle(ctx)
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

	res := &provider.ListRecycleResponse{
		Status:       status.NewOK(ctx),
		RecycleItems: items,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	if err := s.storage.RestoreRecycleItem(ctx, req.Key, req.RestorePath); err != nil {
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
	// if a key was sent as opacque id purge only that item
	if req.GetRef().GetId() != nil && req.GetRef().GetId().GetOpaqueId() != "" {
		if err := s.storage.PurgeRecycleItem(ctx, req.GetRef().GetId().GetOpaqueId()); err != nil {
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

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: req.Path,
		},
	}

	newRef, err := s.unwrap(ctx, ref)
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
		Status: status.NewUnimplemented(ctx, errors.New("CreateSymlink not implemented"), "CreateSymlink not implemented"),
	}, nil
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	total, used, err := s.storage.GetQuota(ctx)
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
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) unwrap(ctx context.Context, ref *provider.Reference) (*provider.Reference, error) {
	if ref.GetId() != nil {
		idRef := &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: &provider.ResourceId{
					StorageId: "", // we are unwrapping on purpose, bottom layers only need OpaqueId.
					OpaqueId:  ref.GetId().OpaqueId,
				},
			},
		}

		return idRef, nil
	}

	if ref.GetPath() == "" {
		// abort, no valid id nor path
		return nil, errors.New("ref is invalid: " + ref.String())
	}

	fn := ref.GetPath()
	fsfn, err := s.trimMountPrefix(fn)
	if err != nil {
		return nil, err
	}

	pathRef := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: fsfn,
		},
	}

	return pathRef, nil
}

func (s *service) trimMountPrefix(fn string) (string, error) {
	if strings.HasPrefix(fn, s.mountPath) {
		return path.Join("/", strings.TrimPrefix(fn, s.mountPath)), nil
	}
	return "", errors.New(fmt.Sprintf("path=%q does not belong to this storage provider mount path=%q"+fn, s.mountPath))
}

func (s *service) wrap(ctx context.Context, ri *provider.ResourceInfo) error {
	if ri.Id.StorageId == "" {
		// For wrapper drivers, the storage ID might already be set. In that case, skip setting it
		ri.Id.StorageId = s.mountID
	}
	ri.Path = path.Join(s.mountPath, ri.Path)
	return nil
}
