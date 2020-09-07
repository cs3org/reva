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

package gateway

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// transferClaims are custom claims for a JWT token to be used between the metadata and data gateways.
type transferClaims struct {
	jwt.StandardClaims
	Target string `json:"target"`
}

func (s *svc) sign(_ context.Context, target string) (string, error) {
	ttl := time.Duration(s.c.TransferExpires) * time.Second
	claims := transferClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
			Audience:  "reva",
			IssuedAt:  time.Now().Unix(),
		},
		Target: target,
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
		log.Err(err).Msg("gateway: error finding storage provider")
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.CreateHomeResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.CreateHome(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error creating home on storage provider")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}

	return res, nil

}
func (s *svc) GetHome(ctx context.Context, _ *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	home := s.getHome(ctx)
	homeRes := &provider.GetHomeResponse{Path: home, Status: status.NewOK(ctx)}
	return homeRes, nil
}

func (s *svc) getHome(_ context.Context) string {
	// TODO(labkode): issue #601, /home will be hardcoded.
	return "/home"
}
func (s *svc) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error initiating file download id: %v", req.Ref.GetId())
		return &gateway.InitiateFileDownloadResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) {
		statReq := &provider.StatRequest{Ref: req.Ref}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}
		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}
		return s.initiateFileDownload(ctx, req)
	}

	if s.isSharedFolder(ctx, p) || s.isShareName(ctx, p) {
		log.Debug().Msgf("path:%s points to shared folder or share name", p)
		err := errtypes.PermissionDenied("gateway: cannot download share folder or share name: path=" + p)
		log.Err(err).Msg("gateway: error downloading")
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInvalidArg(ctx, "path points to share folder or share name"),
		}, nil

	}

	if s.isShareChild(ctx, p) {
		log.Debug().Msgf("shared child: %s", p)
		shareName, shareChild := s.splitShare(ctx, p)

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: shareName,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error creating container")
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			log.Err(err).Msg("gateway: error resolving reference")
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewInternal(ctx, err, "error creating container"),
			}, nil
		}

		if protocol == "webdav" {
			// TODO(ishank011): pass this through the datagateway service
			// for now, we just expose the file server to the user
			ep, opaque, err := s.webdavRefTransferEndpoint(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewInternal(ctx, err, "gateway: error downloading from webdav host: "+p),
				}, nil
			}
			return &gateway.InitiateFileDownloadResponse{
				Opaque:           opaque,
				Status:           status.NewOK(ctx),
				DownloadEndpoint: ep,
			}, nil
		}

		// append child to target
		target := path.Join(ri.Path, shareChild)
		req.Ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: target,
			},
		}
		log.Debug().Msg("download path: " + target)
		return s.initiateFileDownload(ctx, req)
	}

	panic("gateway: download: unknown path:" + p)
}

func (s *svc) initiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileDownload")
	}

	res := &gateway.InitiateFileDownloadResponse{
		Opaque:           storageRes.Opaque,
		Status:           storageRes.Status,
		DownloadEndpoint: storageRes.DownloadEndpoint,
	}

	if storageRes.Expose {
		log.Info().Msg("download is routed directly to data server - skipping data gateway")
		return res, nil
	}

	// sign the download location and pass it to the data gateway
	u, err := url.Parse(res.DownloadEndpoint)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "wrong format for download endpoint"),
		}, nil
	}

	// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
	target := u.String()
	token, err := s.sign(ctx, target)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error creating signature for download"),
		}, nil
	}

	res.DownloadEndpoint = s.c.DataGatewayEndpoint
	res.Token = token

	return res, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error initiating file upload id: %v", req.Ref.GetId())
		return &gateway.InitiateFileUploadResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) {
		return s.initiateFileUpload(ctx, req)
	}

	if s.isSharedFolder(ctx, p) || s.isShareName(ctx, p) {
		log.Debug().Msgf("path:%s points to shared folder or share name", p)
		err := errtypes.PermissionDenied("gateway: cannot upload to share folder or share name: path=" + p)
		log.Err(err).Msg("gateway: error downloading")
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInvalidArg(ctx, "path points to share folder or share name"),
		}, nil

	}

	if s.isShareChild(ctx, p) {
		log.Debug().Msgf("shared child: %s", p)
		shareName, shareChild := s.splitShare(ctx, p)

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: shareName,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error uploading")
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			log.Err(err).Msg("gateway: error resolving reference")
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewInternal(ctx, err, "error creating container"),
			}, nil
		}

		if protocol == "webdav" {
			// TODO(ishank011): pass this through the datagateway service
			// for now, we just expose the file server to the user
			ep, opaque, err := s.webdavRefTransferEndpoint(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, err, "gateway: error uploading to webdav host: "+p),
				}, nil
			}
			return &gateway.InitiateFileUploadResponse{
				Opaque:         opaque,
				Status:         status.NewOK(ctx),
				UploadEndpoint: ep,
			}, nil
		}

		// append child to target
		target := path.Join(ri.Path, shareChild)
		req.Ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: target,
			},
		}
		return s.initiateFileUpload(ctx, req)
	}

	panic("gateway: upload: unknown path:" + p)
}

