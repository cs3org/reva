// Copyright 2018-2024 CERN
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

package gateway

import (
	"context"
	"net/url"
	"path"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
)

// transferClaims are custom claims for a JWT token to be used between the metadata and data gateways.
type transferClaims struct {
	jwt.StandardClaims
	Target     string `json:"target"`
	VersionKey string `json:"version_key,omitempty"`
}

func (s *svc) sign(_ context.Context, target, versionKey string) (string, error) {
	// Tus sends a separate request to the datagateway service for every chunk.
	// For large files, this can take a long time, so we extend the expiration
	ttl := time.Duration(s.c.TransferExpires) * time.Second
	claims := transferClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
			Audience:  "reva",
			IssuedAt:  time.Now().Unix(),
		},
		Target:     target,
		VersionKey: versionKey,
	}

	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tkn, err := t.SignedString([]byte(s.c.TransferSharedSecret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", claims)
	}

	return tkn, nil
}

func (s *svc) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	log := appctx.GetLogger(ctx)
	home := s.getHome(ctx)
	c, err := s.findByPath(ctx, home)
	if err != nil {
		log.Err(err).Str("home", home).Msg("gateway: error finding home on storage provider")
		return &provider.CreateHomeResponse{
			Status: status.NewStatusFromErrType(ctx, "error finding home", err),
		}, nil
	}

	res, err := c.CreateHome(ctx, req)
	if err != nil {
		log.Err(err).Str("home", home).Msg("gateway: error creating home on storage provider")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}
	return res, nil
}

func (s *svc) GetHome(ctx context.Context, _ *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return &provider.GetHomeResponse{
		Path:   s.getHome(ctx),
		Status: status.NewOK(ctx),
	}, nil
}

func (s *svc) getHome(ctx context.Context) string {
	u := appctx.ContextMustGetUser(ctx)
	return templates.WithUser(u, s.c.HomeLayout)
}

func (s *svc) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	if utils.IsRelativeReference(req.Ref) {
		return s.initiateFileDownload(ctx, req)
	}

	statReq := &provider.StatRequest{Ref: req.Ref}
	statRes, err := s.stat(ctx, statReq)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &gateway.InitiateFileDownloadResponse{
			Status: statRes.Status,
		}, nil
	}
	return s.initiateFileDownload(ctx, req)

}

func versionKey(req *provider.InitiateFileDownloadRequest) string {
	if req.Opaque == nil || req.Opaque.Map == nil {
		return ""
	}
	val := req.Opaque.Map["version_key"]
	if val == nil {
		return ""
	}
	return string(val.Value)
}

func (s *svc) initiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	// TODO(ishank011): enable downloading references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewStatusFromErrType(ctx, "error initiating download ref="+req.Ref.String(), err),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &gateway.InitiateFileDownloadResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileDownload")
	}

	protocols := make([]*gateway.FileDownloadProtocol, len(storageRes.Protocols))
	for p := range storageRes.Protocols {
		protocols[p] = &gateway.FileDownloadProtocol{
			Opaque:           storageRes.Protocols[p].Opaque,
			Protocol:         storageRes.Protocols[p].Protocol,
			DownloadEndpoint: storageRes.Protocols[p].DownloadEndpoint,
		}

		if !storageRes.Protocols[p].Expose {
			// sign the download location and pass it to the data gateway
			u, err := url.Parse(protocols[p].DownloadEndpoint)
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewInternal(ctx, err, "wrong format for download endpoint"),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target, versionKey(req))
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewInternal(ctx, err, "error creating signature for download"),
				}, nil
			}

			protocols[p].DownloadEndpoint = s.c.DataGatewayEndpoint
			protocols[p].Token = token
		}
	}

	return &gateway.InitiateFileDownloadResponse{
		Opaque:    storageRes.Opaque,
		Status:    storageRes.Status,
		Protocols: protocols,
	}, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	return s.initiateFileUpload(ctx, req)
}

