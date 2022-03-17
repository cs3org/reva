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
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	rtrace "github.com/cs3org/reva/v2/pkg/trace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
)

// SpaceTypePublic is the public space type
var SpaceTypePublic = "public"

func init() {
	rgrpc.Register("publicstorageprovider", New)
}

type config struct {
	GatewayAddr string `mapstructure:"gateway_addr"`
}

type service struct {
	conf    *config
	gateway gateway.GatewayAPIClient
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

// New creates a new IsPublic Storage Provider service.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	gateway, err := pool.GetGatewayServiceClient(c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:    c,
		gateway: gateway,
	}

	return service, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	ref, _, _, st, err := s.translatePublicRefToCS3Ref(ctx, req.Ref)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.SetArbitraryMetadataResponse{
			Status: st,
		}, nil
	}
	return s.gateway.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{Opaque: req.Opaque, Ref: ref, ArbitraryMetadata: req.ArbitraryMetadata})
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// SetLock puts a lock on the given reference
func (s *service) SetLock(ctx context.Context, req *provider.SetLockRequest) (*provider.SetLockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// GetLock returns an existing lock on the given reference
func (s *service) GetLock(ctx context.Context, req *provider.GetLockRequest) (*provider.GetLockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// RefreshLock refreshes an existing lock on the given reference
func (s *service) RefreshLock(ctx context.Context, req *provider.RefreshLockRequest) (*provider.RefreshLockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// Unlock removes an existing lock from the given reference
func (s *service) Unlock(ctx context.Context, req *provider.UnlockRequest) (*provider.UnlockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	statReq := &provider.StatRequest{Ref: req.Ref}
	statRes, err := s.Stat(ctx, statReq)
	if err != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, "InitiateFileDownload: error stating ref:"+req.Ref.String()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "InitiateFileDownload: file not found"),
			}, nil
		}
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, "InitiateFileDownload: error stating ref"),
		}, nil
	}

	req.Opaque = statRes.Info.Opaque
	return s.initiateFileDownload(ctx, req)
}