func (s *svc) initiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileUpload")
	}

	if storageRes.Status.Code != rpc.Code_CODE_OK {
		if storageRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		err := status.NewErrorFromCode(storageRes.Status.Code, "gateway")
		log.Err(err).Msg("gateway: upload: error uploading")
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error initiating upload"),
		}, nil

	}

	res := &gateway.InitiateFileUploadResponse{
		Opaque:             storageRes.Opaque,
		Status:             storageRes.Status,
		UploadEndpoint:     storageRes.UploadEndpoint,
		AvailableChecksums: storageRes.AvailableChecksums,
	}

	if storageRes.Expose {
		log.Info().Msg("upload is routed directly to data server - skipping data gateway")
		return res, nil
	}

	// sign the upload location and pass it to the data gateway
	u, err := url.Parse(res.UploadEndpoint)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "wrong format for upload endpoint"),
		}, nil
	}

	// TODO(labkode): calculate signature of the url, we only sign the URI. At some points maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
	target := u.String()
	token, err := s.sign(ctx, target)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error creating signature for download"),
		}, nil
	}

	res.UploadEndpoint = s.c.DataGatewayEndpoint
	res.Token = token

	return res, nil
}

func (s *svc) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	statReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: req.ResourceId,
			},
		},
	}
	statRes, err := s.stat(ctx, statReq)
	if err != nil {
		err = errors.Wrap(err, "gateway: error stating ref:"+statReq.Ref.String())
		return nil, err
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.GetPathResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
			}, nil
		}
		err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
		return &provider.GetPathResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
		}, nil
	}

	return &provider.GetPathResponse{
		Status: statRes.Status,
		Path:   statRes.GetInfo().GetPath(),
	}, nil
}

func (s *svc) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.CreateContainerResponse{
				Status: status.NewNotFound(ctx, "gateway: container not found:"+req.Ref.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error creating container on reference id: %v", req.Ref.GetId())
		return &provider.CreateContainerResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) {
		return s.createContainer(ctx, req)
	}

	if s.isSharedFolder(ctx, p) || s.isShareName(ctx, p) {
		log.Debug().Msgf("path:%s points to shared folder or share name", p)
		err := errtypes.PermissionDenied("gateway: cannot create container on share folder or share name: path=" + p)
		log.Err(err).Msg("gateway: error creating container")
		return &provider.CreateContainerResponse{
			Status: status.NewInvalidArg(ctx, "path points to share folder or share name"),
		}, nil

	}

	if s.isShareChild(ctx, p) {
		log.Debug().Msgf("shared child: %s", p)
		shareName, shareChild := s.splitShare(ctx, p)

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: shareName,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.CreateContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.CreateContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: container not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error creating container")
			return &provider.CreateContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.CreateContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			log.Err(err).Msg("gateway: error resolving reference")
			return &provider.CreateContainerResponse{
				Status: status.NewInternal(ctx, err, "error creating container"),
			}, nil
		}

		if protocol == "webdav" {
			err = s.webdavRefMkdir(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &provider.CreateContainerResponse{
					Status: status.NewInternal(ctx, err, "gateway: error creating container on webdav host: "+p),
				}, nil
			}
			return &provider.CreateContainerResponse{
				Status: status.NewOK(ctx),
			}, nil
		}

		// append child to target
		target := path.Join(ri.Path, shareChild)
		req.Ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: target,
			},
		}
		return s.createContainer(ctx, req)
	}

	panic("gateway: create container on unknown path:" + p)
}

