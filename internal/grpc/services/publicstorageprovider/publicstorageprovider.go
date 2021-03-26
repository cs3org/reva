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

package publicstorageprovider

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
)

func init() {
	rgrpc.Register("publicstorageprovider", New)
}

type config struct {
	MountPath   string `mapstructure:"mount_path"`
	GatewayAddr string `mapstructure:"gateway_addr"`
}

type service struct {
	conf      *config
	mountPath string
	gateway   gateway.GatewayAPIClient
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterProviderAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new Public Storage Provider service.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	mountPath := c.MountPath

	gateway, err := pool.GetGatewayServiceClient(c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:      c,
		mountPath: mountPath,
		gateway:   gateway,
	}

	return service, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	statReq := &provider.StatRequest{Ref: req.Ref}
	statRes, err := s.Stat(ctx, statReq)
	if err != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found"),
			}, nil
		}
		err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref"),
		}, nil
	}

	req.Opaque = statRes.Info.Opaque
	return s.initiateFileDownload(ctx, req)
}

func (s *service) translatePublicRefToCS3Ref(ctx context.Context, ref *provider.Reference) (*provider.Reference, string, *link.PublicShare, *rpc.Status, error) {
	log := appctx.GetLogger(ctx)
	tkn, relativePath, err := s.unwrap(ctx, ref)
	if err != nil {
		return nil, "", nil, nil, err
	}

	originalPath, ls, st, err := s.resolveToken(ctx, tkn)
	switch {
	case err != nil:
		return nil, "", nil, nil, err
	case st != nil:
		return nil, "", nil, st, nil
	}

	cs3Ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: path.Join("/", originalPath, relativePath)},
	}
	log.Debug().
		Interface("sourceRef", ref).
		Interface("cs3Ref", cs3Ref).
		Interface("share", ls).
		Str("tkn", tkn).
		Str("originalPath", originalPath).
		Str("relativePath", relativePath).
		Msg("translatePublicRefToCS3Ref")
	return cs3Ref, tkn, ls, nil, nil
}

// Both, t.dir and tokenPath paths need to be merged:
// tokenPath   = /oc/einstein/public-links
// t.dir       = /public/ausGxuUePCOi/foldera/folderb/
// res         = /public-links/foldera/folderb/
// this `res` will get then expanded taking into account the authenticated user and the storage:
// end         = /einstein/files/public-links/foldera/folderb/

