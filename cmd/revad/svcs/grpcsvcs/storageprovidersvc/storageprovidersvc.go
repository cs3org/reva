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

package storageprovidersvc

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"

	"github.com/cs3org/reva/cmd/revad/grpcserver"

	"context"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.Register("storageprovidersvc", New)
}

type config struct {
	Driver        string                            `mapstructure:"driver"`
	MountPath     string                            `mapstructure:"mount_path"`
	MountID       string                            `mapstructure:"mount_id"`
	TmpFolder     string                            `mapstructure:"tmp_folder"`
	Drivers       map[string]map[string]interface{} `mapstructure:"drivers"`
	DataServerURL string                            `mapstructure:"data_server_url"`
	AvailableXS   map[string]uint32                 `mapstructure:"available_checksums"`
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
	var types []*storageproviderv0alphapb.ResourceChecksumPriority
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

func (s *service) GetProvider(ctx context.Context, req *storageproviderv0alphapb.GetProviderRequest) (*storageproviderv0alphapb.GetProviderResponse, error) {
	provider := &storagetypespb.ProviderInfo{
		// Address:  ? TODO(labkode): how to obtain the addresss the service is listening to? not very useful if the request already comes to it :(
		ProviderId:   s.mountID,
		ProviderPath: s.mountPath,
		// Description:  s.description, TODO(labkode): add to config
		// Features: ? TODO(labkode):
	}
	res := &storageproviderv0alphapb.GetProviderResponse{
		Info: provider,
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_OK,
		},
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
		Status:           &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
	}
	return res, nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileUploadRequest) (*storageproviderv0alphapb.InitiateFileUploadResponse, error) {
	// TODO(labkode): same as download
	log := appctx.GetLogger(ctx)
	url := *s.dataServerURL
	url.Path = path.Join("/", url.Path, path.Clean(req.Ref.GetPath()))
	log.Info().Str("data-server", url.String()).
		Str("fn", req.Ref.GetPath()).
		Str("xs", fmt.Sprintf("%+v", s.conf.AvailableXS)).
		Msg("file upload")
	res := &storageproviderv0alphapb.InitiateFileUploadResponse{
		UploadEndpoint:     url.String(),
		Status:             &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		AvailableChecksums: s.availableXS,
	}
	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *storageproviderv0alphapb.GetPathRequest) (*storageproviderv0alphapb.GetPathResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO(labkode): check that the storage ID is the same as the storage provider id.
	fn, err := s.storage.GetPathByID(ctx, req.ResourceId.OpaqueId)
	if err != nil {
		log.Error().Err(err).Msg("error getting path by id")
		res := &storageproviderv0alphapb.GetPathResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}
		return res, nil
	}

	fn = path.Join(s.mountPath, path.Clean(fn))
	res := &storageproviderv0alphapb.GetPathResponse{
		Path: fn,
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_OK,
		},
	}
	return res, nil
}

func (s *service) CreateContainer(ctx context.Context, req *storageproviderv0alphapb.CreateContainerRequest) (*storageproviderv0alphapb.CreateContainerResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()
	fsfn, _, err := s.unwrap(ctx, fn)
	if err != nil {
		log.Error().Err(err).Msg("error unwraping path")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID}
		res := &storageproviderv0alphapb.CreateContainerResponse{Status: status}
		return res, nil
	}

	if err := s.storage.CreateDir(ctx, fsfn); err != nil {
		if _, ok := err.(notFoundError); ok {
			status := &rpcpb.Status{Code: rpcpb.Code_CODE_NOT_FOUND}
			res := &storageproviderv0alphapb.CreateContainerResponse{Status: status}
			return res, nil
		}
		log.Error().Err(err).Msg("error creating folder " + fn)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.CreateContainerResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.CreateContainerResponse{Status: status}
	return res, nil
}

