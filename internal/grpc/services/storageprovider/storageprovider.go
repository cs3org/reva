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
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	pwregistry "github.com/cs3org/reva/pkg/storage/pw/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("storageprovider", New)
}

type config struct {
	MountPath        string                            `mapstructure:"mount_path"`
	MountID          string                            `mapstructure:"mount_id"`
	Driver           string                            `mapstructure:"driver"`
	Drivers          map[string]map[string]interface{} `mapstructure:"drivers"`
	PathWrapper      string                            `mapstructure:"path_wrapper"`
	PathWrappers     map[string]map[string]interface{} `mapstructure:"path_wrappers"`
	TmpFolder        string                            `mapstructure:"tmp_folder"`
	DataServerURL    string                            `mapstructure:"data_server_url"`
	ExposeDataServer bool                              `mapstructure:"expose_data_server"` // if true the client will be able to upload/download directly to it
	AvailableXS      map[string]uint32                 `mapstructure:"available_checksums"`
}

type service struct {
	conf               *config
	storage            storage.FS
	pathWrapper        storage.PathWrapper
	mountPath, mountID string
	tmpFolder          string
	dataServerURL      *url.URL
	availableXS        []*storageproviderv1beta1pb.ResourceChecksumPriority
}

func (s *service) Close() error {
	return s.storage.Shutdown(context.Background())
}