func (s *service) translatePublicRefToCS3Ref(ctx context.Context, ref *provider.Reference) (*provider.Reference, string, *link.PublicShare, *rpc.Status, error) {
	log := appctx.GetLogger(ctx)

	share, _, ok := extractLinkAndInfo(ctx)
	if !ok {
		return nil, "", nil, nil, gstatus.Errorf(codes.NotFound, "share or token not found")
	}
	/*
		tkn, opaqueid, relativePath, err := s.unwrap(ctx, ref)
		if err != nil {
			return nil, "", nil, nil, err
		}
	*/

	ls, shareInfo, st, err := s.resolveToken(ctx, share.Token)
	switch {
	case err != nil:
		return nil, "", nil, nil, err
	case st != nil:
		return nil, "", nil, st, nil
	}

	var path string
	switch shareInfo.Type {
	case provider.ResourceType_RESOURCE_TYPE_CONTAINER:
		// folders point to the folder -> path needs to be added
		path = utils.MakeRelativePath(ref.Path)
	case provider.ResourceType_RESOURCE_TYPE_FILE:
		// files already point to the correct id
		path = "."
	default:
		// TODO: can this happen?
		// path = utils.MakeRelativePath(relativePath)
	}

	cs3Ref := &provider.Reference{
		ResourceId: shareInfo.Id,
		Path:       path,
	}

	log.Debug().
		Interface("sourceRef", ref).
		Interface("cs3Ref", cs3Ref).
		Interface("share", ls).
		Str("tkn", share.Token).
		Str("originalPath", shareInfo.Path).
		Str("relativePath", path).
		Msg("translatePublicRefToCS3Ref")
	return cs3Ref, share.Token, ls, nil, nil
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
			Status: status.NewInternal(ctx, "initiateFileDownload: error calling InitiateFileDownload"),
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
			Status: status.NewInternal(ctx, "InitiateFileUpload: error calling InitiateFileUpload"),
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

// ListStorageSpaces returns a Storage spaces of type "public" when given a filter by id with  the public link token as spaceid.
// The root node of every storag space is the real (spaceid, nodeid) of the publicly shared node
// The ocdav service has to
// 1. Authenticate / Log in at the gateway using the token and can then
// 2. look up the storage space using ListStorageSpaces.
// 3. make related requests to that (spaceid, nodeid)
func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	spaceTypes := map[string]struct{}{}
	var exists = struct{}{}
	appendTypes := []string{}
	var spaceID *provider.ResourceId
	for _, f := range req.Filters {
		switch f.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			spaceType := f.GetSpaceType()
			// do we need to fetch the shares?
			if spaceType == "+mountpoint" || spaceType == "+grant" {
				appendTypes = append(appendTypes, strings.TrimPrefix(spaceType, "+"))
				continue
			}
			spaceTypes[spaceType] = exists
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			spaceid, shareid, err := utils.SplitStorageSpaceID(f.GetId().OpaqueId)
			if err != nil {
				continue
			}
			if spaceid != utils.PublicStorageProviderID {
				return &provider.ListStorageSpacesResponse{
					// a specific id was requested, return not found instead of empty list
					Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND},
				}, nil
			}
			spaceID = &provider.ResourceId{StorageId: spaceid, OpaqueId: shareid}
		}
	}

	// if there is no public scope there are no publicstorage spaces
	share, _, ok := extractLinkAndInfo(ctx)
	if !ok {
		// TODO when the ocdav publicfile handler sends the token as opaqueid the
		// public scope needs to change
		// it always needs to check if the accessed path is a child of the resource identified by
		// the token
		return &provider.ListStorageSpacesResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		}, nil
	}

	if len(spaceTypes) == 0 {
		spaceTypes["mountpoint"] = exists
	}
	for _, s := range appendTypes {
		spaceTypes[s] = exists
	}

	res := &provider.ListStorageSpacesResponse{
		Status: status.NewOK(ctx),
	}
	for k := range spaceTypes {
		switch k {
		case "grant":
			// when a list storage space with the resourceid of an external
			// resource is made we may have a grant for it
			root := share.ResourceId
			// do we filter by id?
			if spaceID != nil && !utils.ResourceIDEqual(spaceID, root) {
				// none of our business
				continue
			}
			/*
				var opaque *typesv1beta1.Opaque
				if etag, ok := shareEtags[share.Id.OpaqueId]; ok {
					opaque = utils.AppendPlainToOpaque(opaque, "etag", etag)
				}
			*/
			// we know a grant for this resource
			space := &provider.StorageSpace{
				//Opaque: opaque,
				Id: &provider.StorageSpaceId{
					OpaqueId: root.StorageId + "!" + root.OpaqueId,
				},
				SpaceType: "grant",
				Owner:     &userv1beta1.User{Id: share.Owner},
				// the publicstorageprovider keeps track of mount points
				Root: root,
			}

			res.StorageSpaces = append(res.StorageSpaces, space)
		case "mountpoint":
			root := &provider.ResourceId{
				StorageId: utils.PublicStorageProviderID,
				OpaqueId:  share.Token, // the link share has no id, only the token
			}
			// do we filter by id
			if spaceID != nil {
				switch {
				case utils.ResourceIDEqual(spaceID, root):
					// we have a virtual node
				case utils.ResourceIDEqual(spaceID, share.ResourceId):
					// we have a mount point
					root = share.ResourceId
				default:
					// none of our business
					continue
				}
			}
			/*
				var opaque *typesv1beta1.Opaque
				if etag, ok := shareEtags[receivedShare.Share.Id.OpaqueId]; ok {
					opaque = utils.AppendPlainToOpaque(opaque, "etag", etag)
				}
			*/
			space := &provider.StorageSpace{
				//Opaque: opaque,
				Id: &provider.StorageSpaceId{
					OpaqueId: root.StorageId + "!" + root.OpaqueId,
				},
				SpaceType: "mountpoint",
				Owner:     &userv1beta1.User{Id: share.Owner}, // FIXME actually, the mount point belongs to the recipient
				// the publicstorageprovider keeps track of mount points
				Root: root,
			}

			// "Public share ~token"? nah, the spaceregistry uses the token in the name as the mountpoint
			// TODO use token in root opaqueid when building path
			space.Name = share.Token

			// what if we don't have a name?
			res.StorageSpaces = append(res.StorageSpaces, space)
		}
	}
	return res, nil
}

