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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	storagetypespb "github.com/cernbox/go-cs3apis/cs3/storagetypes"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/utils"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/storage/fs/registry"
	"github.com/cernbox/reva/pkg/user"
	"google.golang.org/grpc"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"

	"context"

	"github.com/mitchellh/mapstructure"
)

var logger = log.New("storageprovidersvc")
var errors = err.New("storageprovidersvc")

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

func parseXSTypes(xsTypes map[string]uint32) ([]*storageproviderv0alphapb.ResourceChecksumPriority, error) {
	var types []*storageproviderv0alphapb.ResourceChecksumPriority
	for xs, prio := range xsTypes {
		t := PKG2GRPCXS(xs)
		if t == storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID {
			return nil, errors.Newf("checksum type is invalid: %s", xs)
		}
		xsPrio := &storageproviderv0alphapb.ResourceChecksumPriority{
			Priority: prio,
			Type:     t,
		}
		types = append(types, xsPrio)
	}
	return types, nil
}

func (s *service) isXSAvailable(t storageproviderv0alphapb.ResourceChecksumType) bool {
	for _, xsPrio := range s.availableXS {
		if xsPrio.Type == t {
			return true
		}
	}
	return false
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		logger.Error(context.Background(), errors.Wrap(err, "error decoding conf"))
		return nil, err
	}
	return c, nil
}