func (s *svc) createContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.CreateContainerResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	return res, nil
}

// check if the path contains the prefix of the shared folder
func (s *svc) inSharedFolder(ctx context.Context, p string) bool {
	sharedFolder := s.getSharedFolder(ctx)
	return strings.HasPrefix(p, sharedFolder)
}

func (s *svc) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.DeleteResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error deleting reference id: %v", req.Ref.GetId())
		return &provider.DeleteResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) {
		return s.delete(ctx, req)
	}

	if s.isSharedFolder(ctx, p) {
		// TODO(labkode): deleting share names should be allowed, means unmounting.
		log.Debug().Msgf("path:%s points to shared folder or share name", p)
		err := errtypes.PermissionDenied("gateway: cannot delete share folder or share name: path=" + p)
		log.Err(err).Msg("gateway: error creating container")
		return &provider.DeleteResponse{
			Status: status.NewInvalidArg(ctx, "path points to share folder or share name"),
		}, nil

	}

	if s.isShareName(ctx, p) {
		log.Debug().Msgf("path:%s points to share name", p)

		ref := &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: p,
			},
		}

		req.Ref = ref
		return s.delete(ctx, req)
	}

	if s.isShareChild(ctx, p) {
		shareName, shareChild := s.splitShare(ctx, p)
		log.Debug().Msgf("path:%s sharename:%s sharechild: %s", p, shareName, shareChild)

		ref := &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: shareName,
			},
		}

		statReq := &provider.StatRequest{Ref: ref}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.DeleteResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.DeleteResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error deleting")
			return &provider.DeleteResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.DeleteResponse{
					Status: status.NewNotFound(ctx, "gateway: not found"),
				}, nil
			}
			log.Err(err).Msg("gateway: error resolving reference")
			return &provider.DeleteResponse{
				Status: status.NewInternal(ctx, err, "error creating container"),
			}, nil
		}

		if protocol == "webdav" {
			err = s.webdavRefDelete(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &provider.DeleteResponse{
					Status: status.NewInternal(ctx, err, "gateway: error deleting resource on webdav host: "+p),
				}, nil
			}
			return &provider.DeleteResponse{
				Status: status.NewOK(ctx),
			}, nil
		}

		// append child to target
		target := path.Join(ri.Path, shareChild)
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: target,
			},
		}

		req.Ref = ref
		return s.delete(ctx, req)
	}

	panic("gateway: delete called on unknown path:" + p)
}