func extractLinkAndInfo(ctx context.Context) (*link.PublicShare, *provider.ResourceInfo, bool) {
	scopes, ok := ctxpkg.ContextGetScopes(ctx)
	if !ok {
		return nil, nil, false
	}
	var share *link.PublicShare
	var info *provider.ResourceInfo
	for k, v := range scopes {
		switch {
		case strings.HasPrefix(k, "publicshare:") && v.Resource.Decoder == "json":
			share = &link.PublicShare{}
			err := utils.UnmarshalJSONToProtoV1(v.Resource.Value, share)
			if err != nil {
				continue
			}
		case strings.HasPrefix(k, "resourceinfo:") && v.Resource.Decoder == "json":
			info = &provider.ResourceInfo{}
			err := utils.UnmarshalJSONToProtoV1(v.Resource.Value, info)
			if err != nil {
				continue
			}
		}
	}
	if share == nil || info == nil || !utils.ResourceIDEqual(share.ResourceId, info.Id) {
		return nil, nil, false
	}
	return share, info, true
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	ctx, span := rtrace.Provider.Tracer("publicstorageprovider").Start(ctx, "CreateContainer")
	defer span.End()

	span.SetAttributes(attribute.KeyValue{
		Key:   "reference",
		Value: attribute.StringValue(req.Ref.String()),
	})

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
			Status: status.NewInternal(ctx, "createContainer: error calling CreateContainer for ref:"+req.Ref.String()),
		}, nil
	}
	if res.Status.Code == rpc.Code_CODE_INTERNAL {
		return res, nil
	}

	return res, nil
}

func (s *service) TouchFile(ctx context.Context, req *provider.TouchFileRequest) (*provider.TouchFileResponse, error) {
	ref, _, _, st, err := s.translatePublicRefToCS3Ref(ctx, req.Ref)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.TouchFileResponse{
			Status: st,
		}, nil
	}
	return s.gateway.TouchFile(ctx, &provider.TouchFileRequest{Opaque: req.Opaque, Ref: ref})
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	ctx, span := rtrace.Provider.Tracer("publicstorageprovider").Start(ctx, "Delete")
	defer span.End()

	span.SetAttributes(attribute.KeyValue{
		Key:   "reference",
		Value: attribute.StringValue(req.Ref.String()),
	})

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
			Status: status.NewInternal(ctx, "Delete: error calling Delete for ref:"+req.Ref.String()),
		}, nil
	}
	if res.Status.Code == rpc.Code_CODE_INTERNAL {
		return res, nil
	}

	return res, nil
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	ctx, span := rtrace.Provider.Tracer("publicstorageprovider").Start(ctx, "Move")
	defer span.End()

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "source",
			Value: attribute.StringValue(req.Source.String()),
		},
		attribute.KeyValue{
			Key:   "destination",
			Value: attribute.StringValue(req.Destination.String()),
		},
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
			Status: status.NewInternal(ctx, "Move: error calling Move for source ref "+req.Source.String()+" to destination ref "+req.Destination.String()),
		}, nil
	}
	if res.Status.Code == rpc.Code_CODE_INTERNAL {
		return res, nil
	}

	return res, nil
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	ctx, span := rtrace.Provider.Tracer("publicstorageprovider").Start(ctx, "Stat")
	defer span.End()

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "source",
			Value: attribute.StringValue(req.Ref.String()),
		})

	share, _, ok := extractLinkAndInfo(ctx)
	if !ok {
		return &provider.StatResponse{
			Status: status.NewNotFound(ctx, "share or token not found"),
		}, nil
	}

	// the share is minimally populated, we need more than the token
	// look up complete share
	share, shareInfo, st, err := s.resolveToken(ctx, share.Token)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.StatResponse{
			Status: st,
		}, nil
	case share.GetPermissions() == nil || !share.GetPermissions().Permissions.Stat:
		return &provider.StatResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant Stat permission"),
		}, nil
	}

	if shareInfo.Type == provider.ResourceType_RESOURCE_TYPE_FILE || req.Ref.Path == "" {
		res := &provider.StatResponse{
			Status: status.NewOK(ctx),
			Info:   shareInfo,
		}
		s.augmentStatResponse(ctx, res, shareInfo, share, share.Token)
		return res, nil
	}

	ref := &provider.Reference{
		ResourceId: share.ResourceId,
		Path:       utils.MakeRelativePath(req.Ref.Path),
	}

	statResponse, err := s.gateway.Stat(ctx, &provider.StatRequest{Ref: ref})
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, "Stat: error calling Stat for ref:"+req.Ref.String()),
		}, nil
	}

	s.augmentStatResponse(ctx, statResponse, shareInfo, share, share.Token)

	return statResponse, nil
}