func (s *service) Delete(ctx context.Context, req *storageproviderv0alphapb.DeleteRequest) (*storageproviderv0alphapb.DeleteResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	fsfn, _, err := s.unwrap(ctx, fn)
	if err != nil {
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.DeleteResponse{Status: status}
		return res, nil
	}

	if err := s.storage.Delete(ctx, fsfn); err != nil {
		if _, ok := err.(notFoundError); ok {
			log.Error().Err(err).Msg("file not found")
			status := &rpcpb.Status{Code: rpcpb.Code_CODE_NOT_FOUND}
			res := &storageproviderv0alphapb.DeleteResponse{Status: status}
			return res, nil
		}
		log.Error().Err(err).Msg("error deleting file")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.DeleteResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.DeleteResponse{Status: status}
	return res, nil
}

func (s *service) Move(ctx context.Context, req *storageproviderv0alphapb.MoveRequest) (*storageproviderv0alphapb.MoveResponse, error) {
	log := appctx.GetLogger(ctx)
	source := req.Source.GetPath()
	target := req.Destination.GetPath()

	fss, _, err := s.unwrap(ctx, source)
	if err != nil {
		log.Error().Err(err).Msg("error unwraping path")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.MoveResponse{Status: status}
		return res, nil
	}
	fst, _, err := s.unwrap(ctx, target)
	if err != nil {
		log.Error().Err(err).Msg("error unwraping path")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.MoveResponse{Status: status}
		return res, nil
	}

	if err := s.storage.Move(ctx, fss, fst); err != nil {
		log.Error().Err(err).Msg("error moving file")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.MoveResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.MoveResponse{Status: status}
	return res, nil
}

func (s *service) Stat(ctx context.Context, req *storageproviderv0alphapb.StatRequest) (*storageproviderv0alphapb.StatResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	fsfn, fctx, err := s.unwrap(ctx, fn)
	if err != nil {
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID}
		res := &storageproviderv0alphapb.StatResponse{Status: status}
		return res, nil
	}

	md, err := s.storage.GetMD(ctx, fsfn)
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			status := &rpcpb.Status{Code: rpcpb.Code_CODE_NOT_FOUND}
			res := &storageproviderv0alphapb.StatResponse{Status: status}
			return res, nil
		}
		log.Error().Err(err).Msg("error stating file")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.StatResponse{Status: status}
		return res, nil
	}
	md.Path = s.wrap(ctx, md.Path, fctx)

	s.fillInfo(md)
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.StatResponse{Status: status, Info: md}
	return res, nil
}

func (s *service) ListContainerStream(req *storageproviderv0alphapb.ListContainerStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListContainerStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	fsfn, fctx, err := s.unwrap(ctx, fn)
	if err != nil {
		log.Error().Err(err).Msg("error unwraping path")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerStreamResponse{Status: status}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("error sending response")
			return err
		}
		return nil
	}

	mds, err := s.storage.ListFolder(ctx, fsfn)
	if err != nil {
		log.Error().Err(err).Msg("error listing folder")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerStreamResponse{Status: status}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("error sending response")
			return err
		}
		return nil
	}

	for _, md := range mds {
		md.Path = s.wrap(ctx, md.Path, fctx)
		s.fillInfo(md)
		res := &storageproviderv0alphapb.ListContainerStreamResponse{
			Info: md,
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_OK,
			},
		}

		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("error sending response")
			return err
		}
	}
	return nil
}

func (s *service) ListContainer(ctx context.Context, req *storageproviderv0alphapb.ListContainerRequest) (*storageproviderv0alphapb.ListContainerResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	fsfn, fctx, err := s.unwrap(ctx, fn)
	if err != nil {
		log.Error().Err(err).Msg("error unwraping path")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerResponse{Status: status}
		return res, nil
	}

	mds, err := s.storage.ListFolder(ctx, fsfn)
	if err != nil {
		log.Error().Err(err).Msg("error listing folder")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerResponse{Status: status}
		return res, nil
	}

	var infos []*storageproviderv0alphapb.ResourceInfo
	for _, md := range mds {

		md.Path = s.wrap(ctx, md.Path, fctx)
		s.fillInfo(md)
		infos = append(infos, md)
	}
	res := &storageproviderv0alphapb.ListContainerResponse{
		Status: &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Infos:  infos,
	}
	return res, nil
}