func (s *svc) initiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewStatusFromErrType(ctx, "initiateFileUpload ref="+req.Ref.String(), err),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &gateway.InitiateFileUploadResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileUpload")
	}

	if storageRes.Status.Code != rpc.Code_CODE_OK {
		return &gateway.InitiateFileUploadResponse{
			Status: storageRes.Status,
		}, nil
	}

	protocols := make([]*gateway.FileUploadProtocol, len(storageRes.Protocols))
	for p := range storageRes.Protocols {
		protocols[p] = &gateway.FileUploadProtocol{
			Opaque:             storageRes.Protocols[p].Opaque,
			Protocol:           storageRes.Protocols[p].Protocol,
			UploadEndpoint:     storageRes.Protocols[p].UploadEndpoint,
			AvailableChecksums: storageRes.Protocols[p].AvailableChecksums,
		}

		if !storageRes.Protocols[p].Expose {
			// sign the upload location and pass it to the data gateway
			u, err := url.Parse(protocols[p].UploadEndpoint)
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, err, "wrong format for upload endpoint"),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target, "")
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, err, "error creating signature for upload"),
				}, nil
			}

			protocols[p].UploadEndpoint = s.c.DataGatewayEndpoint
			protocols[p].Token = token
		}
	}

	return &gateway.InitiateFileUploadResponse{
		Opaque:    storageRes.Opaque,
		Status:    storageRes.Status,
		Protocols: protocols,
	}, nil
}

func (s *svc) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	statReq := &provider.StatRequest{Ref: &provider.Reference{ResourceId: req.ResourceId}}
	statRes, err := s.stat(ctx, statReq)
	if err != nil {
		err = errors.Wrap(err, "gateway: error stating ref:"+statReq.Ref.String())
		return nil, err
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.GetPathResponse{
			Status: statRes.Status,
		}, nil
	}

	return &provider.GetPathResponse{
		Status: statRes.Status,
		Path:   statRes.GetInfo().GetPath(),
	}, nil
}

func (s *svc) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	if utils.IsRelativeReference(req.Ref) {
		return s.createContainer(ctx, req)
	}

	return s.createContainer(ctx, req)
}

func (s *svc) createContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "createContainer ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.CreateContainerResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	return res, nil
}

func (s *svc) TouchFile(ctx context.Context, req *provider.TouchFileRequest) (*provider.TouchFileResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.TouchFileResponse{
			Status: status.NewStatusFromErrType(ctx, "TouchFile ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.TouchFile(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.TouchFileResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling TouchFile")
	}

	return res, nil
}

func (s *svc) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	return s.delete(ctx, req)
}

func (s *svc) delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	// TODO(ishank011): enable deleting references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewStatusFromErrType(ctx, "delete ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.DeleteResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	return s.move(ctx, req)
}

func (s *svc) move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	srcProviders, err := s.findProviders(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "move src="+req.Source.String(), err),
		}, nil
	}

	dstProviders, err := s.findProviders(ctx, req.Destination)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "move dst="+req.Destination.String(), err),
		}, nil
	}

	// if providers are not the same we do not implement cross storage move yet.
	if len(srcProviders) != 1 || len(dstProviders) != 1 {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	srcProvider, dstProvider := srcProviders[0], dstProviders[0]

	// if providers are not the same we do not implement cross storage copy yet.
	if srcProvider.Address != dstProvider.Address {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	c, err := s.getStorageProviderClient(ctx, srcProvider)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error connecting to storage provider="+srcProvider.Address),
		}, nil
	}

	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.SetArbitraryMetadataResponse{
			Status: status.NewStatusFromErrType(ctx, "SetArbitraryMetadata ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.SetArbitraryMetadata(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.SetArbitraryMetadataResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling SetArbitraryMetadata")
	}

	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.UnsetArbitraryMetadataResponse{
			Status: status.NewStatusFromErrType(ctx, "UnsetArbitraryMetadata ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.UnsetArbitraryMetadata(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.UnsetArbitraryMetadataResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling UnsetArbitraryMetadata")
	}

	return res, nil
}

// SetLock puts a lock on the given reference.
func (s *svc) SetLock(ctx context.Context, req *provider.SetLockRequest) (*provider.SetLockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.SetLockResponse{
			Status: status.NewStatusFromErrType(ctx, "SetLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.SetLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.SetLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling SetLock")
	}

	return res, nil
}

// GetLock returns an existing lock on the given reference.
func (s *svc) GetLock(ctx context.Context, req *provider.GetLockRequest) (*provider.GetLockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.GetLockResponse{
			Status: status.NewStatusFromErrType(ctx, "GetLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.GetLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.GetLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling GetLock")
	}

	return res, nil
}

// RefreshLock refreshes an existing lock on the given reference.
func (s *svc) RefreshLock(ctx context.Context, req *provider.RefreshLockRequest) (*provider.RefreshLockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RefreshLockResponse{
			Status: status.NewStatusFromErrType(ctx, "RefreshLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RefreshLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.RefreshLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling RefreshLock")
	}

	return res, nil
}

// Unlock removes an existing lock from the given reference.
func (s *svc) Unlock(ctx context.Context, req *provider.UnlockRequest) (*provider.UnlockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.UnlockResponse{
			Status: status.NewStatusFromErrType(ctx, "Unlock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Unlock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.UnlockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling Unlock")
	}

	return res, nil
}