func (s *service) augmentStatResponse(ctx context.Context, res *provider.StatResponse, shareInfo *provider.ResourceInfo, share *link.PublicShare, tkn string) {
	// prevent leaking internal paths
	if res.Info != nil {
		if err := addShare(res.Info, share); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("share", share).Interface("info", res.Info).Msg("error when adding share")
		}

		var sharePath string
		if shareInfo.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
			sharePath = path.Base(shareInfo.Path)
		} else {
			sharePath = strings.TrimPrefix(res.Info.Path, shareInfo.Path)
		}

		res.Info.Path = path.Join("/", sharePath)
		s.setPublicStorageID(res.Info, tkn)
		filterPermissions(res.Info.PermissionSet, share.GetPermissions().Permissions)
	}
}

// setPublicStorageID encodes the actual spaceid and nodeid as an opaqueid in the publicstorageprovider space
func (s *service) setPublicStorageID(info *provider.ResourceInfo, shareToken string) {
	info.Id.StorageId = utils.PublicStorageProviderID
	info.Id.OpaqueId = shareToken
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

	share, _, ok := extractLinkAndInfo(ctx)
	if !ok {
		return &provider.ListContainerResponse{
			Status: status.NewNotFound(ctx, "share or token not found"),
		}, nil
	}
	// the share is minimally populated, we need more than the token
	// look up complete share
	share, _, st, err := s.resolveToken(ctx, share.Token)
	switch {
	case err != nil:
		return nil, err
	case st != nil:
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}
	if share.GetPermissions() == nil || !share.GetPermissions().Permissions.ListContainer {
		return &provider.ListContainerResponse{
			Status: status.NewPermissionDenied(ctx, nil, "share does not grant ListContainer permission"),
		}, nil
	}

	listContainerR, err := s.gateway.ListContainer(
		ctx,
		&provider.ListContainerRequest{
			Ref: &provider.Reference{
				ResourceId: share.ResourceId,
				// prefix relative path with './' to make it a CS3 relative path
				Path: utils.MakeRelativePath(req.Ref.Path),
			},
		},
	)
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, "ListContainer: error calling ListContainer for ref:"+req.Ref.String()),
		}, nil
	}

	for i := range listContainerR.Infos {
		// FIXME how do we reduce permissions to what is granted by the public link?
		filterPermissions(listContainerR.Infos[i].PermissionSet, share.GetPermissions().Permissions)
		//s.setPublicStorageID(listContainerR.Infos[i], tkn)
		if err := addShare(listContainerR.Infos[i], share); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("share", share).Interface("info", listContainerR.Infos[i]).Msg("error when adding share")
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

func (s *service) DenyGrant(ctx context.Context, req *provider.DenyGrantRequest) (*provider.DenyGrantResponse, error) {
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

// resolveToken returns the path and share for the publicly shared resource.
func (s *service) resolveToken(ctx context.Context, token string) (*link.PublicShare, *provider.ResourceInfo, *rpc.Status, error) {
	driver, err := pool.GetGatewayServiceClient(s.conf.GatewayAddr)
	if err != nil {
		return nil, nil, nil, err
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
		return nil, nil, nil, err
	case publicShareResponse.Status.Code != rpc.Code_CODE_OK:
		return nil, nil, publicShareResponse.Status, nil
	}

	sRes, err := s.gateway.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: publicShareResponse.GetShare().GetResourceId(),
		},
	})
	switch {
	case err != nil:
		return nil, nil, nil, err
	case sRes.Status.Code != rpc.Code_CODE_OK:
		return nil, nil, sRes.Status, nil
	}
	return publicShareResponse.GetShare(), sRes.Info, nil, nil
}
