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

package gateway

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

// TODO(labkode): add multi-phase commit logic when commit share or commit ref is enabled.
func (s *svc) CreateOCMShare(ctx context.Context, req *ocm.CreateOCMShareRequest) (*ocm.CreateOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.CreateOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateShare")
	}

	// if we don't need to commit we return earlier
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	if s.c.CommitShareToStorageGrant {
		addGrantStatus, err := s.addGrant(ctx, req.ResourceId, req.Grant.Grantee, req.Grant.Permissions.Permissions)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error adding OCM grant to storage")
		}
		if addGrantStatus.Code != rpc.Code_CODE_OK {
			return &ocm.CreateOCMShareResponse{
				Status: addGrantStatus,
			}, err
		}
	}

	return res, nil
}

func (s *svc) RemoveOCMShare(ctx context.Context, req *ocm.RemoveOCMShareRequest) (*ocm.RemoveOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		return &ocm.RemoveOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	// if we need to commit the share, we need the resource it points to.
	var share *ocm.Share
	if s.c.CommitShareToStorageGrant {
		getShareReq := &ocm.GetOCMShareRequest{
			Ref: req.Ref,
		}
		getShareRes, err := c.GetOCMShare(ctx, getShareReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling GetShare")
		}

		if getShareRes.Status.Code != rpc.Code_CODE_OK {
			res := &ocm.RemoveOCMShareResponse{
				Status: status.NewInternal(ctx, status.NewErrorFromCode(getShareRes.Status.Code, "gateway"),
					"error getting share when committing to the storage"),
			}
			return res, nil
		}
		share = getShareRes.Share
	}

	res, err := c.RemoveOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RemoveShare")
	}

	// if we don't need to commit we return earlier
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	if s.c.CommitShareToStorageGrant {
		removeGrantStatus, err := s.removeGrant(ctx, share.ResourceId, share.Grantee, share.Permissions.Permissions)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error removing OCM grant from storage")
		}
		if removeGrantStatus.Code != rpc.Code_CODE_OK {
			return &ocm.RemoveOCMShareResponse{
				Status: removeGrantStatus,
			}, err
		}
	}

	return res, nil
}

// TODO(labkode): we need to validate share state vs storage grant and storage ref
// If there are any inconsistencies, the share needs to be flag as invalid and a background process
// or active fix needs to be performed.
func (s *svc) GetOCMShare(ctx context.Context, req *ocm.GetOCMShareRequest) (*ocm.GetOCMShareResponse, error) {
	return s.getOCMShare(ctx, req)
}

func (s *svc) getOCMShare(ctx context.Context, req *ocm.GetOCMShareRequest) (*ocm.GetOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.GetOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.GetOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetShare")
	}

	return res, nil
}

// TODO(labkode): read GetShare comment.
func (s *svc) ListOCMShares(ctx context.Context, req *ocm.ListOCMSharesRequest) (*ocm.ListOCMSharesResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.ListOCMSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.ListOCMShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListShares")
	}

	return res, nil
}

func (s *svc) UpdateOCMShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) (*ocm.UpdateOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.UpdateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateShare")
	}

	return res, nil
}

func (s *svc) ListReceivedOCMShares(ctx context.Context, req *ocm.ListReceivedOCMSharesRequest) (*ocm.ListReceivedOCMSharesResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.ListReceivedOCMSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.ListReceivedOCMShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListReceivedShares")
	}

	return res, nil
}

