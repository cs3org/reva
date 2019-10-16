// Copyright 2018-2019 CERN
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
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
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
	Driver           string                            `mapstructure:"driver"`
	MountPath        string                            `mapstructure:"mount_path"`
	MountID          string                            `mapstructure:"mount_id"`
	TmpFolder        string                            `mapstructure:"tmp_folder"`
	Drivers          map[string]map[string]interface{} `mapstructure:"drivers"`
	DataServerURL    string                            `mapstructure:"data_server_url"`
	ExposeDataServer bool                              `mapstructure:"expose_data_server"` // if true the client will be able to upload/download directly to it
	AvailableXS      map[string]uint32                 `mapstructure:"available_checksums"`
}

type service struct {
	conf               *config
	storage            storage.FS
	mountPath, mountID string
	tmpFolder          string
	dataServerURL      *url.URL
	availableXS        []*storageproviderv0alphapb.ResourceChecksumPriority
}

func (s *service) Close() error {
	return s.storage.Shutdown(context.Background())
}

func parseXSTypes(xsTypes map[string]uint32) ([]*storageproviderv0alphapb.ResourceChecksumPriority, error) {
	var types = make([]*storageproviderv0alphapb.ResourceChecksumPriority, 0, len(xsTypes))
	for xs, prio := range xsTypes {
		t := PKG2GRPCXS(xs)
		if t == storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID {
			return nil, fmt.Errorf("checksum type is invalid: %s", xs)
		}
		xsPrio := &storageproviderv0alphapb.ResourceChecksumPriority{
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
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// use os temporary folder if empty
	tmpFolder := c.TmpFolder
	if tmpFolder == "" {
		tmpFolder = os.TempDir()
	}

	if err := os.MkdirAll(tmpFolder, 0755); err != nil {
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
		tmpFolder:     tmpFolder,
		mountPath:     mountPath,
		mountID:       mountID,
		dataServerURL: u,
		availableXS:   xsTypes,
	}

	storageproviderv0alphapb.RegisterStorageProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *storageproviderv0alphapb.SetArbitraryMetadataRequest) (*storageproviderv0alphapb.SetArbitraryMetadataResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &storageproviderv0alphapb.SetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error setting arbitrary metadata"),
		}, nil
	}

	if err := s.storage.SetArbitraryMetadata(ctx, newRef, req.ArbitraryMetadata); err != nil {
		var st *rpcpb.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "ref not found when setting arbitrary metadata")
		} else {
			st = status.NewInternal(ctx, err, "error setting arbitrary metadata: "+req.Ref.String())
		}
		return &storageproviderv0alphapb.SetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv0alphapb.SetArbitraryMetadataResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *storageproviderv0alphapb.UnsetArbitraryMetadataRequest) (*storageproviderv0alphapb.UnsetArbitraryMetadataResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &storageproviderv0alphapb.UnsetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error unsetting arbitrary metadata"),
		}, nil
	}

	if err := s.storage.UnsetArbitraryMetadata(ctx, newRef, req.ArbitraryMetadataKeys); err != nil {
		var st *rpcpb.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "path not found when unsetting arbitrary metadata")
		} else {
			st = status.NewInternal(ctx, err, "error unsetting arbitrary metadata: "+req.Ref.String())
		}
		return &storageproviderv0alphapb.UnsetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv0alphapb.UnsetArbitraryMetadataResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetProvider(ctx context.Context, req *storageproviderv0alphapb.GetProviderRequest) (*storageproviderv0alphapb.GetProviderResponse, error) {
	provider := &storagetypespb.ProviderInfo{
		// Address:  ? TODO(labkode): how to obtain the addresss the service is listening to? not very useful if the request already comes to it :(
		ProviderId:   s.mountID,
		ProviderPath: s.mountPath,
		// Description:  s.description, TODO(labkode): add to config
		// Features: ? TODO(labkode):
	}
	res := &storageproviderv0alphapb.GetProviderResponse{
		Info:   provider,
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) InitiateFileDownload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileDownloadRequest) (*storageproviderv0alphapb.InitiateFileDownloadResponse, error) {
	// TODO(labkode): maybe add some checks before download starts?
	// TODO(labkode): maybe add short-lived token?
	// We now simply point the client to the data server.
	// For example, https://data-server.example.org/home/docs/myfile.txt
	// or ownclouds://data-server.example.org/home/docs/myfile.txt
	log := appctx.GetLogger(ctx)
	url := *s.dataServerURL
	url.Path = path.Join("/", url.Path, path.Clean(req.Ref.GetPath()))
	log.Info().Str("data-server", url.String()).Str("fn", req.Ref.GetPath()).Msg("file download")
	res := &storageproviderv0alphapb.InitiateFileDownloadResponse{
		DownloadEndpoint: url.String(),
		Status:           status.NewOK(ctx),
		Expose:           s.conf.ExposeDataServer,
	}
	return res, nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileUploadRequest) (*storageproviderv0alphapb.InitiateFileUploadResponse, error) {
	// TODO(labkode): same considerations as download
	log := appctx.GetLogger(ctx)
	url := *s.dataServerURL
	url.Path = path.Join("/", url.Path, path.Clean(req.Ref.GetPath()))
	log.Info().Str("data-server", url.String()).
		Str("fn", req.Ref.GetPath()).
		Str("xs", fmt.Sprintf("%+v", s.conf.AvailableXS)).
		Msg("file upload")
	res := &storageproviderv0alphapb.InitiateFileUploadResponse{
		UploadEndpoint:     url.String(),
		Status:             status.NewOK(ctx),
		AvailableChecksums: s.availableXS,
		Expose:             s.conf.ExposeDataServer,
	}
	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *storageproviderv0alphapb.GetPathRequest) (*storageproviderv0alphapb.GetPathResponse, error) {
	// TODO(labkode): check that the storage ID is the same as the storage provider id.
	fn, err := s.storage.GetPathByID(ctx, req.ResourceId)
	if err != nil {
		return &storageproviderv0alphapb.GetPathResponse{
			Status: status.NewInternal(ctx, err, "error getting path by id"),
		}, nil
	}

	fn = path.Join(s.mountPath, path.Clean(fn))
	res := &storageproviderv0alphapb.GetPathResponse{
		Path:   fn,
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) CreateContainer(ctx context.Context, req *storageproviderv0alphapb.CreateContainerRequest) (*storageproviderv0alphapb.CreateContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.CreateDir(ctx, newRef.GetPath()); err != nil {
		var st *rpcpb.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "path not found when creating container")
		} else {
			st = status.NewInternal(ctx, err, "error creating container: "+req.Ref.String())
		}
		return &storageproviderv0alphapb.CreateContainerResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv0alphapb.CreateContainerResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Delete(ctx context.Context, req *storageproviderv0alphapb.DeleteRequest) (*storageproviderv0alphapb.DeleteResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.DeleteResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.Delete(ctx, newRef); err != nil {
		var st *rpcpb.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "file not found")
		} else {
			st = status.NewInternal(ctx, err, "error deleting file: "+req.Ref.String())
		}
		return &storageproviderv0alphapb.DeleteResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv0alphapb.DeleteResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Move(ctx context.Context, req *storageproviderv0alphapb.MoveRequest) (*storageproviderv0alphapb.MoveResponse, error) {
	sourceRef, err := s.unwrap(ctx, req.Source)
	if err != nil {
		return &storageproviderv0alphapb.MoveResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping source path"),
		}, nil
	}
	targetRef, err := s.unwrap(ctx, req.Destination)
	if err != nil {
		return &storageproviderv0alphapb.MoveResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping destination path"),
		}, nil
	}

	if err := s.storage.Move(ctx, sourceRef, targetRef); err != nil {
		return &storageproviderv0alphapb.MoveResponse{
			Status: status.NewInternal(ctx, err, "error moving file"),
		}, nil
	}

	res := &storageproviderv0alphapb.MoveResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Stat(ctx context.Context, req *storageproviderv0alphapb.StatRequest) (*storageproviderv0alphapb.StatResponse, error) {
	ctx, span := trace.StartSpan(ctx, "Stat")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.StatResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	md, err := s.storage.GetMD(ctx, newRef)
	if err != nil {
		var st *rpcpb.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "file not found")
		} else {
			st = status.NewInternal(ctx, err, "error stating file: "+req.Ref.String())
		}
		return &storageproviderv0alphapb.StatResponse{
			Status: st,
		}, nil
	}

	s.wrap(md)
	res := &storageproviderv0alphapb.StatResponse{
		Status: status.NewOK(ctx),
		Info:   md,
	}
	return res, nil
}