func (s *svc) delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.DeleteResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Source)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.MoveResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Source.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error moving reference id: %v to `%v`", req.Source.GetId(), req.Destination.String())
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	dp, st2 := s.getPath(ctx, req.Destination)
	if st2.Code != rpc.Code_CODE_OK && st2.Code != rpc.Code_CODE_NOT_FOUND {
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) && !s.inSharedFolder(ctx, dp) {
		return s.move(ctx, req)
	}

	// allow renaming the share folder, the mount point, not the target.
	if s.isShareName(ctx, p) && s.isShareName(ctx, dp) {
		log.Info().Msgf("gateway: move: renaming share mountpoint: from:%s to:%s", p, dp)
		return s.move(ctx, req)
	}

	// resolve references and check the ref points to the same base path, paranoia check.
	if s.isShareChild(ctx, p) && s.isShareChild(ctx, dp) {
		shareName, shareChild := s.splitShare(ctx, p)
		dshareName, dshareChild := s.splitShare(ctx, dp)
		log.Debug().Msgf("srcpath:%s dstpath:%s srcsharename:%s srcsharechild: %s dstsharename:%s dstsharechild:%s ", p, dp, shareName, shareChild, dshareName, dshareChild)

		if shareName != dshareName {
			err := errors.New("gateway: move: src and dst points to different targets")
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "gateway: error moving"),
			}, nil

		}

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: shareName,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.MoveResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error moving")
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.MoveResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			log.Err(err).Msg("gateway: error resolving reference")
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "error moving"),
			}, nil
		}

		if protocol == "webdav" {
			err = s.webdavRefMove(ctx, statRes.Info.Target, shareChild, dshareChild)
			if err != nil {
				return &provider.MoveResponse{
					Status: status.NewInternal(ctx, err, "gateway: error moving resource on webdav host: "+p),
				}, nil
			}
			return &provider.MoveResponse{
				Status: status.NewOK(ctx),
			}, nil
		}

		src := &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join(ri.Path, shareChild),
			},
		}
		dst := &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join(ri.Path, dshareChild),
			},
		}

		req.Source = src
		req.Destination = dst

		return s.move(ctx, req)
	}

	return &provider.MoveResponse{
		Status: status.NewInternal(ctx, errors.New("gateway: move called on unknown path: "+p), ""),
	}, nil
}

func (s *svc) move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	srcP, err := s.findProvider(ctx, req.Source)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.MoveResponse{
				Status: status.NewNotFound(ctx, "source storage provider not found"),
			}, nil
		}
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	dstP, err := s.findProvider(ctx, req.Destination)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.MoveResponse{
				Status: status.NewNotFound(ctx, "destination storage provider not found"),
			}, nil
		}
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	// if providers are not the same we do not implement cross storage copy yet.
	if srcP.Address != dstP.Address {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	c, err := s.getStorageProviderClient(ctx, srcP)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error connecting to storage provider="+srcP.Address),
		}, nil
	}

	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.SetArbitraryMetadataResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.SetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.SetArbitraryMetadata(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Stat")
	}

	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.UnsetArbitraryMetadataResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.UnsetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.UnsetArbitraryMetadata(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Stat")
	}

	return res, nil
}

func (s *svc) stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.StatResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	return c.Stat(ctx, req)
}

func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref, req.ArbitraryMetadataKeys...)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.StatResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error during STAT id: %v", req.Ref.GetId())
		return &provider.StatResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) {
		return s.stat(ctx, req)
	}

	// TODO(labkode): we need to generate a unique etag based on the contained share names.
	if s.isSharedFolder(ctx, p) {
		return s.stat(ctx, req)
	}

	// we need to provide the info of the target, not the reference.
	if s.isShareName(ctx, p) {
		statRes, err := s.stat(ctx, req)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.StatResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error stating")
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.StatResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error resolving reference: "+p),
			}, nil
		}

		if protocol == "webdav" {
			ri, err = s.webdavRefStat(ctx, statRes.Info.Target)
			if err != nil {
				return &provider.StatResponse{
					Status: status.NewInternal(ctx, err, "gateway: error resolving webdav reference: "+p),
				}, nil
			}
		}

		// we need to make sure we don't expose the reference target in the resource
		// information. For example, if requests comes to: /home/MyShares/photos and photos
		// is reference to /user/peter/Holidays/photos, we need to still return to the user
		// /home/MyShares/photos
		orgPath := statRes.Info.Path
		statRes.Info = ri
		statRes.Info.Path = orgPath
		return statRes, nil

	}

	if s.isShareChild(ctx, p) {
		shareName, shareChild := s.splitShare(ctx, p)

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: shareName,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.StatResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error stating")
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.StatResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			log.Err(err).Msg("gateway: error resolving reference")
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "error stating"),
			}, nil
		}

		if protocol == "webdav" {
			ri, err = s.webdavRefStat(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &provider.StatResponse{
					Status: status.NewInternal(ctx, err, "gateway: error resolving webdav reference: "+p),
				}, nil
			}
			ri.Path = p
			return &provider.StatResponse{
				Status: status.NewOK(ctx),
				Info:   ri,
			}, nil
		}

		// append child to target
		req.Ref = &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join(ri.Path, shareChild),
			},
		}
		res, err := s.stat(ctx, req)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
			}, nil
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.StatResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(res.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error stating")
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
			}, nil
		}

		// we need to make sure we don't expose the reference target in the resource
		// information.
		res.Info.Path = p
		return res, nil
	}

	panic("gateway: stating an unknown path:" + p)
}