func getResourceType(isDir bool) storageproviderv0alphapb.ResourceType {
	if isDir {
		return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE
}

func (s *service) ListFileVersions(ctx context.Context, req *storageproviderv0alphapb.ListFileVersionsRequest) (*storageproviderv0alphapb.ListFileVersionsResponse, error) {
	log := appctx.GetLogger(ctx)
	revs, err := s.storage.ListRevisions(ctx, req.Ref.GetPath())
	if err != nil {
		log.Error().Err(err).Msg("error listing file versions")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListFileVersionsResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.ListFileVersionsResponse{Status: status, Versions: revs}
	return res, nil
}

func (s *service) RestoreFileVersion(ctx context.Context, req *storageproviderv0alphapb.RestoreFileVersionRequest) (*storageproviderv0alphapb.RestoreFileVersionResponse, error) {
	log := appctx.GetLogger(ctx)
	if err := s.storage.RestoreRevision(ctx, req.Ref.GetPath(), req.Key); err != nil {
		log.Error().Err(err).Msg("error restoring version")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.RestoreFileVersionResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
	res := &storageproviderv0alphapb.RestoreFileVersionResponse{Status: status}
	return res, nil
}

func (s *service) ListRecycleStream(req *storageproviderv0alphapb.ListRecycleStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListRecycleStreamServer) error {
	ctx := ss.Context()
	log := appctx.GetLogger(ctx)
	items, err := s.storage.ListRecycle(ctx, "")
	if err != nil {
		log.Error().Err(err).Msg("error listing recycle")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListRecycleStreamResponse{Status: status}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("error sending response")
			return err
		}
		return nil
	}

	for _, item := range items {

		res := &storageproviderv0alphapb.ListRecycleStreamResponse{
			RecycleItem: item,
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_OK,
			},
		}
		if err := ss.Send(res); err != nil {
			log.Error().Err(err).Msg("error sending response")
			return err
		}
	}
	return nil
}

func (s *service) ListRecycle(ctx context.Context, req *storageproviderv0alphapb.ListRecycleRequest) (*storageproviderv0alphapb.ListRecycleResponse, error) {
	log := appctx.GetLogger(ctx)
	items, err := s.storage.ListRecycle(ctx, "")
	if err != nil {
		log.Error().Err(err).Msg("error listing recycle")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListRecycleResponse{Status: status}
		return res, nil
	}

	var recycleItems []*storageproviderv0alphapb.RecycleItem
	for _, item := range items {
		recycleItems = append(recycleItems, item)
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.ListRecycleResponse{
		Status:       status,
		RecycleItems: recycleItems,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *storageproviderv0alphapb.RestoreRecycleItemRequest) (*storageproviderv0alphapb.RestoreRecycleItemResponse, error) {
	log := appctx.GetLogger(ctx)
	if err := s.storage.RestoreRecycleItem(ctx, req.Ref.GetPath(), req.Key); err != nil {
		log.Error().Err(err).Msg("error restoring recycle item")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.RestoreRecycleItemResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.RestoreRecycleItemResponse{Status: status}
	return res, nil
}

func (s *service) PurgeRecycle(ctx context.Context, req *storageproviderv0alphapb.PurgeRecycleRequest) (*storageproviderv0alphapb.PurgeRecycleResponse, error) {
	log := appctx.GetLogger(ctx)
	if err := s.storage.EmptyRecycle(ctx, req.Ref.GetPath()); err != nil {
		log.Error().Err(err).Msg("error purging recycle")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.PurgeRecycleResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.PurgeRecycleResponse{Status: status}
	return res, nil
}

func (s *service) ListGrants(ctx context.Context, req *storageproviderv0alphapb.ListGrantsRequest) (*storageproviderv0alphapb.ListGrantsResponse, error) {
	return nil, nil
}

func (s *service) AddGrant(ctx context.Context, req *storageproviderv0alphapb.AddGrantRequest) (*storageproviderv0alphapb.AddGrantResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	// check mode is valid
	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		log.Warn().Msg("grantee type is invalid")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID_ARGUMENT, Message: "grantee type is invalid"}
		res := &storageproviderv0alphapb.AddGrantResponse{Status: status}
		return res, nil
	}

	err := s.storage.AddGrant(ctx, fn, req.Grant)
	if err != nil {
		log.Error().Err(err).Msg("error setting acl")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.AddGrantResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.AddGrantResponse{Status: status}
	return res, nil
}

func (s *service) UpdateGrant(ctx context.Context, req *storageproviderv0alphapb.UpdateGrantRequest) (*storageproviderv0alphapb.UpdateGrantResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		log.Warn().Msg("grantee type is invalid")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID_ARGUMENT, Message: "grantee type is invalid"}
		res := &storageproviderv0alphapb.UpdateGrantResponse{Status: status}
		return res, nil
	}

	if err := s.storage.UpdateGrant(ctx, fn, req.Grant); err != nil {
		log.Error().Err(err).Msg("error updating acl")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.UpdateGrantResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.UpdateGrantResponse{Status: status}
	return res, nil
}

func (s *service) RemoveGrant(ctx context.Context, req *storageproviderv0alphapb.RemoveGrantRequest) (*storageproviderv0alphapb.RemoveGrantResponse, error) {
	log := appctx.GetLogger(ctx)
	fn := req.Ref.GetPath()

	// check targetType is valid
	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		log.Warn().Msg("grantee type is invalid")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID_ARGUMENT, Message: "grantee type is invalid"}
		res := &storageproviderv0alphapb.RemoveGrantResponse{Status: status}
		return res, nil
	}

	if err := s.storage.RemoveGrant(ctx, fn, req.Grant); err != nil {
		log.Error().Err(err).Msg("error removing grant")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.RemoveGrantResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.RemoveGrantResponse{Status: status}
	return res, nil
}

func (s *service) GetQuota(ctx context.Context, req *storageproviderv0alphapb.GetQuotaRequest) (*storageproviderv0alphapb.GetQuotaResponse, error) {
	log := appctx.GetLogger(ctx)
	total, used, err := s.storage.GetQuota(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error gettign quota")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.GetQuotaResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.GetQuotaResponse{Status: status, TotalBytes: uint64(total), UsedBytes: uint64(used)}
	return res, nil
}

func (s *service) splitFn(fsfn string) (string, string, error) {
	tokens := strings.Split(fsfn, "/")
	l := len(tokens)
	if l == 0 {
		return "", "", errors.New("fsfn is not id-based")
	}

	fid := tokens[0]
	if l > 1 {
		return fid, path.Join(tokens[1:]...), nil
	}
	return fid, "", nil
}

type fnCtx struct {
	mountPrefix string
	*derefCtx
}

type derefCtx struct {
	derefPath string
	fid       string
	rootFidFn string
}

func (s *service) deref(ctx context.Context, fsfn string) (*derefCtx, error) {
	if strings.HasPrefix(fsfn, "/") {
		return &derefCtx{derefPath: fsfn}, nil
	}

	fid, right, err := s.splitFn(fsfn)
	if err != nil {
		return nil, err
	}
	// resolve fid to path in the fs
	fnPointByID, err := s.storage.GetPathByID(ctx, fid)
	if err != nil {
		return nil, err
	}

	derefPath := path.Join(fnPointByID, right)
	return &derefCtx{derefPath: derefPath, fid: fid, rootFidFn: fnPointByID}, nil
}

func (s *service) unwrap(ctx context.Context, fn string) (string, *fnCtx, error) {
	mp, fsfn, err := s.trimMounPrefix(fn)
	if err != nil {
		return "", nil, err
	}

	derefCtx, err := s.deref(ctx, fsfn)
	if err != nil {
		return "", nil, err
	}

	fctx := &fnCtx{
		derefCtx:    derefCtx,
		mountPrefix: mp,
	}
	return fsfn, fctx, nil
}

func (s *service) wrap(ctx context.Context, fsfn string, fctx *fnCtx) string {
	if !strings.HasPrefix(fsfn, "/") {
		fsfn = strings.TrimPrefix(fsfn, fctx.rootFidFn)
		fsfn = path.Join(fctx.fid, fsfn)
		fsfn = fctx.mountPrefix + ":" + fsfn
	} else {
		fsfn = path.Join(fctx.mountPrefix, fsfn)
	}

	return fsfn
}

func (s *service) trimMounPrefix(fn string) (string, string, error) {
	mountID := s.mountID + ":"
	if strings.HasPrefix(fn, s.mountPath) {
		return s.mountPath, path.Join("/", strings.TrimPrefix(fn, s.mountPath)), nil
	}
	if strings.HasPrefix(fn, mountID) {
		return mountID, strings.TrimPrefix(fn, mountID), nil
	}
	return "", "", errors.New("fn does not belong to this storage provider: " + fn)
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

type notFoundError interface {
	IsNotFound()
}

func (s *service) fillInfo(ri *storageproviderv0alphapb.ResourceInfo) {
	ri.Id.StorageId = s.mountID
}
