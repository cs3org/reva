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

package usershareprovidersvc

import (
	"fmt"
	"io"
	"strings"

	"github.com/cs3org/reva/cmd/revad/grpcserver"

	"context"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.Register("usershareprovidersvc", New)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	conf    *config
	storage storage.FS
}

func (s *service) Close() error {
	return s.storage.Shutdown()
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new user share provider svc
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	fs, err := getFS(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:    c,
		storage: fs,
	}

	usershareproviderv0alphapb.RegisterUserShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreateShare(ctx context.Context, req *usershareproviderv0alphapb.CreateShareRequest) (*usershareproviderv0alphapb.CreateShareResponse, error) {
	log := appctx.GetLogger(ctx)

	path := req.ResourceId.OpaqueId
	grant := &storage.Grant{
		Grantee: &storage.Grantee{
			UserID: &user.ID{
				// IDP TODO ?
				OpaqueID: req.Grant.Grantee.Id.OpaqueId,
			},
			Type: storage.GranteeTypeUser, // TODO hardcoded, read from share
		},
		PermissionSet: &storage.PermissionSet{
			ListContainer:   req.Grant.Permissions.Permissions.ListContainer,
			CreateContainer: req.Grant.Permissions.Permissions.CreateContainer,
			Move:            req.Grant.Permissions.Permissions.Move,
			Delete:          req.Grant.Permissions.Permissions.Delete,
			// TODO map more permissions
		},
	}

	log.Debug().Str("path", path).Msg("list shares")
	// check if path exists
	err := s.storage.AddGrant(ctx, path, grant)
	if err != nil {
		return nil, err
	}
	share := grantToShare(grant)

	res := &usershareproviderv0alphapb.CreateShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_OK,
		},
		Share: share,
	}
	return res, nil
}

func (s *service) RemoveShare(ctx context.Context, req *usershareproviderv0alphapb.RemoveShareRequest) (*usershareproviderv0alphapb.RemoveShareResponse, error) {
	res := &usershareproviderv0alphapb.RemoveShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) GetShare(ctx context.Context, req *usershareproviderv0alphapb.GetShareRequest) (*usershareproviderv0alphapb.GetShareResponse, error) {
	//log := appctx.GetLogger(ctx)
	ref := req.Ref.GetId()
	if ref == nil {
		res := &usershareproviderv0alphapb.GetShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_UNIMPLEMENTED,
			},
		}
		return res, nil
	}
	id := ref.OpaqueId

	// TODO split at @ ... encode parts as base64 / something url compatible
	sp := strings.Split(id, "@")
	sID, path := sp[0], sp[1]

	md, err := s.storage.GetMD(ctx, path)
	if err != nil {
		// TODO not found
		return nil, err
	}

	// TODO the storage has no method to get a grand by shareid
	grants, err := s.storage.ListGrants(ctx, path)
	if err != nil {
		// TODO not found
		return nil, err
	}
	res := &usershareproviderv0alphapb.GetShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_NOT_FOUND,
		},
	}
	for _, grant := range grants {
		share := grantToShare(grant)
		if share.Id.OpaqueId == sID {
			share.ResourceId = &storageproviderv0alphapb.ResourceId{
				StorageId: "TODO", // we need to lookup the resource id
				OpaqueId:  path,
			}
			// TODO check this kind of id works not only for acls ...
			share.Id.OpaqueId = share.Id.OpaqueId + "@" + share.ResourceId.OpaqueId
			// the owner is the file owner, which is the same for all shares in this case
			// share.Owner = md.? // TODO how do we get the owner? for eos it might be in the opaque metadata, no .. by asking the broker for the owner?
			share.Mtime = &typespb.Timestamp{
				Seconds: md.Mtime.Seconds,
				Nanos:   md.Mtime.Nanos,
			}
			res.Status.Code = rpcpb.Code_CODE_OK
			res.Share = share
			break
		}
	}

	return res, nil
}

func (s *service) ListShares(ctx context.Context, req *usershareproviderv0alphapb.ListSharesRequest) (*usershareproviderv0alphapb.ListSharesResponse, error) {
	log := appctx.GetLogger(ctx)

	shares := []*usershareproviderv0alphapb.Share{}

	for _, filter := range req.Filters {
		if filter.Type == usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID {
			path := filter.GetResourceId().OpaqueId
			log.Debug().Str("path", path).Msg("list shares")
			// check if path exists
			md, err := s.storage.GetMD(ctx, path)
			if err != nil {
				return nil, err
			}

			grants, err := s.storage.ListGrants(ctx, path)
			if err != nil {
				return nil, err
			}
			for _, grant := range grants {
				share := grantToShare(grant)
				share.ResourceId = filter.GetResourceId()
				// TODO check this kind of id works not only for acls ...
				share.Id.OpaqueId = share.Id.OpaqueId + "@" + share.ResourceId.OpaqueId
				// the owner is the file owner, which is the same for all shares in this case
				// share.Owner = md.? // TODO how do we get the owner? for eos it might be in the opaque metadata, no .. by asking the broker for the owner?
				share.Mtime = &typespb.Timestamp{Seconds: md.Mtime.Seconds, Nanos: md.Mtime.Nanos}
				shares = append(shares, share)
			}
		}
	}
	// TODO list all shares
	res := &usershareproviderv0alphapb.ListSharesResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_OK,
		},
		Shares: shares,
	}
	return res, nil
}