func (s *svc) checkRef(ctx context.Context, ri *provider.ResourceInfo) (*provider.ResourceInfo, string, error) {
	if ri.Type != provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		panic("gateway: calling checkRef on a non reference type:" + ri.String())
	}

	// reference types MUST have a target resource id.
	if ri.Target == "" {
		err := errors.New("gateway: ref target is an empty uri")
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
		err := errors.New("gateway: no reference handler for scheme: " + uri.Scheme)
		return nil, "", err
	}
}

func (s *svc) handleCS3Ref(ctx context.Context, opaque string) (*provider.ResourceInfo, error) {
	// a cs3 ref has the following layout: <storage_id>/<opaque_id>
	parts := strings.SplitN(opaque, "/", 2)
	if len(parts) < 2 {
		err := errors.New("gateway: cs3 ref does not follow the layout storageid/opaqueid:" + opaque)
		return nil, err
	}

	// we could call here the Stat method again, but that is calling for problems in case
	// there is a loop of targets pointing to targets, so better avoid it.

	req := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: &provider.ResourceId{
					StorageId: parts[0],
					OpaqueId:  parts[1],
				},
			},
		},
	}
	res, err := s.stat(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling stat")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound(req.Ref.String())
		}
		err := errors.New("gateway: error stating target reference")
		return nil, err
	}

	if res.Info.Type == provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		err := errors.New("gateway: error the target of a reference cannot be another reference")
		return nil, err
	}

	return res.Info, nil
}

func (s *svc) ListContainerStream(_ *provider.ListContainerStreamRequest, _ gateway.GatewayAPI_ListContainerStreamServer) error {
	return errors.New("Unimplemented")
}

func (s *svc) listContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.ListContainerResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.ListContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListContainer")
	}

	return res, nil
}