// New creates a new storage provider svc
func New(m map[string]interface{}, ss *grpc.Server) error {

	c, err := parseConfig(m)
	if err != nil {
		return err
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
		return err
	}

	// parse data server url
	u, err := url.Parse(c.DataServerURL)
	if err != nil {
		return err
	}

	// validate available checksums
	xsTypes, err := parseXSTypes(c.AvailableXS)
	if err != nil {
		return err
	}

	if len(xsTypes) == 0 {
		return errors.Newf("no available checksum, please set in config")
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
	return nil
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
	url := *s.dataServerURL
	url.Path = path.Join("/", url.Path, path.Clean(req.Ref.GetPath()))
	logger.Build().Str("data-server", url.String()).Str("fn", req.Ref.GetPath()).Msg(ctx, "file download")
	res := &storageproviderv0alphapb.InitiateFileDownloadResponse{
		DownloadEndpoint: url.String(),
		Status:           &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
	}
	return res, nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileUploadRequest) (*storageproviderv0alphapb.InitiateFileUploadResponse, error) {
	// TODO(labkode): same as download
	url := *s.dataServerURL
	url.Path = path.Join("/", url.Path, path.Clean(req.Ref.GetPath()))
	logger.Build().Str("data-server", url.String()).
		Str("fn", req.Ref.GetPath()).
		Str("xs", fmt.Sprintf("%+v", s.conf.AvailableXS)).
		Msg(ctx, "file upload")
	res := &storageproviderv0alphapb.InitiateFileUploadResponse{
		UploadEndpoint:     url.String(),
		Status:             &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		AvailableChecksums: s.availableXS,
	}
	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *storageproviderv0alphapb.GetPathRequest) (*storageproviderv0alphapb.GetPathResponse, error) {
	// TODO(labkode): check that the storage ID is the same as the storage provider id.
	fn, err := s.storage.GetPathByID(ctx, req.ResourceId.OpaqueId)
	if err != nil {
		logger.Error(ctx, err)
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
	fn := req.Ref.GetPath()
	fsfn, _, err := s.unwrap(ctx, fn)
	if err != nil {
		logger.Error(ctx, err)
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
		err = errors.Wrap(err, "error creating folder "+fn)
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.CreateContainerResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.CreateContainerResponse{Status: status}
	return res, nil
}

func (s *service) Delete(ctx context.Context, req *storageproviderv0alphapb.DeleteRequest) (*storageproviderv0alphapb.DeleteResponse, error) {
	fn := req.Ref.GetPath()

	fsfn, _, err := s.unwrap(ctx, fn)
	if err != nil {
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.DeleteResponse{Status: status}
		return res, nil
	}

	if err := s.storage.Delete(ctx, fsfn); err != nil {
		if _, ok := err.(notFoundError); ok {
			err := errors.Wrap(err, "file not found")
			logger.Error(ctx, err)
			status := &rpcpb.Status{Code: rpcpb.Code_CODE_NOT_FOUND}
			res := &storageproviderv0alphapb.DeleteResponse{Status: status}
			return res, nil
		}
		err := errors.Wrap(err, "error deleting file")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.DeleteResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.DeleteResponse{Status: status}
	return res, nil
}

func (s *service) Move(ctx context.Context, req *storageproviderv0alphapb.MoveRequest) (*storageproviderv0alphapb.MoveResponse, error) {
	source := req.Source.GetPath()
	target := req.Destination.GetPath()

	fss, _, err := s.unwrap(ctx, source)
	if err != nil {
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.MoveResponse{Status: status}
		return res, nil
	}
	fst, _, err := s.unwrap(ctx, target)
	if err != nil {
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.MoveResponse{Status: status}
		return res, nil
	}

	if err := s.storage.Move(ctx, fss, fst); err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error moving file")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.MoveResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.MoveResponse{Status: status}
	return res, nil
}

func (s *service) Stat(ctx context.Context, req *storageproviderv0alphapb.StatRequest) (*storageproviderv0alphapb.StatResponse, error) {
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
		err := errors.Wrap(err, "error stating file")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.StatResponse{Status: status}
		return res, nil
	}
	md.Path = s.wrap(ctx, md.Path, fctx)

	info := s.toInfo(md)
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.StatResponse{Status: status, Info: info}
	return res, nil
}

func (s *service) ListContainerStream(req *storageproviderv0alphapb.ListContainerStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListContainerStreamServer) error {
	ctx := ss.Context()
	fn := req.Ref.GetPath()

	fsfn, fctx, err := s.unwrap(ctx, fn)
	if err != nil {
		logger.Println(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerStreamResponse{Status: status}
		if err := ss.Send(res); err != nil {
			logger.Error(ctx, err)
			return err
		}
		return nil
	}

	mds, err := s.storage.ListFolder(ctx, fsfn)
	if err != nil {
		err := errors.Wrap(err, "error listing folder")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerStreamResponse{Status: status}
		if err := ss.Send(res); err != nil {
			logger.Error(ctx, err)
			return err
		}
		return nil
	}

	for _, md := range mds {
		md.Path = s.wrap(ctx, md.Path, fctx)
		info := s.toInfo(md)
		res := &storageproviderv0alphapb.ListContainerStreamResponse{
			Info: info,
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_OK,
			},
		}

		if err := ss.Send(res); err != nil {
			logger.Error(ctx, err)
			return err
		}
	}
	return nil
}

func (s *service) ListContainer(ctx context.Context, req *storageproviderv0alphapb.ListContainerRequest) (*storageproviderv0alphapb.ListContainerResponse, error) {
	fn := req.Ref.GetPath()

	fsfn, fctx, err := s.unwrap(ctx, fn)
	if err != nil {
		logger.Println(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerResponse{Status: status}
		return res, nil
	}

	mds, err := s.storage.ListFolder(ctx, fsfn)
	if err != nil {
		err := errors.Wrap(err, "error listing folder")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListContainerResponse{Status: status}
		return res, nil
	}

	var infos []*storageproviderv0alphapb.ResourceInfo
	for _, md := range mds {

		md.Path = s.wrap(ctx, md.Path, fctx)
		infos = append(infos, s.toInfo(md))
	}
	res := &storageproviderv0alphapb.ListContainerResponse{
		Status: &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Infos:  infos,
	}
	return res, nil
}

func (s *service) getSessionFolder(sessionID string) string {
	return filepath.Join(s.tmpFolder, sessionID)
}

func getResourceType(isDir bool) storageproviderv0alphapb.ResourceType {
	if isDir {
		return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE
}

func (s *service) ListFileVersions(ctx context.Context, req *storageproviderv0alphapb.ListFileVersionsRequest) (*storageproviderv0alphapb.ListFileVersionsResponse, error) {
	revs, err := s.storage.ListRevisions(ctx, req.Ref.GetPath())
	if err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error listing revisions")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListFileVersionsResponse{Status: status}
		return res, nil
	}

	var versions []*storageproviderv0alphapb.FileVersion
	for _, rev := range revs {
		versions = append(versions, &storageproviderv0alphapb.FileVersion{
			Key:   rev.RevKey,
			Mtime: rev.Mtime,
			Size:  rev.Size,
		})
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.ListFileVersionsResponse{Status: status, Versions: versions}
	return res, nil
}

func (s *service) RestoreFileVersion(ctx context.Context, req *storageproviderv0alphapb.RestoreFileVersionRequest) (*storageproviderv0alphapb.RestoreFileVersionResponse, error) {
	if err := s.storage.RestoreRevision(ctx, req.Ref.GetPath(), req.Key); err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error restoring version")
		logger.Error(ctx, err)
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
	items, err := s.storage.ListRecycle(ctx, "")
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error listing recycle")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListRecycleStreamResponse{Status: status}
		if err := ss.Send(res); err != nil {
			logger.Error(ctx, err)
			return err
		}
		return nil
	}

	for _, item := range items {
		recycleItem := &storageproviderv0alphapb.RecycleItem{
			Path:         item.RestorePath,
			Key:          item.RestoreKey,
			DeletionTime: utils.UnixNanoToTS(item.DelMtime),
			Type:         getResourceType(item.IsDir),
		}
		res := &storageproviderv0alphapb.ListRecycleStreamResponse{
			RecycleItem: recycleItem,
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_OK,
			},
		}
		if err := ss.Send(res); err != nil {
			logger.Error(ctx, err)
			return err
		}
	}
	return nil
}

func (s *service) ListRecycle(ctx context.Context, req *storageproviderv0alphapb.ListRecycleRequest) (*storageproviderv0alphapb.ListRecycleResponse, error) {
	items, err := s.storage.ListRecycle(ctx, "")
	if err != nil {
		err := errors.Wrap(err, "storageprovidersvc: error listing recycle")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.ListRecycleResponse{Status: status}
		return res, nil
	}

	var recycleItems []*storageproviderv0alphapb.RecycleItem
	for _, item := range items {
		recycleItems = append(recycleItems, &storageproviderv0alphapb.RecycleItem{
			Path:         item.RestorePath,
			Key:          item.RestoreKey,
			Size:         item.Size,
			DeletionTime: utils.UnixNanoToTS(item.DelMtime),
			Type:         getResourceType(item.IsDir),
		})
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.ListRecycleResponse{
		Status:       status,
		RecycleItems: recycleItems,
	}
	return res, nil
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *storageproviderv0alphapb.RestoreRecycleItemRequest) (*storageproviderv0alphapb.RestoreRecycleItemResponse, error) {
	if err := s.storage.RestoreRecycleItem(ctx, req.Ref.GetPath(), req.Key); err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error restoring recycle item")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.RestoreRecycleItemResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.RestoreRecycleItemResponse{Status: status}
	return res, nil
}

func (s *service) PurgeRecycle(ctx context.Context, req *storageproviderv0alphapb.PurgeRecycleRequest) (*storageproviderv0alphapb.PurgeRecycleResponse, error) {
	if err := s.storage.EmptyRecycle(ctx, req.Ref.GetPath()); err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error purging recycle")
		logger.Error(ctx, err)
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
	fn := req.Ref.GetPath()

	// check mode is valid
	if req.Grant.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID {
		logger.Println(ctx, "grantee type is invalid")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID_ARGUMENT, Message: "grantee type is invalid"}
		res := &storageproviderv0alphapb.AddGrantResponse{Status: status}
		return res, nil
	}

	userID := &user.ID{
		IDP:      req.Grant.Grantee.Id.Idp,
		OpaqueID: req.Grant.Grantee.Id.OpaqueId,
	}
	g := &storage.Grant{
		Grantee: &storage.Grantee{
			UserID: userID,
			Type:   s.getStorageGranteeType(req.Grant.Grantee.Type),
		},
		PermissionSet: s.getStoragePermissionSet(req.Grant.Permissions),
	}

	err := s.storage.AddGrant(ctx, fn, g)
	if err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error setting acl")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.AddGrantResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.AddGrantResponse{Status: status}
	return res, nil
}

func (s *service) getStorageGranteeType(t storageproviderv0alphapb.GranteeType) storage.GranteeType {
	switch t {
	case storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER:
		return storage.GranteeTypeUser
	case storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP:
		return storage.GranteeTypeGroup
	default:
		return storage.GranteeTypeInvalid
	}
}

func (s *service) getStoragePermissionSet(set *storageproviderv0alphapb.ResourcePermissions) *storage.PermissionSet {
	toret := &storage.PermissionSet{}
	if set.ListContainer {
		toret.ListContainer = true
	}
	if set.CreateContainer {
		toret.CreateContainer = true
	}
	return toret
}

func (s *service) UpdateGrant(ctx context.Context, req *storageproviderv0alphapb.UpdateGrantRequest) (*storageproviderv0alphapb.UpdateGrantResponse, error) {
	fn := req.Ref.GetPath()
	storagePerm := s.getStoragePermissionSet(req.Grant.Permissions)
	granteeType := s.getStorageGranteeType(req.Grant.Grantee.Type)

	if granteeType == storage.GranteeTypeInvalid {
		logger.Println(ctx, "grantee type is invalid")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID_ARGUMENT, Message: "grantee type is invalid"}
		res := &storageproviderv0alphapb.UpdateGrantResponse{Status: status}
		return res, nil
	}

	userID := &user.ID{
		OpaqueID: req.Grant.Grantee.Id.OpaqueId,
		IDP:      req.Grant.Grantee.Id.Idp,
	}
	g := &storage.Grant{
		Grantee: &storage.Grantee{
			UserID: userID,
			Type:   granteeType,
		},
		PermissionSet: storagePerm,
	}

	if err := s.storage.UpdateGrant(ctx, fn, g); err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error updating acl")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.UpdateGrantResponse{Status: status}
		return res, nil
	}
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.UpdateGrantResponse{Status: status}
	return res, nil
}

func (s *service) RemoveGrant(ctx context.Context, req *storageproviderv0alphapb.RemoveGrantRequest) (*storageproviderv0alphapb.RemoveGrantResponse, error) {
	fn := req.Ref.GetPath()
	granteeType := s.getStorageGranteeType(req.Grant.Grantee.Type)
	storagePerm := s.getStoragePermissionSet(req.Grant.Permissions)

	// check targetType is valid
	if granteeType == storage.GranteeTypeInvalid {
		logger.Println(ctx, "grantee type is invalid")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INVALID_ARGUMENT, Message: "grantee type is invalid"}
		res := &storageproviderv0alphapb.RemoveGrantResponse{Status: status}
		return res, nil
	}

	userID := &user.ID{
		OpaqueID: req.Grant.Grantee.Id.OpaqueId,
		IDP:      req.Grant.Grantee.Id.Idp,
	}
	g := &storage.Grant{
		Grantee: &storage.Grantee{
			UserID: userID,
			Type:   granteeType,
		},
		PermissionSet: storagePerm,
	}

	if err := s.storage.RemoveGrant(ctx, fn, g); err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error removing  grant")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL}
		res := &storageproviderv0alphapb.RemoveGrantResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &storageproviderv0alphapb.RemoveGrantResponse{Status: status}
	return res, nil
}