func (s *service) initiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	cs3Ref, _, ls, st, err := s.translatePublicRefToCS3Ref(ctx, req.Ref)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.InitiateFileDownloadResponse{
			Status: st,
		}, nil
	case ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.InitiateFileDownload:
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant InitiateFileDownload permission"),
		}, nil
	}
	dReq := &provider.InitiateFileDownloadRequest{
		Ref: cs3Ref,
	}

	dRes, err := s.gateway.InitiateFileDownload(ctx, dReq)
	if err != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling InitiateFileDownload"),
		}, nil
	}

	if dRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.InitiateFileDownloadResponse{
			Status: dRes.Status,
		}, nil
	}

	protocols := make([]*provider.FileDownloadProtocol, len(dRes.Protocols))
	for p := range dRes.Protocols {
		if !strings.HasSuffix(dRes.Protocols[p].DownloadEndpoint, "/") {
			dRes.Protocols[p].DownloadEndpoint += "/"
		}
		dRes.Protocols[p].DownloadEndpoint += dRes.Protocols[p].Token

		protocols = append(protocols, &provider.FileDownloadProtocol{
			Opaque:           dRes.Protocols[p].Opaque,
			Protocol:         dRes.Protocols[p].Protocol,
			DownloadEndpoint: dRes.Protocols[p].DownloadEndpoint,
			Expose:           true, // the gateway already has encoded the upload endpoint
		})
	}

	return &provider.InitiateFileDownloadResponse{
		Status:    dRes.Status,
		Protocols: protocols,
	}, nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	cs3Ref, _, ls, st, err := s.translatePublicRefToCS3Ref(ctx, req.Ref)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.InitiateFileUploadResponse{
			Status: st,
		}, nil
	case ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.InitiateFileUpload:
		return &provider.InitiateFileUploadResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant InitiateFileUpload permission"),
		}, nil
	}
	uReq := &provider.InitiateFileUploadRequest{
		Ref:    cs3Ref,
		Opaque: req.Opaque,
	}

	uRes, err := s.gateway.InitiateFileUpload(ctx, uReq)
	if err != nil {
		return &provider.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling InitiateFileUpload"),
		}, nil
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.InitiateFileUploadResponse{
			Status: uRes.Status,
		}, nil
	}

	protocols := make([]*provider.FileUploadProtocol, len(uRes.Protocols))
	for p := range uRes.Protocols {
		if !strings.HasSuffix(uRes.Protocols[p].UploadEndpoint, "/") {
			uRes.Protocols[p].UploadEndpoint += "/"
		}
		uRes.Protocols[p].UploadEndpoint += uRes.Protocols[p].Token

		protocols = append(protocols, &provider.FileUploadProtocol{
			Opaque:             uRes.Protocols[p].Opaque,
			Protocol:           uRes.Protocols[p].Protocol,
			UploadEndpoint:     uRes.Protocols[p].UploadEndpoint,
			AvailableChecksums: uRes.Protocols[p].AvailableChecksums,
			Expose:             true, // the gateway already has encoded the upload endpoint
		})
	}

	res := &provider.InitiateFileUploadResponse{
		Status:    uRes.Status,
		Protocols: protocols,
	}

	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	ctx, span := trace.StartSpan(ctx, "CreateContainer")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	cs3Ref, _, ls, st, err := s.translatePublicRefToCS3Ref(ctx, req.Ref)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.CreateContainerResponse{
			Status: st,
		}, nil
	case ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.CreateContainer:
		return &provider.CreateContainerResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant CreateContainer permission"),
		}, nil
	}

	var res *provider.CreateContainerResponse
	// the call has to be made to the gateway instead of the storage.
	res, err = s.gateway.CreateContainer(ctx, &provider.CreateContainerRequest{
		Ref: cs3Ref,
	})
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling CreateContainer for ref:"+req.Ref.String()),
		}, nil
	}
	if res.Status.Code == rpc.Code_CODE_INTERNAL {
		return res, nil
	}

	return res, nil
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	ctx, span := trace.StartSpan(ctx, "Delete")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	cs3Ref, _, ls, st, err := s.translatePublicRefToCS3Ref(ctx, req.Ref)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.DeleteResponse{
			Status: st,
		}, nil
	case ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.Delete:
		return &provider.DeleteResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant Delete permission"),
		}, nil
	}

	var res *provider.DeleteResponse
	// the call has to be made to the gateway instead of the storage.
	res, err = s.gateway.Delete(ctx, &provider.DeleteRequest{
		Ref: cs3Ref,
	})
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling Delete for ref:"+req.Ref.String()),
		}, nil
	}
	if res.Status.Code == rpc.Code_CODE_INTERNAL {
		return res, nil
	}

	return res, nil
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	ctx, span := trace.StartSpan(ctx, "Move")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("source", req.Source.String()),
		trace.StringAttribute("destination", req.Destination.String()),
	)

	cs3RefSource, tknSource, ls, st, err := s.translatePublicRefToCS3Ref(ctx, req.Source)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.MoveResponse{
			Status: st,
		}, nil
	case ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.Move:
		return &provider.MoveResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant Move permission"),
		}, nil
	}
	// FIXME: maybe there's a shortcut possible here using the source path
	cs3RefDestination, tknDest, _, st, err := s.translatePublicRefToCS3Ref(ctx, req.Destination)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	if tknSource != tknDest {
		return &provider.MoveResponse{
			Status: status.NewInvalidArg(ctx, "Source and destination token must be the same"),
		}, nil
	}

	var res *provider.MoveResponse
	// the call has to be made to the gateway instead of the storage.
	res, err = s.gateway.Move(ctx, &provider.MoveRequest{
		Source:      cs3RefSource,
		Destination: cs3RefDestination,
	})
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling Move for source ref "+req.Source.String()+" to destination ref "+req.Destination.String()),
		}, nil
	}
	if res.Status.Code == rpc.Code_CODE_INTERNAL {
		return res, nil
	}

	return res, nil
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	ctx, span := trace.StartSpan(ctx, "Stat")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	tkn, relativePath, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	originalPath, ls, st, err := s.resolveToken(ctx, tkn)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.StatResponse{
			Status: st,
		}, nil
	case ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.Stat:
		return &provider.StatResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant Stat permission"),
		}, nil
	}

	var statResponse *provider.StatResponse
	// the call has to be made to the gateway instead of the storage.
	statResponse, err = s.gateway.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join("/", originalPath, relativePath),
			},
		},
	})
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling Stat for ref:"+req.Ref.String()),
		}, nil
	}

	// prevent leaking internal paths
	if statResponse.Info != nil {
		if err := addShare(statResponse.Info, ls); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("share", ls).Interface("info", statResponse.Info).Msg("error when adding share")
		}
		statResponse.Info.Path = path.Join(s.mountPath, "/", tkn, relativePath)
		filterPermissions(statResponse.Info.PermissionSet, ls.GetPermissions().Permissions)
	}

	return statResponse, nil
}