func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref, req.ArbitraryMetadataKeys...)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.ListContainerResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		log.Error().Str("rpc_code", st.Code.String()).
			Msgf("error listing directory id: %v", req.Ref.GetId())
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) {
		return s.listContainer(ctx, req)
	}

	if s.isSharedFolder(ctx, p) {
		// TODO(labkode): we need to generate a unique etag if any of the underlying share changes.
		// the response will contain all the share names and we need to convert them to non reference types.
		lcr, err := s.listContainer(ctx, req)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error listing shared folder:"+req.Ref.String()),
			}, nil
		}

		for i, ref := range lcr.Infos {
			info, protocol, err := s.checkRef(ctx, ref)
			if err != nil {
				if _, ok := err.(errtypes.IsNotFound); ok {
					return &provider.ListContainerResponse{
						Status: status.NewNotFound(ctx, "gateway: reference not found:"+ref.Target),
					}, nil
				}
				return &provider.ListContainerResponse{
					Status: status.NewInternal(ctx, err, "gateway: error resolving reference:"+ref.Path),
				}, nil
			}

			if protocol == "webdav" {
				info, err = s.webdavRefStat(ctx, ref.Target)
				if err != nil {
					// Might be the case that the webdav token has expired. In that case, use the reference's info
					info = ref
				}
			}

			base := path.Base(ref.Path)
			info.Path = path.Join(p, base)
			lcr.Infos[i] = info
		}
		return lcr, nil
	}

	// we need to provide the info of the target, not the reference.
	if s.isShareName(ctx, p) {
		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: p,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating share:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.ListContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: file not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error stating")
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating share:"+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.ListContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statRes.Info.Target),
				}, nil
			}
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error resolving reference:"+p),
			}, nil
		}

		if protocol == "webdav" {
			infos, err := s.webdavRefLs(ctx, statRes.Info.Target)
			if err != nil {
				return &provider.ListContainerResponse{
					Status: status.NewInternal(ctx, err, "gateway: error listing webdav reference: "+p),
				}, nil
			}

			for _, info := range infos {
				base := path.Base(info.Path)
				info.Path = path.Join(p, base)
			}
			return &provider.ListContainerResponse{
				Status: status.NewOK(ctx),
				Infos:  infos,
			}, nil
		}

		if ri.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			err := errtypes.NotSupported("gateway: list container: cannot list non-container type:" + ri.Path)
			log.Err(err).Msg("gateway: error listing")
			return &provider.ListContainerResponse{
				Status: status.NewInvalidArg(ctx, "resource is not a container"),
			}, nil
		}

		newReq := &provider.ListContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: ri.Path,
				},
			},
			ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
		}
		newRes, err := s.listContainer(ctx, newReq)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error listing "+newReq.Ref.String()),
			}, nil
		}

		if newRes.Status.Code != rpc.Code_CODE_OK {
			if newRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.ListContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: container not found:"+newReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(newRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error listing")
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error listing "+newReq.Ref.String()),
			}, nil
		}

		// paths needs to be converted
		for _, info := range newRes.Infos {
			base := path.Base(info.Path)
			info.Path = path.Join(p, base)
		}

		return newRes, nil

	}

	if s.isShareChild(ctx, p) {
		shareName, shareChild := s.splitShare(ctx, p)

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: shareName,
				},
			},
		}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating share child "+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.ListContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: container not found:"+statReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error listing")
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating share child "+statReq.Ref.String()),
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &provider.ListContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: reference not found:"+statReq.Ref.String()),
				}, nil
			}
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error resolving reference:"+p),
			}, nil
		}

		if protocol == "webdav" {
			infos, err := s.webdavRefLs(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &provider.ListContainerResponse{
					Status: status.NewInternal(ctx, err, "gateway: error listing webdav reference: "+p),
				}, nil
			}

			for _, info := range infos {
				base := path.Base(info.Path)
				info.Path = path.Join(shareName, shareChild, base)
			}
			return &provider.ListContainerResponse{
				Status: status.NewOK(ctx),
				Infos:  infos,
			}, nil
		}

		if ri.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			err := errtypes.NotSupported("gateway: list container: cannot list non-container type:" + ri.Path)
			log.Err(err).Msg("gateway: error listing")
			return &provider.ListContainerResponse{
				Status: status.NewInvalidArg(ctx, "resource is not a container"),
			}, nil
		}

		newReq := &provider.ListContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: path.Join(ri.Path, shareChild),
				},
			},
			ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
		}
		newRes, err := s.listContainer(ctx, newReq)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error listing "+newReq.Ref.String()),
			}, nil
		}

		if newRes.Status.Code != rpc.Code_CODE_OK {
			if newRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				return &provider.ListContainerResponse{
					Status: status.NewNotFound(ctx, "gateway: container not found:"+newReq.Ref.String()),
				}, nil
			}
			err := status.NewErrorFromCode(newRes.Status.Code, "gateway")
			log.Err(err).Msg("gateway: error listing")
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error listing "+newReq.Ref.String()),
			}, nil
		}

		// paths needs to be converted
		for _, info := range newRes.Infos {
			base := path.Base(info.Path)
			info.Path = path.Join(shareName, shareChild, base)
		}

		return newRes, nil

	}

	panic("gateway: stating an unknown path:" + p)
}

func (s *svc) getPath(ctx context.Context, ref *provider.Reference, keys ...string) (string, *rpc.Status) {
	if ref.GetPath() != "" {
		return ref.GetPath(), &rpc.Status{Code: rpc.Code_CODE_OK}
	}

	if ref.GetId() != nil && ref.GetId().GetOpaqueId() != "" {
		req := &provider.StatRequest{Ref: ref, ArbitraryMetadataKeys: keys}
		res, err := s.stat(ctx, req)
		if (res != nil && res.Status.Code != rpc.Code_CODE_OK) || err != nil {
			return "", res.Status
		}

		return res.Info.Path, res.Status
	}

	return "", &rpc.Status{Code: rpc.Code_CODE_INTERNAL}
}