func (s *service) GetQuota(ctx context.Context, req *storageproviderv0alphapb.GetQuotaRequest) (*storageproviderv0alphapb.GetQuotaResponse, error) {
	total, used, err := s.storage.GetQuota(ctx)
	if err != nil {
		err = errors.Wrap(err, "storageprovidersvc: error getting quota")
		logger.Error(ctx, err)
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

// TODO(labkode): more fine grained control.
func toResourcePermissions(p *storage.PermissionSet) *storageproviderv0alphapb.ResourcePermissions {
	return &storageproviderv0alphapb.ResourcePermissions{
		ListContainer:   true,
		CreateContainer: true,
	}
}

func (s *service) toInfo(md *storage.MD) *storageproviderv0alphapb.ResourceInfo {
	perm := toResourcePermissions(md.Permissions)
	id := &storageproviderv0alphapb.ResourceId{
		StorageId: s.mountID,
		OpaqueId:  md.ID,
	}
	checksum := &storageproviderv0alphapb.ResourceChecksum{
		Type: storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5,
		Sum:  md.Checksum,
	}
	info := &storageproviderv0alphapb.ResourceInfo{
		Type:          getResourceType(md.IsDir),
		Id:            id,
		Path:          md.Path,
		Checksum:      checksum,
		Etag:          md.Etag,
		MimeType:      md.Mime,
		Mtime:         utils.UnixNanoToTS(md.Mtime),
		Size:          md.Size,
		PermissionSet: perm,
	}

	return info
}