func grantToShare(grant *storage.Grant) *usershareproviderv0alphapb.Share {
	share := &usershareproviderv0alphapb.Share{
		Id: &usershareproviderv0alphapb.ShareId{},
		// ResourceId: not available in grant, set in parent
		Permissions: &usershareproviderv0alphapb.SharePermissions{
			Permissions: &storageproviderv0alphapb.ResourcePermissions{
				ListContainer:   grant.PermissionSet.ListContainer,
				CreateContainer: grant.PermissionSet.CreateContainer,
				Move:            grant.PermissionSet.Move,
				Delete:          grant.PermissionSet.Delete,
				// TODO add more permissons to grant.PermissionSet
			},
		},
		Grantee: &storageproviderv0alphapb.Grantee{
			Type: storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID,
			Id: &typespb.UserId{
				Idp:      grant.Grantee.UserID.IDP,
				OpaqueId: grant.Grantee.UserID.OpaqueID,
			},
		},
		// Owner: not available in grant, set in parent
		// Creator: TODO not available in grant, add it?
		Ctime: &typespb.Timestamp{}, // TODO should be named btime, not available in grant, add it?
		// Mtime: TODO not available in grant, add it?
	}
	switch grant.Grantee.Type {
	case storage.GranteeTypeUser:
		share.Grantee.Type = storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER
		// FIXME this kind of id works only for acls ...
		// it becomes unique if prefixed with the fileid ...
		share.Id.OpaqueId = "u:" + grant.Grantee.UserID.OpaqueID
	case storage.GranteeTypeGroup:
		share.Grantee.Type = storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP
		// FIXME this kind of id works only for acls ...
		// it becomes unique if prefixed with the fileid ...
		share.Id.OpaqueId = "g:" + grant.Grantee.UserID.OpaqueID
		// FIXME grantee.UserID ... might be a group ... rename to identifier?
	}
	return share
}

func (s *service) UpdateShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateShareRequest) (*usershareproviderv0alphapb.UpdateShareResponse, error) {

	ref := req.Ref.GetId()
	if ref == nil {
		res := &usershareproviderv0alphapb.UpdateShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_UNIMPLEMENTED,
			},
		}
		return res, nil
	}
	oid := ref.OpaqueId

	// TODO split at @ ... encode parts as base64 / something url compatible
	sp := strings.Split(oid, "@")
	sID, id := sp[0], sp[1]

	path, err := s.storage.GetPathByID(ctx, id)
	if err != nil {
		// TODO not found
		return nil, err
	}

	sp = strings.Split(sID, ":")
	sType, username := sp[0], sp[1]

	sPerm := req.Field.GetPermissions()
	if sPerm == nil {
		res := &usershareproviderv0alphapb.UpdateShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INVALID_ARGUMENT,
			},
		}
		return res, nil
	}
	rPerm := sPerm.Permissions
	grant := &storage.Grant{
		Grantee: &storage.Grantee{
			UserID: &user.ID{
				// IDP TODO ?
				OpaqueID: username,
			},
		},
		PermissionSet: &storage.PermissionSet{
			//AddGrant:        rPerm.AddGrand, // TODO map more permissions
			ListContainer:   rPerm.ListContainer,
			CreateContainer: rPerm.CreateContainer,
			Move:            rPerm.Move,
			Delete:          rPerm.Delete,
		},
	}
	switch sType {
	case "u":
		grant.Grantee.Type = storage.GranteeTypeUser
	case "g":
		grant.Grantee.Type = storage.GranteeTypeGroup
	default:
		grant.Grantee.Type = storage.GranteeTypeInvalid
	}
	// TODO the storage has no method to get a grant by shareid
	if err := s.storage.UpdateGrant(ctx, path, grant); err != nil {
		// TODO not found error
		return nil, err
	}
	res := &usershareproviderv0alphapb.UpdateShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_OK,
		},
	}
	return res, nil
}

func (s *service) ListReceivedShares(ctx context.Context, req *usershareproviderv0alphapb.ListReceivedSharesRequest) (*usershareproviderv0alphapb.ListReceivedSharesResponse, error) {
	res := &usershareproviderv0alphapb.ListReceivedSharesResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) UpdateReceivedShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateReceivedShareRequest) (*usershareproviderv0alphapb.UpdateReceivedShareResponse, error) {
	res := &usershareproviderv0alphapb.UpdateReceivedShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}