func (s *svc) stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	providers, err := s.findProviders(ctx, req.Ref)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewStatusFromErrType(ctx, "stat ref: "+req.Ref.String(), err),
		}, nil
	}
	providers = getUniqueProviders(providers)

	resPath := req.Ref.GetPath()
	if len(providers) == 1 && (utils.IsRelativeReference(req.Ref) || resPath == "" || strings.HasPrefix(resPath, providers[0].ProviderPath)) {
		c, err := s.getStorageProviderClient(ctx, providers[0])
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "error connecting to storage provider="+providers[0].Address),
			}, nil
		}
		rsp, err := c.Stat(ctx, req)
		if err != nil || rsp.Status.Code != rpc.Code_CODE_OK {
			return rsp, err
		}
		return rsp, nil
	}

	// otherwise, this is a Stat for "/", which corresponds to a 0-Depth PROPFIND from web to just get the fileid:
	// we respond with an hardcoded value, no need to poke all storage providers as we did before
	info := &provider.ResourceInfo{
		Id: &provider.ResourceId{
			StorageId: "/",
			OpaqueId:  uuid.New().String(),
		},
		Type:     provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Path:     req.Ref.GetPath(),
		MimeType: "httpd/unix-directory",
		Size:     0,
		Mtime:    &types.Timestamp{},
	}

	return &provider.StatResponse{
		Status: status.NewOK(ctx),
		Info:   info,
	}, nil
}

func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	if utils.IsRelativeReference(req.Ref) {
		return s.stat(ctx, req)
	}
	return s.stat(ctx, req)
}

func (s *svc) checkRef(ctx context.Context, ri *provider.ResourceInfo) (*provider.ResourceInfo, string, error) {
	if ri.Type != provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		panic("gateway: calling checkRef on a non reference type:" + ri.String())
	}

	// reference types MUST have a target resource id.
	if ri.Target == "" {
		err := errtypes.BadRequest("gateway: ref target is an empty uri")
		return nil, "", err
	}

	uri, err := url.Parse(ri.Target)
	if err != nil {
		return nil, "", errors.Wrapf(err, "gateway: error parsing target uri: %s", ri.Target)
	}

	switch uri.Scheme {
	case "cs3":
		ref, err := s.handleCS3Ref(ctx, uri.Opaque)
		return ref, "cs3", err
	case "webdav":
		return nil, "webdav", nil
	default:
		err := errtypes.BadRequest("gateway: no reference handler for scheme: " + uri.Scheme)
		return nil, "", err
	}
}

func (s *svc) handleCS3Ref(ctx context.Context, opaque string) (*provider.ResourceInfo, error) {
	// a cs3 ref has the following layout: <storage_id>/<opaque_id>
	parts := strings.SplitN(opaque, "/", 2)
	if len(parts) < 2 {
		err := errtypes.BadRequest("gateway: cs3 ref does not follow the layout storageid/opaqueid:" + opaque)
		return nil, err
	}

	// we could call here the Stat method again, but that is calling for problems in case
	// there is a loop of targets pointing to targets, so better avoid it.

	req := &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: parts[0],
				OpaqueId:  parts[1],
			},
		},
	}
	res, err := s.stat(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling stat")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			return nil, errtypes.NotFound(req.Ref.String())
		case rpc.Code_CODE_PERMISSION_DENIED:
			return nil, errtypes.PermissionDenied(req.Ref.String())
		case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_OUT_OF_RANGE:
			return nil, errtypes.BadRequest(req.Ref.String())
		case rpc.Code_CODE_UNIMPLEMENTED:
			return nil, errtypes.NotSupported(req.Ref.String())
		default:
			return nil, errtypes.InternalError("gateway: error stating target reference")
		}
	}

	if res.Info.Type == provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		err := errtypes.BadRequest("gateway: error the target of a reference cannot be another reference")
		return nil, err
	}

	return res.Info, nil
}

func (s *svc) ListContainerStream(_ *provider.ListContainerStreamRequest, _ gateway.GatewayAPI_ListContainerStreamServer) error {
	return errtypes.NotSupported("Unimplemented")
}

func (s *svc) filterProvidersByUserAgent(ctx context.Context, providers []*registry.ProviderInfo) []*registry.ProviderInfo {
	cat, ok := appctx.ContextGetUserAgentCategory(ctx)
	if !ok {
		return providers
	}

	filters := []*registry.ProviderInfo{}
	for _, p := range providers {
		if s.isPathAllowed(cat, p.ProviderPath) {
			filters = append(filters, p)
		}
	}
	return filters
}