func (s *service) ListContainerStream(req *storageproviderv0alphapb.ListContainerStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListContainerStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		res := &storageproviderv0alphapb.ListContainerStreamResponse{
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
		res := &storageproviderv0alphapb.ListContainerStreamResponse{
			Status: status.NewInternal(ctx, err, "error listing folder"),
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("ListContainerStream: error sending response")
			return err
		}
		return nil
	}

	for _, md := range mds {
		s.wrap(md)
		res := &storageproviderv0alphapb.ListContainerStreamResponse{
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

func (s *service) ListContainer(ctx context.Context, req *storageproviderv0alphapb.ListContainerRequest) (*storageproviderv0alphapb.ListContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	mds, err := s.storage.ListFolder(ctx, newRef)
	if err != nil {
		return &storageproviderv0alphapb.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error listing folder"),
		}, nil
	}

	var infos = make([]*storageproviderv0alphapb.ResourceInfo, 0, len(mds))
	for _, md := range mds {
		s.wrap(md)
		infos = append(infos, md)
	}
	res := &storageproviderv0alphapb.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}
	return res, nil
}

func (s *service) ListFileVersions(ctx context.Context, req *storageproviderv0alphapb.ListFileVersionsRequest) (*storageproviderv0alphapb.ListFileVersionsResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	revs, err := s.storage.ListRevisions(ctx, newRef)
	if err != nil {
		return &storageproviderv0alphapb.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error listing file versions"),
		}, nil
	}

	res := &storageproviderv0alphapb.ListFileVersionsResponse{
		Status:   status.NewOK(ctx),
		Versions: revs,
	}
	return res, nil
}