func (s *svc) UpdateReceivedOCMShare(ctx context.Context, req *ocm.UpdateReceivedOCMShareRequest) (*ocm.UpdateReceivedOCMShareResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateReceivedOCMShare(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	// if we don't need to create/delete references then we return early.
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// properties are updated in the order they appear in the field mask
	// when an error occurs the request ends and no further fields are updated
	for i := range req.UpdateMask.Paths {
		switch req.UpdateMask.Paths[i] {
		case "state":
			switch req.GetShare().GetState() {
			case ocm.ShareState_SHARE_STATE_ACCEPTED:
				getShareReq := &ocm.GetReceivedOCMShareRequest{
					Ref: &ocm.ShareReference{
						Spec: &ocm.ShareReference_Id{
							Id: req.Share.Share.Id,
						},
					},
				}
				getShareRes, err := s.GetReceivedOCMShare(ctx, getShareReq)
				if err != nil {
					log.Err(err).Msg("gateway: error calling GetReceivedShare")
					return &ocm.UpdateReceivedOCMShareResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_INTERNAL,
						},
					}, nil
				}

				if getShareRes.Status.Code != rpc.Code_CODE_OK {
					log.Error().Msg("gateway: error calling GetReceivedShare")
					return &ocm.UpdateReceivedOCMShareResponse{
						Status: &rpc.Status{
							Code: rpc.Code_CODE_INTERNAL,
						},
					}, nil
				}

				share := getShareRes.Share
				if share == nil {
					panic("gateway: error updating a received share: the share is nil")
				}

				if share.GetShare().ShareType == ocm.Share_SHARE_TYPE_TRANSFER {
					srcIdp := share.GetShare().GetOwner().GetIdp()
					meshProvider, err := s.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
						Domain: srcIdp,
					})
					if err != nil {
						log.Err(err).Msg("gateway: error calling GetInfoByDomain")
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
						}, nil
					}
					var srcServiceHost string
					var srcEndpointPath string
					// target URI scheme will be the webdav endpoint scheme
					var srcEndpointScheme string
					for _, s := range meshProvider.ProviderInfo.Services {
						if strings.ToLower(s.Endpoint.Type.Name) == "webdav" {
							endpointURL, err := url.Parse(s.Endpoint.Path)
							if err != nil {
								log.Err(err).Msg("gateway: error calling UpdateReceivedShare: unable to parse webdav endpoint " + s.Endpoint.Path)
								return &ocm.UpdateReceivedOCMShareResponse{
									Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
								}, nil
							}
							urlServiceHostFull, err := url.Parse(s.Host)
							if err != nil {
								log.Err(err).Msg("gateway: error calling UpdateReceivedShare: unable to parse webdav service host " + s.Host)
								return &ocm.UpdateReceivedOCMShareResponse{
									Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
								}, nil
							}
							srcServiceHost = urlServiceHostFull.Host + urlServiceHostFull.Path
							// optional prefix must only appear in target url path:
							// http://...token...@reva.eu/prefix/?name=remote.php/webdav/home/...
							srcEndpointPath = strings.TrimPrefix(endpointURL.Path, urlServiceHostFull.Path)
							srcEndpointScheme = endpointURL.Scheme
							break
						}
					}

					var srcToken string
					srcTokenOpaque, ok := share.GetShare().Grantee.Opaque.Map["token"]
					if !ok {
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: status.NewNotFound(ctx, "token not found"),
						}, nil
					}
					switch srcTokenOpaque.Decoder {
					case "plain":
						srcToken = string(srcTokenOpaque.Value)
					default:
						err := errtypes.NotSupported("opaque entry decoder not recognized: " + srcTokenOpaque.Decoder)
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: status.NewInternal(ctx, err, "error updating received share"),
						}, nil
					}

					srcPath := path.Join(srcEndpointPath, share.GetShare().Name)
					srcTargetURI := fmt.Sprintf("%s://%s@%s?name=%s", srcEndpointScheme, srcToken, srcServiceHost, srcPath)

					// get the webdav endpoint of the grantee's idp
					var granteeIdp string
					if share.GetShare().GetGrantee().Type == provider.GranteeType_GRANTEE_TYPE_USER {
						granteeIdp = share.GetShare().GetGrantee().GetUserId().Idp
					}
					if share.GetShare().GetGrantee().Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
						granteeIdp = share.GetShare().GetGrantee().GetGroupId().Idp
					}
					destWebdavEndpoint, err := s.getWebdavEndpoint(ctx, granteeIdp)
					if err != nil {
						log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
						}, nil
					}
					destWebdavEndpointURL, err := url.Parse(destWebdavEndpoint)
					if err != nil {
						log.Err(err).Msg("gateway: error calling UpdateReceivedShare: unable to parse webdav endpoint " + destWebdavEndpoint)
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
						}, nil
					}
					destWebdavHost, err := s.getWebdavHost(ctx, granteeIdp)
					if err != nil {
						log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
						}, nil
					}
					urlServiceHostFull, err := url.Parse(destWebdavHost)
					if err != nil {
						log.Err(err).Msg("gateway: error calling UpdateReceivedShare: unable to parse webdav service host " + destWebdavHost)
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
						}, nil
					}
					destServiceHost := urlServiceHostFull.Host + urlServiceHostFull.Path
					// optional prefix must only appear in target url path:
					// http://...token...@reva.eu/prefix/?name=remote.php/webdav/home/...
					destEndpointPath := strings.TrimPrefix(destWebdavEndpointURL.Path, urlServiceHostFull.Path)
					destEndpointScheme := destWebdavEndpointURL.Scheme
					destToken := ctxpkg.ContextMustGetToken(ctx)
					homeRes, err := s.GetHome(ctx, &provider.GetHomeRequest{})
					if err != nil {
						log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL},
						}, nil
					}
					destPath := path.Join(destEndpointPath, homeRes.Path, s.c.DataTransfersFolder, path.Base(share.GetShare().Name))
					destTargetURI := fmt.Sprintf("%s://%s@%s?name=%s", destEndpointScheme, destToken, destServiceHost, destPath)

					opaqueObj := &types.Opaque{
						Map: map[string]*types.OpaqueEntry{
							"shareId": {
								Decoder: "plain",
								Value:   []byte(share.GetShare().GetId().OpaqueId),
							},
						},
					}
					req := &datatx.PullTransferRequest{
						SrcTargetUri:  srcTargetURI,
						DestTargetUri: destTargetURI,
						Opaque:        opaqueObj,
					}
					res, err := s.PullTransfer(ctx, req)
					if err != nil {
						log.Err(err).Msg("gateway: error calling PullTransfer")
						return &ocm.UpdateReceivedOCMShareResponse{
							Status: &rpc.Status{
								Code: rpc.Code_CODE_INTERNAL,
							},
						}, err
					}

					log.Info().Msgf("gateway: PullTransfer: %v", res.TxInfo)

					// do not create an OCM reference, just return
					return &ocm.UpdateReceivedOCMShareResponse{
						Status: status.NewOK(ctx),
					}, nil
				}

				createRefStatus, err := s.createOCMReference(ctx, share.Share)
				return &ocm.UpdateReceivedOCMShareResponse{
					Status: createRefStatus,
				}, err
			case ocm.ShareState_SHARE_STATE_REJECTED:
				s.removeReference(ctx, req.GetShare().GetShare().ResourceId) // error is logged inside removeReference
				// FIXME we are ignoring an error from removeReference here
				return res, nil
			}
		case "mount_point":
			// TODO(labkode): implementing updating mount point
			err = errtypes.NotSupported("gateway: update of mount point is not yet implemented")
			return &ocm.UpdateReceivedOCMShareResponse{
				Status: status.NewUnimplemented(ctx, err, "error updating received share"),
			}, nil
		default:
			return nil, errtypes.NotSupported("updating " + req.UpdateMask.Paths[i] + " is not supported")
		}
	}
	return res, nil
}