func addShare(i *provider.ResourceInfo, ls *link.PublicShare) error {
	if i.Opaque == nil {
		i.Opaque = &typesv1beta1.Opaque{}
	}
	if i.Opaque.Map == nil {
		i.Opaque.Map = map[string]*typesv1beta1.OpaqueEntry{}
	}
	val, err := json.Marshal(ls)
	if err != nil {
		return err
	}
	i.Opaque.Map["link-share"] = &typesv1beta1.OpaqueEntry{Decoder: "json", Value: val}
	return nil
}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	tkn, relativePath, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	pathFromToken, ls, st, err := s.resolveToken(ctx, tkn)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}
	if ls.GetPermissions() == nil || !ls.GetPermissions().Permissions.ListContainer {
		return &provider.ListContainerResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant ListContainer permission"),
		}, nil
	}

	listContainerR, err := s.gateway.ListContainer(
		ctx,
		&provider.ListContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: path.Join("/", pathFromToken, relativePath),
				},
			},
		},
	)
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling ListContainer for ref:"+req.Ref.String()),
		}, nil
	}

	for i := range listContainerR.Infos {
		filterPermissions(listContainerR.Infos[i].PermissionSet, ls.GetPermissions().Permissions)
		listContainerR.Infos[i].Path = path.Join(s.mountPath, "/", tkn, relativePath, path.Base(listContainerR.Infos[i].Path))
		if err := addShare(listContainerR.Infos[i], ls); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("share", ls).Interface("info", listContainerR.Infos[i]).Msg("error when adding share")
		}
	}

	return listContainerR, nil
}

func filterPermissions(l *provider.ResourcePermissions, r *provider.ResourcePermissions) {
	l.AddGrant = l.AddGrant && r.AddGrant
	l.CreateContainer = l.CreateContainer && r.CreateContainer
	l.Delete = l.Delete && r.Delete
	l.GetPath = l.GetPath && r.GetPath
	l.GetQuota = l.GetQuota && r.GetQuota
	l.InitiateFileDownload = l.InitiateFileDownload && r.InitiateFileDownload
	l.InitiateFileUpload = l.InitiateFileUpload && r.InitiateFileUpload
	l.ListContainer = l.ListContainer && r.ListContainer
	l.ListFileVersions = l.ListFileVersions && r.ListFileVersions
	l.ListGrants = l.ListGrants && r.ListGrants
	l.ListRecycle = l.ListRecycle && r.ListRecycle
	l.Move = l.Move && r.Move
	l.PurgeRecycle = l.PurgeRecycle && r.PurgeRecycle
	l.RemoveGrant = l.RemoveGrant && r.RemoveGrant
	l.RestoreFileVersion = l.RestoreFileVersion && r.RestoreFileVersion
	l.RestoreRecycleItem = l.RestoreRecycleItem && r.RestoreRecycleItem
	l.Stat = l.Stat && r.Stat
	l.UpdateGrant = l.UpdateGrant && r.UpdateGrant
}

func (s *service) unwrap(ctx context.Context, ref *provider.Reference) (token string, relativePath string, err error) {
	if ref.GetId() != nil {
		return "", "", errors.New("need path based ref: got " + ref.String())
	}

	if ref.GetPath() == "" {
		// abort, no valid id nor path
		return "", "", errors.New("invalid ref: " + ref.String())
	}

	// i.e path: /public/{token}/path/to/subfolders
	fn := ref.GetPath()
	// fsfn: /{token}/path/to/subfolders
	fsfn, err := s.trimMountPrefix(fn)
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(fsfn, "/", 3)
	token = parts[1]
	if len(parts) > 2 {
		relativePath = parts[2]
	}

	return
}

func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycleStream(req *provider.ListRecycleStreamRequest, ss provider.ProviderAPI_ListRecycleStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListGrants(ctx context.Context, req *provider.ListGrantsRequest) (*provider.ListGrantsResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) AddGrant(ctx context.Context, req *provider.AddGrantRequest) (*provider.AddGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateReference(ctx context.Context, req *provider.CreateReferenceRequest) (*provider.CreateReferenceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateGrant(ctx context.Context, req *provider.UpdateGrantRequest) (*provider.UpdateGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RemoveGrant(ctx context.Context, req *provider.RemoveGrantRequest) (*provider.RemoveGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) trimMountPrefix(fn string) (string, error) {
	if strings.HasPrefix(fn, s.mountPath) {
		return path.Join("/", strings.TrimPrefix(fn, s.mountPath)), nil
	}
	return "", errors.Errorf("path=%q does not belong to this storage provider mount path=%q"+fn, s.mountPath)
}

// resolveToken returns the path and share for the publicly shared resource.
func (s *service) resolveToken(ctx context.Context, token string) (string, *link.PublicShare, *rpc.Status, error) {
	driver, err := pool.GetGatewayServiceClient(s.conf.GatewayAddr)
	if err != nil {
		return "", nil, nil, err
	}

	publicShareResponse, err := driver.GetPublicShare(
		ctx,
		&link.GetPublicShareRequest{
			Ref: &link.PublicShareReference{
				Spec: &link.PublicShareReference_Token{
					Token: token,
				},
			},
			Sign: true,
		},
	)
	switch {
	case err != nil:
		return "", nil, nil, err
	case publicShareResponse.Status.Code != rpc.Code_CODE_OK:
		return "", nil, publicShareResponse.Status, nil
	}

	pathRes, err := s.gateway.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: publicShareResponse.GetShare().GetResourceId(),
	})
	switch {
	case err != nil:
		return "", nil, nil, err
	case pathRes.Status.Code != rpc.Code_CODE_OK:
		return "", nil, pathRes.Status, nil
	}
	return pathRes.Path, publicShareResponse.GetShare(), nil, nil
}