func (s *svc) isPathAllowed(cat string, path string) bool {
	allowedUserAgents, ok := s.c.AllowedUserAgents[path]
	if !ok {
		// if no user agent is defined for a path, all user agents are allowed
		return true
	}

	for _, userAgent := range allowedUserAgents {
		if userAgent == cat {
			return true
		}
	}
	return false
}

func (s *svc) listContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	providers, err := s.findProviders(ctx, req.Ref)
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "listContainer ref: "+req.Ref.String(), err),
		}, nil
	}
	providers = getUniqueProviders(providers)

	resPath := req.Ref.GetPath()

	if len(providers) == 1 && (utils.IsRelativeReference(req.Ref) || resPath == "" || strings.HasPrefix(resPath, providers[0].ProviderPath)) {
		c, err := s.getStorageProviderClient(ctx, providers[0])
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "error connecting to storage provider="+providers[0].Address),
			}, nil
		}
		rsp, err := c.ListContainer(ctx, req)
		if err != nil || rsp.Status.Code != rpc.Code_CODE_OK {
			return rsp, err
		}
		return rsp, nil
	}

	return s.listContainerAcrossProviders(ctx, req, providers)
}

func (s *svc) listContainerAcrossProviders(ctx context.Context, req *provider.ListContainerRequest, providers []*registry.ProviderInfo) (*provider.ListContainerResponse, error) {
	nestedInfos := make(map[string]*provider.ResourceInfo)
	log := appctx.GetLogger(ctx)

	for _, p := range s.filterProvidersByUserAgent(ctx, providers) {
		c, err := s.getStorageProviderClient(ctx, p)
		if err != nil {
			log.Err(err).Msg("error connecting to storage provider=" + p.Address)
			continue
		}
		resp, err := c.ListContainer(ctx, req)
		if err != nil {
			log.Err(err).Msgf("gateway: error calling Stat %s: %+v", req.Ref.String(), p)
			continue
		}
		if resp.Status.Code != rpc.Code_CODE_OK {
			log.Err(status.NewErrorFromCode(rpc.Code_CODE_OK, "gateway")).Send()
			continue
		}

		for _, info := range resp.Infos {
			if p, ok := nestedInfos[info.Path]; ok {
				// Since more than one providers contribute to this path,
				// use a generic ID
				p.Id = &provider.ResourceId{
					StorageId: "/",
					OpaqueId:  uuid.New().String(),
				}
				// TODO(ishank011): aggregrate properties such as etag, checksum, etc.
				p.Size += info.Size
				if utils.TSToUnixNano(info.Mtime) > utils.TSToUnixNano(p.Mtime) {
					p.Mtime = info.Mtime
					p.Etag = info.Etag
					p.Checksum = info.Checksum
				}
				if p.Etag == "" && p.Etag != info.Etag {
					p.Etag = info.Etag
				}
				p.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
				p.MimeType = "httpd/unix-directory"
			} else {
				nestedInfos[info.Path] = info
			}
		}
	}

	infos := make([]*provider.ResourceInfo, 0, len(nestedInfos))
	for _, info := range nestedInfos {
		infos = append(infos, info)
	}

	return &provider.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}, nil
}

func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	return s.listContainer(ctx, req)
}

func (s *svc) getPath(ctx context.Context, ref *provider.Reference, keys ...string) (string, *rpc.Status) {
	// check if it is an id based or combined reference first
	if ref.ResourceId != nil {
		req := &provider.StatRequest{Ref: ref, ArbitraryMetadataKeys: keys}
		res, err := s.stat(ctx, req)
		if err != nil {
			return "", status.NewStatusFromErrType(ctx, "getPath ref="+ref.String(), err)
		}
		if res != nil && res.Status.Code != rpc.Code_CODE_OK {
			return "", res.Status
		}

		return res.Info.Path, res.Status
	}

	if utils.IsAbsolutePathReference(ref) {
		return ref.Path, &rpc.Status{Code: rpc.Code_CODE_OK}
	}
	return "", &rpc.Status{Code: rpc.Code_CODE_INTERNAL}
}

// /home/MyShares/.
func (s *svc) isSharedFolder(ctx context.Context, p string) bool {
	return s.split(ctx, p, 2)
}

// /home/MyShares/photos/.
func (s *svc) isShareName(ctx context.Context, p string) bool {
	return s.split(ctx, p, 3)
}

// /home/MyShares/photos/Ibiza/beach.png.
func (s *svc) isShareChild(ctx context.Context, p string) bool {
	return s.split(ctx, p, 4)
}