func (s *svc) GetReceivedOCMShare(ctx context.Context, req *ocm.GetReceivedOCMShareRequest) (*ocm.GetReceivedOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(pool.Endpoint(s.c.OCMShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.GetReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.GetReceivedOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetReceivedShare")
	}

	return res, nil
}

func (s *svc) createOCMReference(ctx context.Context, share *ocm.Share) (*rpc.Status, error) {

	log := appctx.GetLogger(ctx)

	var token string
	tokenOpaque, ok := share.Grantee.Opaque.Map["token"]
	if !ok {
		return status.NewNotFound(ctx, "token not found"), nil
	}
	switch tokenOpaque.Decoder {
	case "plain":
		token = string(tokenOpaque.Value)
	default:
		err := errtypes.NotSupported("opaque entry decoder not recognized: " + tokenOpaque.Decoder)
		return status.NewInternal(ctx, err, "invalid opaque entry decoder"), nil
	}

	homeRes, err := s.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		err := errors.Wrap(err, "gateway: error calling GetHome")
		return status.NewInternal(ctx, err, "error updating received share"), nil
	}

	var refPath, targetURI string
	// reference path is the home path + some name on the corresponding
	// mesh provider (/home/MyShares/x)
	// It is the responsibility of the gateway to resolve these references and merge the response back
	// from the main request.
	refPath = path.Join(homeRes.Path, s.c.ShareFolder, path.Base(share.Name))
	// webdav is the scheme, token@host the opaque part and the share name the query of the URL.
	targetURI = fmt.Sprintf("webdav://%s@%s?name=%s", token, share.Creator.Idp, share.Name)

	log.Info().Msg("mount path will be:" + refPath)
	createRefReq := &provider.CreateReferenceRequest{
		Ref:       &provider.Reference{Path: refPath},
		TargetUri: targetURI,
	}

	c, err := s.findByPath(ctx, refPath)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found"), nil
		}
		return status.NewInternal(ctx, err, "error finding storage provider"), nil
	}

	createRefRes, err := c.CreateReference(ctx, createRefReq)
	if err != nil {
		log.Err(err).Msg("gateway: error calling GetHome")
		return &rpc.Status{
			Code: rpc.Code_CODE_INTERNAL,
		}, nil
	}

	if createRefRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(createRefRes.Status.GetCode(), "gateway")
		return status.NewInternal(ctx, err, "error updating received share"), nil
	}

	return status.NewOK(ctx), nil
}