// /home/MyShares/
func (s *svc) isSharedFolder(ctx context.Context, p string) bool {
	return s.split(ctx, p, 2)
}

// /home/MyShares/photos/
func (s *svc) isShareName(ctx context.Context, p string) bool {
	return s.split(ctx, p, 3)
}

// /home/MyShares/photos/Ibiza/beach.png
func (s *svc) isShareChild(ctx context.Context, p string) bool {
	return s.split(ctx, p, 4)
}

// always validate that the path contains the share folder
// split cannot be called with i<2
func (s *svc) split(ctx context.Context, p string, i int) bool {
	log := appctx.GetLogger(ctx)
	if i < 2 {
		panic("split called with i < 2")
	}

	parts := s.splitPath(ctx, p)

	// validate that we have always at least two elements
	if len(parts) < 2 {
		panic(fmt.Sprintf("split: len(parts) < 2: path:%s parts:%+v", p, parts))
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
// should be called after checking isShareChild == true
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

func (s *svc) getSharedFolder(ctx context.Context) string {
	home := s.getHome(ctx)
	shareFolder := path.Join(home, s.c.ShareFolder)
	return shareFolder
}

func (s *svc) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.ListFileVersionsResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
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
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.RestoreFileVersionResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreFileVersion")
	}

	return res, nil
}

func (s *svc) ListRecycleStream(_ *gateway.ListRecycleStreamRequest, _ gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errors.New("Unimplemented")
}

// TODO use the ListRecycleRequest.Ref to only list the trash of a specific storage
func (s *svc) ListRecycle(ctx context.Context, req *gateway.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	c, err := s.find(ctx, req.GetRef())
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.ListRecycleResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.ListRecycleResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.ListRecycle(ctx, &provider.ListRecycleRequest{
		Opaque: req.Opaque,
		FromTs: req.FromTs,
		ToTs:   req.ToTs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListRecycleRequest")
	}

	return res, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.RestoreRecycleItemResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreRecycleItem")
	}

	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *gateway.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	// lookup storage by treating the key as a path. It has been prefixed with the storage path in ListRecycle
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.PurgeRecycleResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.PurgeRecycleResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{
		Opaque: req.GetOpaque(),
		Ref:    req.GetRef(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling PurgeRecycle")
	}
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, _ *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	res := &provider.GetQuotaResponse{
		Status: status.NewUnimplemented(ctx, nil, "GetQuota not yet implemented"),
	}
	return res, nil
}

func (s *svc) findByID(ctx context.Context, id *provider.ResourceId) (provider.ProviderAPIClient, error) {
	ref := &provider.Reference{
		Spec: &provider.Reference_Id{
			Id: id,
		},
	}
	return s.find(ctx, ref)
}

func (s *svc) findByPath(ctx context.Context, path string) (provider.ProviderAPIClient, error) {
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: path,
		},
	}
	return s.find(ctx, ref)
}

func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, error) {
	p, err := s.findProvider(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.getStorageProviderClient(ctx, p)
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return c, nil
}

func (s *svc) findProvider(ctx context.Context, ref *provider.Reference) (*registry.ProviderInfo, error) {
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting storage registry client")
		return nil, err
	}

	res, err := c.GetStorageProvider(ctx, &registry.GetStorageProviderRequest{
		Ref: ref,
	})

	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetStorageProvider")
		return nil, err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound("gateway: storage provider not found for reference:" + ref.String())
		}
		err := status.NewErrorFromCode(res.Status.Code, "gateway")
		return nil, err
	}

	if res.Provider == nil {
		err := errors.New("gateway: provider is nil")
		return nil, err
	}

	return res.Provider, nil
}