// always validate that the path contains the share folder
// split cannot be called with i<2.
func (s *svc) split(ctx context.Context, p string, i int) bool {
	log := appctx.GetLogger(ctx)
	if i < 2 {
		panic("split called with i < 2")
	}

	parts := s.splitPath(ctx, p)

	// validate that we have always at least two elements
	if len(parts) < 2 {
		return false
	}

	// validate the share folder is always the second element, first element is always the hardcoded value of "home"
	if parts[1] != s.c.ShareFolder {
		log.Debug().Msgf("gateway: split: parts[1]:%+v != shareFolder:%+v", parts[1], s.c.ShareFolder)
		return false
	}

	log.Debug().Msgf("gateway: split: path:%+v parts:%+v shareFolder:%+v", p, parts, s.c.ShareFolder)

	if len(parts) == i && parts[i-1] != "" {
		return true
	}

	return false
}

// path must contain a share path with share children, if not it will panic.
// should be called after checking isShareChild == true.
func (s *svc) splitShare(ctx context.Context, p string) (string, string) {
	parts := s.splitPath(ctx, p)
	if len(parts) != 4 {
		panic("gateway: path for splitShare does not contain 4 elements:" + p)
	}

	shareName := path.Join("/", parts[0], parts[1], parts[2])
	shareChild := path.Join("/", parts[3])
	return shareName, shareChild
}

func (s *svc) splitPath(_ context.Context, p string) []string {
	p = strings.Trim(p, "/")
	return strings.SplitN(p, "/", 4) // ["home", "MyShares", "photos", "Ibiza/beach.png"]
}

func (s *svc) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return &provider.CreateSymlinkResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CreateSymlink not implemented"), "CreateSymlink not implemented"),
	}, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.ListFileVersionsResponse{
			Status: status.NewStatusFromErrType(ctx, "ListFileVersions ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.ListFileVersions(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListFileVersions")
	}

	return res, nil
}

func (s *svc) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreFileVersion ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreFileVersion")
	}

	return res, nil
}

func (s *svc) ListRecycleStream(_ *provider.ListRecycleStreamRequest, _ gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errtypes.NotSupported("ListRecycleStream unimplemented")
}

// TODO use the ListRecycleRequest.Ref to only list the trash of a specific storage.
func (s *svc) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	c, err := s.find(ctx, req.GetRef())
	if err != nil {
		return &provider.ListRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "ListFileVersions ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.ListRecycle(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListRecycleRequest")
	}

	return res, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreRecycleItem")
	}

	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "PurgeRecycle ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling PurgeRecycle")
	}
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewStatusFromErrType(ctx, "GetQuota ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.GetQuota(ctx, &provider.GetQuotaRequest{
		Opaque: req.GetOpaque(),
		Ref:    req.GetRef(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetQuota")
	}
	return res, nil
}

func (s *svc) findByPath(ctx context.Context, path string) (provider.ProviderAPIClient, error) {
	ref := &provider.Reference{Path: path}
	return s.find(ctx, ref)
}

func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, error) {
	p, err := s.findProviders(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.getStorageProviderClient(ctx, p[0])
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(pool.Endpoint(p.Address))
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return c, nil
}

func (s *svc) findProviders(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error) {
	c, err := pool.GetStorageRegistryClient(pool.Endpoint(s.c.StorageRegistryEndpoint))
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	res, err := c.GetStorageProviders(ctx, &registry.GetStorageProvidersRequest{
		Ref: ref,
	})

	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetStorageProvider")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			return nil, errtypes.NotFound("gateway: storage provider not found for reference:" + ref.String())
		case rpc.Code_CODE_PERMISSION_DENIED:
			return nil, errtypes.PermissionDenied("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_OUT_OF_RANGE:
			return nil, errtypes.BadRequest("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_UNIMPLEMENTED:
			return nil, errtypes.NotSupported("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		default:
			return nil, status.NewErrorFromCode(res.Status.Code, "gateway")
		}
	}

	if res.Providers == nil {
		return nil, errtypes.NotFound("gateway: provider is nil")
	}

	return res.Providers, nil
}

func getUniqueProviders(providers []*registry.ProviderInfo) []*registry.ProviderInfo {
	unique := make(map[string]*registry.ProviderInfo)
	for _, p := range providers {
		unique[p.Address] = p
	}
	res := make([]*registry.ProviderInfo, 0, len(unique))
	for _, provider := range unique {
		res = append(res, provider)
	}
	return res
}

type etagWithTS struct {
	Etag      string
	Timestamp time.Time
}
