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

package ocmincoming

// This package implements the core OCM API for receiving external shares from remote EFSS systems.

import (
	"context"
	"errors"
	"fmt"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmincoming", New)
	plugin.RegisterNamespace("grpc.services.ocmincoming.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                    `mapstructure:"driver"`
	Drivers map[string]map[string]any `mapstructure:"drivers"`
	// AllowVerifiedEmptyRecipientIdp requires a qualified local directory and authentication source.
	AllowVerifiedEmptyRecipientIdp bool `mapstructure:"allow_verified_empty_recipient_idp"`
}

type service struct {
	conf config
	repo share.Repository
}

type resolvedRecipientStore interface {
	StoreReceivedShareForResolvedRecipient(context.Context, *ocm.ReceivedShare, *userpb.UserId) (*ocm.ReceivedShare, error)
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (s *service) Register(ss *grpc.Server) {
	ocmincoming.RegisterOcmIncomingAPIServer(ss, s)
}

func getShareRepository(ctx context.Context, c *config) (share.Repository, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound(fmt.Sprintf("driver not found: %s", c.Driver))
}

// New creates a new ocm core svc.
func New(ctx context.Context, m map[string]any) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	repo, err := getShareRepository(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: c,
		repo: repo,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.ocm.incoming.v1beta1.OcmIncomingAPI/CreateOCMIncomingShare"}
}

// CreateOCMIncomingShare is called when a remote OCM request comes into this reva instance.
func (s *service) CreateOCMIncomingShare(ctx context.Context, req *ocmincoming.CreateOCMIncomingShareRequest) (*ocmincoming.CreateOCMIncomingShareResponse, error) {
	if req.RecipientType != ocm.RecipientType_RECIPIENT_TYPE_USER {
		return nil, errtypes.NotSupported("share type not supported")
	}
	origin, err := share.ReceivedGrantOrigin(req.GetSender())
	if err != nil {
		return &ocmincoming.CreateOCMIncomingShareResponse{
			Status: status.NewInvalidArg(ctx, "invalid provider or remote share ID"),
		}, nil
	}

	now := &typesv1beta1.Timestamp{
		Seconds: uint64(time.Now().Unix()),
	}

	received := &ocm.ReceivedShare{
		RemoteShareId: req.ResourceId,
		Name:          req.Name,
		Grantee: &providerpb.Grantee{
			Type: providerpb.GranteeType_GRANTEE_TYPE_USER,
			Id: &providerpb.Grantee_UserId{
				UserId: req.ShareWith,
			},
		},
		SharedResourceType: req.SharedResourceType,
		RecipientType:      req.RecipientType,
		Owner:              req.Owner,
		Creator:            req.Sender,
		Protocols:          req.Protocols,
		Ctime:              now,
		Mtime:              now,
		Expiration:         req.Expiration,
		State:              ocm.ShareState_SHARE_STATE_PENDING,
	}
	var stored *ocm.ReceivedShare
	recipient := req.GetShareWith()
	if repo, ok := s.repo.(resolvedRecipientStore); ok && s.conf.AllowVerifiedEmptyRecipientIdp && recipient.GetIdp() == "" && recipient.GetOpaqueId() != "" {
		if _, err := share.GrantKey(origin, req.GetResourceId()); err != nil {
			return &ocmincoming.CreateOCMIncomingShareResponse{Status: status.NewInvalidArg(ctx, "invalid OCM grant identity")}, nil
		}
		resolved, failure := resolveRecipient(ctx, recipient)
		if failure != nil {
			return &ocmincoming.CreateOCMIncomingShareResponse{Status: failure}, nil
		}
		stored, err = repo.StoreReceivedShareForResolvedRecipient(ctx, received, resolved)
	} else {
		stored, err = s.repo.StoreReceivedShare(ctx, received)
	}
	if err != nil {
		if errors.Is(err, share.ErrInvalidGrantIdentity) {
			return &ocmincoming.CreateOCMIncomingShareResponse{
				Status: status.NewInvalidArg(ctx, "invalid OCM grant identity"),
			}, nil
		}
		// TODO: identify errors
		return &ocmincoming.CreateOCMIncomingShareResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, nil
	}

	return &ocmincoming.CreateOCMIncomingShareResponse{
		Status:  status.NewOK(ctx),
		Id:      stored.Id.OpaqueId,
		Created: stored.Ctime,
	}, nil
}

func resolveRecipient(ctx context.Context, recipient *userpb.UserId) (*userpb.UserId, *rpc.Status) {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const unavailable = "local recipient directory lookup failed"
	const unverified = "local recipient identity could not be verified"
	directory, err := revaservice.UserProvider(lookupCtx)
	if err != nil {
		return nil, status.NewInternal(ctx, errors.New(unavailable), unavailable)
	}
	res, err := directory.GetUserByClaim(lookupCtx, &userpb.GetUserByClaimRequest{
		Claim: "userid", Value: recipient.GetOpaqueId(), SkipFetchingUserGroups: true,
	})
	if err != nil || res.GetStatus() == nil {
		return nil, status.NewInternal(ctx, errors.New(unavailable), unavailable)
	}
	switch res.GetStatus().GetCode() {
	case rpc.Code_CODE_OK:
		id := res.GetUser().GetId()
		if id.GetOpaqueId() == recipient.GetOpaqueId() && id.GetIdp() == recipient.GetIdp() &&
			id.GetTenantId() == recipient.GetTenantId() && id.GetType() == recipient.GetType() {
			return id, nil
		}
	case rpc.Code_CODE_NOT_FOUND, rpc.Code_CODE_ABORTED, rpc.Code_CODE_UNIMPLEMENTED:
	default:
		return nil, status.NewInternal(ctx, errors.New(unavailable), unavailable)
	}
	return nil, status.NewFailedPrecondition(ctx, nil, unverified)
}

func (s *service) UpdateOCMIncomingShare(ctx context.Context, req *ocmincoming.UpdateOCMIncomingShareRequest) (*ocmincoming.UpdateOCMIncomingShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}

func (s *service) DeleteOCMIncomingShare(ctx context.Context, req *ocmincoming.DeleteOCMIncomingShareRequest) (*ocmincoming.DeleteOCMIncomingShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}