func (s *service) RestoreFileVersion(ctx context.Context, req *storageproviderv0alphapb.RestoreFileVersionRequest) (*storageproviderv0alphapb.RestoreFileVersionResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.RestoreRevision(ctx, newRef, req.Key); err != nil {
		return &storageproviderv0alphapb.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error restoring version"),
		}, nil
	}

	res := &storageproviderv0alphapb.RestoreFileVersionResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListRecycleStream(req *storageproviderv0alphapb.ListRecycleStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListRecycleStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	items, err := s.storage.ListRecycle(ctx)
	if err != nil {
		res := &storageproviderv0alphapb.ListRecycleStreamResponse{
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
		res := &storageproviderv0alphapb.ListRecycleStreamResponse{
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

func (s *service) ListRecycle(ctx context.Context, req *storageproviderv0alphapb.ListRecycleRequest) (*storageproviderv0alphapb.ListRecycleResponse, error) {
	items, err := s.storage.ListRecycle(ctx)
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	if err != nil {
		return &storageproviderv0alphapb.ListRecycleResponse{
			Status: status.NewInternal(ctx, err, "error listing recycle bin"),
		}, nil
	}

	res := &storageproviderv0alphapb.ListRecycleResponse{
		Status:       status.NewOK(ctx),
		RecycleItems: items,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *storageproviderv0alphapb.RestoreRecycleItemRequest) (*storageproviderv0alphapb.RestoreRecycleItemResponse, error) {
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	if err := s.storage.RestoreRecycleItem(ctx, req.Key); err != nil {
		return &storageproviderv0alphapb.RestoreRecycleItemResponse{
			Status: status.NewInternal(ctx, err, "error restoring recycle bin item"),
		}, nil
	}

	res := &storageproviderv0alphapb.RestoreRecycleItemResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) PurgeRecycle(ctx context.Context, req *storageproviderv0alphapb.PurgeRecycleRequest) (*storageproviderv0alphapb.PurgeRecycleResponse, error) {
	if err := s.storage.EmptyRecycle(ctx); err != nil {
		return &storageproviderv0alphapb.PurgeRecycleResponse{
			Status: status.NewInternal(ctx, err, "error purging recycle bin"),
		}, nil
	}

	res := &storageproviderv0alphapb.PurgeRecycleResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListGrants(ctx context.Context, req *storageproviderv0alphapb.ListGrantsRequest) (*storageproviderv0alphapb.ListGrantsResponse, error) {
	return nil, nil
}

func (s *service) AddGrant(ctx context.Context, req *storageproviderv0alphapb.AddGrantRequest) (*storageproviderv0alphapb.AddGrantResponse, error) {
	// check grantee type is valid
	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		return &storageproviderv0alphapb.AddGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.AddGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	err = s.storage.AddGrant(ctx, newRef, req.Grant)
	if err != nil {
		return &storageproviderv0alphapb.AddGrantResponse{
			Status: status.NewInternal(ctx, err, "error setting ACL"),
		}, nil
	}

	res := &storageproviderv0alphapb.AddGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) CreateReference(ctx context.Context, req *storageproviderv0alphapb.CreateReferenceRequest) (*storageproviderv0alphapb.CreateReferenceResponse, error) {
	log := appctx.GetLogger(ctx)

	// parse uri is valid
	u, err := url.Parse(req.TargetUri)
	if err != nil {
		log.Err(err).Msg("invalid target uri")
		return &storageproviderv0alphapb.CreateReferenceResponse{
			Status: status.NewInvalidArg(ctx, "target uri is invalid: "+err.Error()),
		}, nil
	}

	if err := s.storage.CreateReference(ctx, req.Path, u); err != nil {
		log.Err(err).Msg("error calling CreateReference")
		return &storageproviderv0alphapb.CreateReferenceResponse{
			Status: status.NewInternal(ctx, err, "error creating reference"),
		}, nil
	}

	return &storageproviderv0alphapb.CreateReferenceResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) UpdateGrant(ctx context.Context, req *storageproviderv0alphapb.UpdateGrantRequest) (*storageproviderv0alphapb.UpdateGrantResponse, error) {
	// check grantee type is valid
	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		return &storageproviderv0alphapb.UpdateGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.UpdateGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.UpdateGrant(ctx, newRef, req.Grant); err != nil {
		return &storageproviderv0alphapb.UpdateGrantResponse{
			Status: status.NewInternal(ctx, err, "error updating ACL"),
		}, nil
	}

	res := &storageproviderv0alphapb.UpdateGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) RemoveGrant(ctx context.Context, req *storageproviderv0alphapb.RemoveGrantRequest) (*storageproviderv0alphapb.RemoveGrantResponse, error) {
	// check targetType is valid
	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		return &storageproviderv0alphapb.RemoveGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv0alphapb.RemoveGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.RemoveGrant(ctx, newRef, req.Grant); err != nil {
		return &storageproviderv0alphapb.RemoveGrantResponse{
			Status: status.NewInternal(ctx, err, "error removing ACL"),
		}, nil
	}

	res := &storageproviderv0alphapb.RemoveGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetQuota(ctx context.Context, req *storageproviderv0alphapb.GetQuotaRequest) (*storageproviderv0alphapb.GetQuotaResponse, error) {
	total, used, err := s.storage.GetQuota(ctx)
	if err != nil {
		return &storageproviderv0alphapb.GetQuotaResponse{
			Status: status.NewInternal(ctx, err, "error getting quota"),
		}, nil
	}

	res := &storageproviderv0alphapb.GetQuotaResponse{
		Status:     status.NewOK(ctx),
		TotalBytes: uint64(total),
		UsedBytes:  uint64(used),
	}
	return res, nil
}

func (s *service) unwrap(ctx context.Context, ref *storageproviderv0alphapb.Reference) (*storageproviderv0alphapb.Reference, error) {
	if ref.GetId() != nil {
		idRef := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Id{
				Id: &storageproviderv0alphapb.ResourceId{
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

	pathRef := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{
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

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) wrap(ri *storageproviderv0alphapb.ResourceInfo) {
	ri.Id.StorageId = s.mountID
	ri.Path = path.Join(s.mountPath, ri.Path)
}