func parseXSTypes(xsTypes map[string]uint32) ([]*storageproviderv1beta1pb.ResourceChecksumPriority, error) {
	var types = make([]*storageproviderv1beta1pb.ResourceChecksumPriority, 0, len(xsTypes))
	for xs, prio := range xsTypes {
		t := PKG2GRPCXS(xs)
		if t == storageproviderv1beta1pb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID {
			return nil, fmt.Errorf("checksum type is invalid: %s", xs)
		}
		xsPrio := &storageproviderv1beta1pb.ResourceChecksumPriority{
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
	pw, err := getPW(c)
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
		pathWrapper:   pw,
		tmpFolder:     tmpFolder,
		mountPath:     mountPath,
		mountID:       mountID,
		dataServerURL: u,
		availableXS:   xsTypes,
	}

	storageproviderv1beta1pb.RegisterStorageProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *storageproviderv1beta1pb.SetArbitraryMetadataRequest) (*storageproviderv1beta1pb.SetArbitraryMetadataResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &storageproviderv1beta1pb.SetArbitraryMetadataResponse{
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
		return &storageproviderv1beta1pb.SetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv1beta1pb.SetArbitraryMetadataResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *storageproviderv1beta1pb.UnsetArbitraryMetadataRequest) (*storageproviderv1beta1pb.UnsetArbitraryMetadataResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error unwrapping path")
		return &storageproviderv1beta1pb.UnsetArbitraryMetadataResponse{
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
		return &storageproviderv1beta1pb.UnsetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv1beta1pb.UnsetArbitraryMetadataResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetProvider(ctx context.Context, req *storageproviderv1beta1pb.GetProviderRequest) (*storageproviderv1beta1pb.GetProviderResponse, error) {
	provider := &storagetypespb.ProviderInfo{
		// Address:  ? TODO(labkode): how to obtain the addresss the service is listening to? not very useful if the request already comes to it :(
		ProviderId:   s.mountID,
		ProviderPath: s.mountPath,
		// Description:  s.description, TODO(labkode): add to config
		// Features: ? TODO(labkode):
	}
	res := &storageproviderv1beta1pb.GetProviderResponse{
		Info:   provider,
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) InitiateFileDownload(ctx context.Context, req *storageproviderv1beta1pb.InitiateFileDownloadRequest) (*storageproviderv1beta1pb.InitiateFileDownloadResponse, error) {
	// TODO(labkode): maybe add some checks before download starts?
	// TODO(labkode): maybe add short-lived token?
	// We now simply point the client to the data server.
	// For example, https://data-server.example.org/home/docs/myfile.txt
	// or ownclouds://data-server.example.org/home/docs/myfile.txt
	log := appctx.GetLogger(ctx)
	url := *s.dataServerURL
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	url.Path = path.Join("/", url.Path, newRef.GetPath())
	log.Info().Str("data-server", url.String()).Str("fn", req.Ref.GetPath()).Msg("file download")
	res := &storageproviderv1beta1pb.InitiateFileDownloadResponse{
		DownloadEndpoint: url.String(),
		Status:           status.NewOK(ctx),
		Expose:           s.conf.ExposeDataServer,
	}
	return res, nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *storageproviderv1beta1pb.InitiateFileUploadRequest) (*storageproviderv1beta1pb.InitiateFileUploadResponse, error) {
	// TODO(labkode): same considerations as download
	log := appctx.GetLogger(ctx)
	url := *s.dataServerURL
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}
	url.Path = path.Join("/", url.Path, newRef.GetPath())
	log.Info().Str("data-server", url.String()).
		Str("fn", req.Ref.GetPath()).
		Str("xs", fmt.Sprintf("%+v", s.conf.AvailableXS)).
		Msg("file upload")
	res := &storageproviderv1beta1pb.InitiateFileUploadResponse{
		UploadEndpoint:     url.String(),
		Status:             status.NewOK(ctx),
		AvailableChecksums: s.availableXS,
		Expose:             s.conf.ExposeDataServer,
	}
	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *storageproviderv1beta1pb.GetPathRequest) (*storageproviderv1beta1pb.GetPathResponse, error) {
	// TODO(labkode): check that the storage ID is the same as the storage provider id.
	fn, err := s.storage.GetPathByID(ctx, req.ResourceId)
	if err != nil {
		return &storageproviderv1beta1pb.GetPathResponse{
			Status: status.NewInternal(ctx, err, "error getting path by id"),
		}, nil
	}

	fn = path.Join(s.mountPath, path.Clean(fn))
	res := &storageproviderv1beta1pb.GetPathResponse{
		Path:   fn,
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) CreateContainer(ctx context.Context, req *storageproviderv1beta1pb.CreateContainerRequest) (*storageproviderv1beta1pb.CreateContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.CreateContainerResponse{
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
		return &storageproviderv1beta1pb.CreateContainerResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv1beta1pb.CreateContainerResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Delete(ctx context.Context, req *storageproviderv1beta1pb.DeleteRequest) (*storageproviderv1beta1pb.DeleteResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.DeleteResponse{
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
		return &storageproviderv1beta1pb.DeleteResponse{
			Status: st,
		}, nil
	}

	res := &storageproviderv1beta1pb.DeleteResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Move(ctx context.Context, req *storageproviderv1beta1pb.MoveRequest) (*storageproviderv1beta1pb.MoveResponse, error) {
	sourceRef, err := s.unwrap(ctx, req.Source)
	if err != nil {
		return &storageproviderv1beta1pb.MoveResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping source path"),
		}, nil
	}
	targetRef, err := s.unwrap(ctx, req.Destination)
	if err != nil {
		return &storageproviderv1beta1pb.MoveResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping destination path"),
		}, nil
	}

	if err := s.storage.Move(ctx, sourceRef, targetRef); err != nil {
		return &storageproviderv1beta1pb.MoveResponse{
			Status: status.NewInternal(ctx, err, "error moving file"),
		}, nil
	}

	res := &storageproviderv1beta1pb.MoveResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) Stat(ctx context.Context, req *storageproviderv1beta1pb.StatRequest) (*storageproviderv1beta1pb.StatResponse, error) {
	ctx, span := trace.StartSpan(ctx, "Stat")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.StatResponse{
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
		return &storageproviderv1beta1pb.StatResponse{
			Status: st,
		}, nil
	}

	if err := s.wrap(ctx, md); err != nil {
		return &storageproviderv1beta1pb.StatResponse{
			Status: status.NewInternal(ctx, err, "error wrapping path"),
		}, nil
	}
	res := &storageproviderv1beta1pb.StatResponse{
		Status: status.NewOK(ctx),
		Info:   md,
	}
	return res, nil
}

func (s *service) ListContainerStream(req *storageproviderv1beta1pb.ListContainerStreamRequest, ss storageproviderv1beta1pb.StorageProviderService_ListContainerStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		res := &storageproviderv1beta1pb.ListContainerStreamResponse{
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
		res := &storageproviderv1beta1pb.ListContainerStreamResponse{
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
			res := &storageproviderv1beta1pb.ListContainerStreamResponse{
				Status: status.NewInternal(ctx, err, "error wrapping path"),
			}
			if err := ss.Send(res); err != nil {
				log.Error().Err(err).Msg("ListContainerStream: error sending response")
				return err
			}
			return nil
		}
		res := &storageproviderv1beta1pb.ListContainerStreamResponse{
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

func (s *service) ListContainer(ctx context.Context, req *storageproviderv1beta1pb.ListContainerRequest) (*storageproviderv1beta1pb.ListContainerResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	mds, err := s.storage.ListFolder(ctx, newRef)
	if err != nil {
		return &storageproviderv1beta1pb.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error listing folder"),
		}, nil
	}

	var infos = make([]*storageproviderv1beta1pb.ResourceInfo, 0, len(mds))
	for _, md := range mds {
		if err := s.wrap(ctx, md); err != nil {
			return &storageproviderv1beta1pb.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "error wrapping path"),
			}, nil
		}
		infos = append(infos, md)
	}
	res := &storageproviderv1beta1pb.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}
	return res, nil
}

func (s *service) ListFileVersions(ctx context.Context, req *storageproviderv1beta1pb.ListFileVersionsRequest) (*storageproviderv1beta1pb.ListFileVersionsResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	revs, err := s.storage.ListRevisions(ctx, newRef)
	if err != nil {
		return &storageproviderv1beta1pb.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error listing file versions"),
		}, nil
	}

	res := &storageproviderv1beta1pb.ListFileVersionsResponse{
		Status:   status.NewOK(ctx),
		Versions: revs,
	}
	return res, nil
}

func (s *service) RestoreFileVersion(ctx context.Context, req *storageproviderv1beta1pb.RestoreFileVersionRequest) (*storageproviderv1beta1pb.RestoreFileVersionResponse, error) {
	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.RestoreRevision(ctx, newRef, req.Key); err != nil {
		return &storageproviderv1beta1pb.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error restoring version"),
		}, nil
	}

	res := &storageproviderv1beta1pb.RestoreFileVersionResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListRecycleStream(req *storageproviderv1beta1pb.ListRecycleStreamRequest, ss storageproviderv1beta1pb.StorageProviderService_ListRecycleStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)

	items, err := s.storage.ListRecycle(ctx)
	if err != nil {
		res := &storageproviderv1beta1pb.ListRecycleStreamResponse{
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
		res := &storageproviderv1beta1pb.ListRecycleStreamResponse{
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

func (s *service) ListRecycle(ctx context.Context, req *storageproviderv1beta1pb.ListRecycleRequest) (*storageproviderv1beta1pb.ListRecycleResponse, error) {
	items, err := s.storage.ListRecycle(ctx)
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	if err != nil {
		return &storageproviderv1beta1pb.ListRecycleResponse{
			Status: status.NewInternal(ctx, err, "error listing recycle bin"),
		}, nil
	}

	res := &storageproviderv1beta1pb.ListRecycleResponse{
		Status:       status.NewOK(ctx),
		RecycleItems: items,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *storageproviderv1beta1pb.RestoreRecycleItemRequest) (*storageproviderv1beta1pb.RestoreRecycleItemResponse, error) {
	// TODO(labkode): CRITICAL: fill recycle info with storage provider.
	if err := s.storage.RestoreRecycleItem(ctx, req.Key); err != nil {
		return &storageproviderv1beta1pb.RestoreRecycleItemResponse{
			Status: status.NewInternal(ctx, err, "error restoring recycle bin item"),
		}, nil
	}

	res := &storageproviderv1beta1pb.RestoreRecycleItemResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) PurgeRecycle(ctx context.Context, req *storageproviderv1beta1pb.PurgeRecycleRequest) (*storageproviderv1beta1pb.PurgeRecycleResponse, error) {
	// if a key was sent as opacque id purge only that item
	if req.GetRef().GetId() != nil && req.GetRef().GetId().GetOpaqueId() != "" {
		if err := s.storage.PurgeRecycleItem(ctx, req.GetRef().GetId().GetOpaqueId()); err != nil {
			return &storageproviderv1beta1pb.PurgeRecycleResponse{
				Status: status.NewInternal(ctx, err, "error purging recycle item"),
			}, nil
		}
	} else if err := s.storage.EmptyRecycle(ctx); err != nil {
		// otherwise try emptying the whole recycle bin
		return &storageproviderv1beta1pb.PurgeRecycleResponse{
			Status: status.NewInternal(ctx, err, "error emptying recycle bin"),
		}, nil
	}

	res := &storageproviderv1beta1pb.PurgeRecycleResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListGrants(ctx context.Context, req *storageproviderv1beta1pb.ListGrantsRequest) (*storageproviderv1beta1pb.ListGrantsResponse, error) {
	return nil, nil
}

func (s *service) AddGrant(ctx context.Context, req *storageproviderv1beta1pb.AddGrantRequest) (*storageproviderv1beta1pb.AddGrantResponse, error) {
	// check grantee type is valid
	if req.Grant.Grantee.Type == storageproviderv1beta1pb.GranteeType_GRANTEE_TYPE_INVALID {
		return &storageproviderv1beta1pb.AddGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.AddGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	err = s.storage.AddGrant(ctx, newRef, req.Grant)
	if err != nil {
		return &storageproviderv1beta1pb.AddGrantResponse{
			Status: status.NewInternal(ctx, err, "error setting ACL"),
		}, nil
	}

	res := &storageproviderv1beta1pb.AddGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) CreateReference(ctx context.Context, req *storageproviderv1beta1pb.CreateReferenceRequest) (*storageproviderv1beta1pb.CreateReferenceResponse, error) {
	log := appctx.GetLogger(ctx)

	// parse uri is valid
	u, err := url.Parse(req.TargetUri)
	if err != nil {
		log.Err(err).Msg("invalid target uri")
		return &storageproviderv1beta1pb.CreateReferenceResponse{
			Status: status.NewInvalidArg(ctx, "target uri is invalid: "+err.Error()),
		}, nil
	}

	if err := s.storage.CreateReference(ctx, req.Path, u); err != nil {
		log.Err(err).Msg("error calling CreateReference")
		return &storageproviderv1beta1pb.CreateReferenceResponse{
			Status: status.NewInternal(ctx, err, "error creating reference"),
		}, nil
	}

	return &storageproviderv1beta1pb.CreateReferenceResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) UpdateGrant(ctx context.Context, req *storageproviderv1beta1pb.UpdateGrantRequest) (*storageproviderv1beta1pb.UpdateGrantResponse, error) {
	// check grantee type is valid
	if req.Grant.Grantee.Type == storageproviderv1beta1pb.GranteeType_GRANTEE_TYPE_INVALID {
		return &storageproviderv1beta1pb.UpdateGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.UpdateGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.UpdateGrant(ctx, newRef, req.Grant); err != nil {
		return &storageproviderv1beta1pb.UpdateGrantResponse{
			Status: status.NewInternal(ctx, err, "error updating ACL"),
		}, nil
	}

	res := &storageproviderv1beta1pb.UpdateGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) RemoveGrant(ctx context.Context, req *storageproviderv1beta1pb.RemoveGrantRequest) (*storageproviderv1beta1pb.RemoveGrantResponse, error) {
	// check targetType is valid
	if req.Grant.Grantee.Type == storageproviderv1beta1pb.GranteeType_GRANTEE_TYPE_INVALID {
		return &storageproviderv1beta1pb.RemoveGrantResponse{
			Status: status.NewInvalid(ctx, "grantee type is invalid"),
		}, nil
	}

	newRef, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return &storageproviderv1beta1pb.RemoveGrantResponse{
			Status: status.NewInternal(ctx, err, "error unwrapping path"),
		}, nil
	}

	if err := s.storage.RemoveGrant(ctx, newRef, req.Grant); err != nil {
		return &storageproviderv1beta1pb.RemoveGrantResponse{
			Status: status.NewInternal(ctx, err, "error removing ACL"),
		}, nil
	}

	res := &storageproviderv1beta1pb.RemoveGrantResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetQuota(ctx context.Context, req *storageproviderv1beta1pb.GetQuotaRequest) (*storageproviderv1beta1pb.GetQuotaResponse, error) {
	total, used, err := s.storage.GetQuota(ctx)
	if err != nil {
		return &storageproviderv1beta1pb.GetQuotaResponse{
			Status: status.NewInternal(ctx, err, "error getting quota"),
		}, nil
	}

	res := &storageproviderv1beta1pb.GetQuotaResponse{
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

func getPW(c *config) (storage.PathWrapper, error) {
	if c.PathWrapper == "" {
		return nil, nil
	}
	if f, ok := pwregistry.NewFuncs[c.PathWrapper]; ok {
		return f(c.PathWrappers[c.PathWrapper])
	}
	return nil, fmt.Errorf("path wrapper not found: %s", c.Driver)
}

func (s *service) unwrap(ctx context.Context, ref *storageproviderv1beta1pb.Reference) (*storageproviderv1beta1pb.Reference, error) {
	if ref.GetId() != nil {
		idRef := &storageproviderv1beta1pb.Reference{
			Spec: &storageproviderv1beta1pb.Reference_Id{
				Id: &storageproviderv1beta1pb.ResourceId{
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

	if s.pathWrapper != nil {
		fsfn, err = s.pathWrapper.Unwrap(ctx, fsfn)
		if err != nil {
			return nil, err
		}
	}

	pathRef := &storageproviderv1beta1pb.Reference{
		Spec: &storageproviderv1beta1pb.Reference_Path{
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

func (s *service) wrap(ctx context.Context, ri *storageproviderv1beta1pb.ResourceInfo) error {
	ri.Id.StorageId = s.mountID

	if s.pathWrapper != nil {
		var err error
		ri.Path, err = s.pathWrapper.Wrap(ctx, ri.Path)
		if err != nil {
			return err
		}
	}

	ri.Path = path.Join(s.mountPath, ri.Path)
	return nil
}
