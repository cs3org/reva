// Copyright 2018-2020 CERN
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
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/sharedconf"
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
	Drivers          map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:docs/config/packages/storage/fs"`
	TmpFolder        string                            `mapstructure:"tmp_folder" docs:"/var/tmp;Path to temporary folder."`
	DataServerURL    string                            `mapstructure:"data_server_url" docs:"http://localhost/data;The URL for the data server."`
	ExposeDataServer bool                              `mapstructure:"expose_data_server" docs:"false;Whether to expose data server."` // if true the client will be able to upload/download directly to it
	DisableTus       bool                              `mapstructure:"disable_tus" docs:"false;Whether to disable TUS uploads."`
	AvailableXS      map[string]uint32                 `mapstructure:"available_checksums" docs:"nil;List of available checksums."`
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

	c.DataServerURL = sharedconf.GetDataGateway(c.DataServerURL)

	// TODO: Uploads currently don't work when ExposeDataServer is false
	c.ExposeDataServer = true

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
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "ref not found when setting arbitrary metadata")
		} else {
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
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "path not found when unsetting arbitrary metadata")
		} else {
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
	// TODO(labkode): maybe add some checks before download starts?
	// TODO(labkode): maybe add short-lived token?
	// We now simply point the client to the data server.
	// For example, https://data-server.example.org/home/docs/myfile.txt
	// or ownclouds://data-server.example.org/home/docs/myfile.txt
	log := appctx.GetLogger(ctx)
	url := *s.dataServerURL
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	url.Path = path.Join("/", url.Path, newRef.GetPath())
	log.Info().Str("data-server", url.String()).Str("fn", req.Ref.GetPath()).Msg("file download")
	res := &provider.InitiateFileDownloadResponse{
		DownloadEndpoint: url.String(),
		Status:           status.NewOK(ctx),
		Expose:           s.conf.ExposeDataServer,
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
	url := *s.dataServerURL
	if s.conf.DisableTus {
		url.Path = path.Join("/", url.Path, newRef.GetPath())
	} else {
		var uploadLength int64
		if req.Opaque != nil && req.Opaque.Map != nil && req.Opaque.Map["Upload-Length"] != nil {
			var err error
			uploadLength, err = strconv.ParseInt(string(req.Opaque.Map["Upload-Length"].Value), 10, 64)
			if err != nil {
				return &provider.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, err, "error parsing upload length"),
				}, nil
			}
		}
		uploadID, err := s.storage.InitiateUpload(ctx, newRef, uploadLength)
		if err != nil {
			return &provider.InitiateFileUploadResponse{
				Status: status.NewInternal(ctx, err, "error getting upload id"),
			}, nil
		}
		url.Path = path.Join("/", url.Path, uploadID)
	}

	log.Info().Str("data-server", url.String()).
		Str("fn", req.Ref.GetPath()).
		Str("xs", fmt.Sprintf("%+v", s.conf.AvailableXS)).
		Msg("file upload")
	res := &provider.InitiateFileUploadResponse{
		UploadEndpoint:     url.String(),
		Status:             status.NewOK(ctx),
		AvailableChecksums: s.availableXS,
		Expose:             s.conf.ExposeDataServer,
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

	if err := s.storage.Delete(ctx, newRef); err != nil {
		var st *rpc.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "file not found")
		} else {
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
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error moving file"),
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

	md, err := s.storage.GetMD(ctx, newRef)
	if err != nil {
		var st *rpc.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "file not found")
		} else {
			st = status.NewInternal(ctx, err, "error stating file: "+req.Ref.String())
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

	mds, err := s.storage.ListFolder(ctx, newRef)
	if err != nil {
		res := &provider.ListContainerStreamResponse{
			Status: status.NewInternal(ctx, err, "error listing folder"),
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

	mds, err := s.storage.ListFolder(ctx, newRef)
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error listing folder"),
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
		return &provider.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error listing file versions"),
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
		return &provider.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error restoring version"),
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
		res := &provider.ListRecycleStreamResponse{
			Status: status.NewInternal(ctx, err, "error listing recycle"),
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
		return &provider.ListRecycleResponse{
			Status: status.NewInternal(ctx, err, "error listing recycle bin"),
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
	if err := s.storage.RestoreRecycleItem(ctx, req.Key); err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewInternal(ctx, err, "error restoring recycle bin item"),
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
			return &provider.PurgeRecycleResponse{
				Status: status.NewInternal(ctx, err, "error purging recycle item"),
			}, nil
		}
	} else if err := s.storage.EmptyRecycle(ctx); err != nil {
		// otherwise try emptying the whole recycle bin
		return &provider.PurgeRecycleResponse{
			Status: status.NewInternal(ctx, err, "error emptying recycle bin"),
		}, nil
	}

	res := &provider.PurgeRecycleResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListGrants(ctx context.Context, req *provider.ListGrantsRequest) (*provider.ListGrantsResponse, error) {
	return nil, nil
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
		return &provider.AddGrantResponse{
			Status: status.NewInternal(ctx, err, "error setting ACL"),
		}, nil
	}

	res := &provider.AddGrantResponse{
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
		return &provider.CreateReferenceResponse{
			Status: status.NewInternal(ctx, err, "error creating reference"),
		}, nil
	}

	return &provider.CreateReferenceResponse{
		Status: status.NewOK(ctx),
	}, nil
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
		return &provider.UpdateGrantResponse{
			Status: status.NewInternal(ctx, err, "error updating ACL"),
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
		return &provider.RemoveGrantResponse{
			Status: status.NewInternal(ctx, err, "error removing ACL"),
		}, nil
	}

	res := &provider.RemoveGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	total, used, err := s.storage.GetQuota(ctx)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewInternal(ctx, err, "error getting quota"),
		}, nil
	}

	res := &provider.GetQuotaResponse{
		Status:     status.NewOK(ctx),
		TotalBytes: uint64(total),
		UsedBytes:  uint64(used),
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
					StorageId: "", // on purpose, we are unwrapping, bottom layers only need OpaqueId.
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
	ri.Id.StorageId = s.mountID
	ri.Path = path.Join(s.mountPath, ri.Path)
	return nil
}
