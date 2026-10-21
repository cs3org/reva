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

package json

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
)

func init() {
	registry.Register("json", New)
}

// New returns a new authorizer object.
func New(ctx context.Context, m map[string]any) (share.Repository, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// load or create file
	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the shares")
		return nil, err
	}

	mgr := &mgr{
		c:     &c,
		model: model,
	}

	return mgr, nil
}

func loadOrCreate(file string) (*shareModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := os.WriteFile(file, []byte("{}"), 0700); err != nil {
			return nil, errors.Wrap(err, "error creating the file: "+file)
		}
	}

	f, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening the file: "+file)
		return nil, err
	}
	defer f.Close()

	var m shareModel
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		if err != io.EOF {
			return nil, errors.Wrap(err, "error decoding data to json")
		}
	}

	if m.Shares == nil {
		m.Shares = map[string]*ocm.Share{}
	}
	if m.ReceivedShares == nil {
		m.ReceivedShares = map[string]*ocm.ReceivedShare{}
	}
	if m.TransferGenerations == nil {
		m.TransferGenerations = map[string]uint64{}
	}
	if m.ResourceTypes == nil {
		m.ResourceTypes = map[string]ocm.SharedResourceType{}
	}

	return &m, nil
}

type shareModel struct {
	Shares         map[string]*ocm.Share         `json:"shares"`          // share_id -> share
	ReceivedShares map[string]*ocm.ReceivedShare `json:"received_shares"` // share_id -> share
	// TransferGenerations holds the embedded transfer attempt of received
	// shares, which the CS3 received share has no field for.
	TransferGenerations map[string]uint64 `json:"transfer_generations"` // share_id -> attempt
	// ResourceTypes holds the resource type of outgoing shares, which the
	// CS3 share has no field for.
	ResourceTypes map[string]ocm.SharedResourceType `json:"resource_types"` // share_id -> type
}

func (s *shareModel) UnmarshalJSON(d []byte) error {
	m := struct {
		Shares              map[string]json.RawMessage        `json:"shares"`
		ReceivedShares      map[string]json.RawMessage        `json:"received_shares"`
		TransferGenerations map[string]uint64                 `json:"transfer_generations"`
		ResourceTypes       map[string]ocm.SharedResourceType `json:"resource_types"`
	}{}

	if err := json.Unmarshal(d, &m); err != nil {
		return err
	}

	share := map[string]*ocm.Share{}
	for k, v := range m.Shares {
		var s ocm.Share
		if err := utils.UnmarshalJSONToProtoV1(v, &s); err != nil {
			return err
		}
		share[k] = &s
	}

	received := map[string]*ocm.ReceivedShare{}
	for k, v := range m.ReceivedShares {
		var s ocm.ReceivedShare
		if err := utils.UnmarshalJSONToProtoV1(v, &s); err != nil {
			return err
		}
		received[k] = &s
	}

	generations := m.TransferGenerations
	if generations == nil {
		generations = map[string]uint64{}
	}
	resourceTypes := m.ResourceTypes
	if resourceTypes == nil {
		resourceTypes = map[string]ocm.SharedResourceType{}
	}

	*s = shareModel{
		Shares:              share,
		ReceivedShares:      received,
		TransferGenerations: generations,
		ResourceTypes:       resourceTypes,
	}

	return nil
}

func (s *shareModel) MarshalJSON() ([]byte, error) {
	shares := map[string]json.RawMessage{}
	for k, v := range s.Shares {
		d, err := utils.MarshalProtoV1ToJSON(v)
		if err != nil {
			return nil, err
		}
		shares[k] = d
	}

	received := map[string]json.RawMessage{}
	for k, v := range s.ReceivedShares {
		d, err := utils.MarshalProtoV1ToJSON(v)
		if err != nil {
			return nil, err
		}
		received[k] = d
	}

	return json.Marshal(map[string]any{
		"shares":               shares,
		"received_shares":      received,
		"transfer_generations": s.TransferGenerations,
		"resource_types":       s.ResourceTypes,
	})
}

type config struct {
	File string `mapstructure:"file"`
}

func (c *config) ApplyDefaults() {
	if c.File == "" {
		c.File = "/var/tmp/reva/ocm-shares.json"
	}
}

type mgr struct {
	c          *config
	sync.Mutex // concurrent access to the file
	model      *shareModel
}

func (m *mgr) save() error {
	f, err := os.OpenFile(m.c.File, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Wrap(err, "error opening file "+m.c.File)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(m.model); err != nil {
		return errors.Wrap(err, "error encoding to json")
	}

	return f.Sync()
}

func (m *mgr) load() error {
	f, err := os.OpenFile(m.c.File, os.O_RDONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "error opening file "+m.c.File)
	}
	defer f.Close()

	d, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	var model shareModel
	if err := json.Unmarshal(d, &model); err != nil {
		return err
	}

	m.model = &model
	return nil
}

func genID() string {
	return uuid.New().String()
}

// GenerateID generates a unique ID for a share that is yet to be stored,
// so that the share can be referenced before being persisted with StoreShare.
func (m *mgr) GenerateID(ctx context.Context) (*ocm.ShareId, error) {
	return &ocm.ShareId{OpaqueId: genID()}, nil
}

func (m *mgr) StoreShare(ctx context.Context, ocmshare *ocm.Share, resourceType ocm.SharedResourceType) (*ocm.Share, error) {
	if !share.SupportedResourceType(resourceType) {
		return nil, share.ErrInvalidResourceType
	}
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	if _, err := m.getByKey(ctx, &ocm.ShareKey{
		Owner:      ocmshare.Owner,
		ResourceId: ocmshare.ResourceId,
		Grantee:    ocmshare.Grantee,
	}); err == nil {
		return nil, share.ErrShareAlreadyExisting
	}

	m.model.Shares[ocmshare.Id.OpaqueId] = proto.Clone(ocmshare).(*ocm.Share)
	m.model.ResourceTypes[ocmshare.Id.OpaqueId] = resourceType

	if err := m.save(); err != nil {
		return nil, errors.Wrap(err, "error saving share")
	}

	return ocmshare, nil
}

func (m *mgr) GetShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	var (
		s   *ocm.Share
		err error
	)

	if err := m.load(); err != nil {
		return nil, err
	}

	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, ref.GetKey())
	case ref.GetToken() != "":
		return m.getByToken(ctx, ref.GetToken())
	default:
		err = errtypes.NotFound(ref.String())
	}

	if err != nil {
		return nil, err
	}

	// Outgoing OCM shares must be readable by the owner, the creator,
	// and the federated grantee identified in the share payload.
	if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
		return s, nil
	}
	if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, s.Grantee.GetUserId()) {
		return s, nil
	}

	return nil, share.ErrShareNotFound
}

func (m *mgr) getByToken(ctx context.Context, token string) (*ocm.Share, error) {
	for _, share := range m.model.Shares {
		if share.Token == token {
			return share, nil
		}
	}
	return nil, errtypes.NotFound(token)
}

func (m *mgr) getByID(ctx context.Context, id *ocm.ShareId) (*ocm.Share, error) {
	if share, ok := m.model.Shares[id.OpaqueId]; ok {
		return share, nil
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *mgr) getByKey(ctx context.Context, key *ocm.ShareKey) (*ocm.Share, error) {
	for _, share := range m.model.Shares {
		if (utils.UserEqual(key.Owner, share.Owner) || utils.UserEqual(key.Owner, share.Creator)) &&
			utils.ResourceIDEqual(key.ResourceId, share.ResourceId) && utils.GranteeEqual(key.Grantee, share.Grantee) {
			return share, nil
		}
	}
	return nil, share.ErrShareNotFound
}

// DeleteShare deletes the share and returns it as it was stored. Separate
// processes writing the same file are not coordinated.
func (m *mgr) DeleteShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*share.ShareRemoval, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	for id, s := range m.model.Shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				rt := m.model.ResourceTypes[id]
				if !share.SupportedResourceType(rt) {
					return nil, share.ErrInvalidResourceType
				}
				removal := &share.ShareRemoval{Share: s, ResourceType: rt}
				delete(m.model.Shares, id)
				delete(m.model.ResourceTypes, id)
				if err := m.save(); err != nil {
					return nil, err
				}
				return removal, nil
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func sharesEqual(ref *ocm.ShareReference, s *ocm.Share) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {
		if (utils.UserEqual(ref.GetKey().Owner, s.Owner) || utils.UserEqual(ref.GetKey().Owner, s.Creator)) &&
			utils.ResourceIDEqual(ref.GetKey().ResourceId, s.ResourceId) && utils.GranteeEqual(ref.GetKey().Grantee, s.Grantee) {
			return true
		}
	}
	return false
}

func receivedShareEqual(ref *ocm.ShareReference, s *ocm.ReceivedShare) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	}
	return false
}

func (m *mgr) UpdateShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*share.ShareUpdate, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	var s *ocm.Share
	var err error
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, ref.GetKey())
	case ref.GetToken() != "":
		s, err = m.getByToken(ctx, ref.GetToken())
	default:
		return nil, errtypes.NotFound(ref.String())
	}
	if err != nil {
		return nil, err
	}

	if !utils.UserEqual(user.Id, s.Owner) && !utils.UserEqual(user.Id, s.Creator) {
		return nil, share.ErrShareNotFound
	}
	resourceType := m.model.ResourceTypes[s.Id.OpaqueId]
	if !share.SupportedResourceType(resourceType) {
		return nil, share.ErrInvalidResourceType
	}
	before := proto.Clone(s).(*ocm.Share)

	for _, field := range f {
		switch u := field.Field.(type) {
		case *ocm.UpdateOCMShareRequest_UpdateField_Expiration:
			s.Expiration = u.Expiration
		case *ocm.UpdateOCMShareRequest_UpdateField_AccessMethods:
			if err := mergeAccessMethod(s, u.AccessMethods); err != nil {
				return nil, err
			}
		}
	}

	now := time.Now().UnixNano()
	s.Mtime = &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}
	if err := m.save(); err != nil {
		return nil, errors.Wrap(err, "error saving model")
	}

	after := proto.Clone(s).(*ocm.Share)
	return &share.ShareUpdate{
		Before:                   before,
		After:                    after,
		ResourceType:             resourceType,
		WebDAVPermissionsChanged: permissionsChanged(webdavPermissions(before), webdavPermissions(after)),
		WebappPermissionsChanged: permissionsChanged(webappPermissions(before), webappPermissions(after)),
	}, nil
}

func webdavPermissions(s *ocm.Share) proto.Message {
	for _, m := range s.AccessMethods {
		if w, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
			return w.WebdavOptions.GetPermissions()
		}
	}
	return nil
}

func webappPermissions(s *ocm.Share) proto.Message {
	for _, m := range s.AccessMethods {
		if w, ok := m.Term.(*ocm.AccessMethod_WebappOptions); ok {
			return w.WebappOptions.GetPermissions()
		}
	}
	return nil
}

func permissionsChanged(before, after proto.Message) bool {
	if before == nil || after == nil {
		return (before == nil) != (after == nil)
	}
	return !proto.Equal(before, after)
}

func mergeAccessMethod(s *ocm.Share, incoming *ocm.AccessMethod) error {
	switch inc := incoming.Term.(type) {
	case *ocm.AccessMethod_WebdavOptions:
		for _, am := range s.AccessMethods {
			if existing, ok := am.Term.(*ocm.AccessMethod_WebdavOptions); ok {
				if err := validateImmutableStringSlice(existing.WebdavOptions.Requirements, inc.WebdavOptions.Requirements); err != nil {
					return err
				}
				if err := validateImmutableAccessTypes(existing.WebdavOptions.AccessTypes, inc.WebdavOptions.AccessTypes); err != nil {
					return err
				}
				existing.WebdavOptions.Permissions = inc.WebdavOptions.Permissions
				return nil
			}
		}
		s.AccessMethods = append(s.AccessMethods, incoming)
	case *ocm.AccessMethod_WebappOptions:
		for _, am := range s.AccessMethods {
			if existing, ok := am.Term.(*ocm.AccessMethod_WebappOptions); ok {
				if err := validateImmutableStringSlice(existing.WebappOptions.Requirements, inc.WebappOptions.Requirements); err != nil {
					return err
				}
				existing.WebappOptions.Permissions = inc.WebappOptions.Permissions
				return nil
			}
		}
		s.AccessMethods = append(s.AccessMethods, incoming)
	}
	return nil
}

func validateImmutableStringSlice(stored, incoming []string) error {
	if len(incoming) == 0 {
		return nil
	}
	if len(stored) == 0 {
		return errtypes.BadRequest("requirements cannot be set on update; they are immutable after creation")
	}
	if len(stored) != len(incoming) {
		return errtypes.BadRequest("requirements are immutable after creation")
	}
	for i := range stored {
		if stored[i] != incoming[i] {
			return errtypes.BadRequest("requirements are immutable after creation")
		}
	}
	return nil
}

func validateImmutableAccessTypes(stored, incoming []ocm.AccessType) error {
	if len(incoming) == 0 {
		return nil
	}
	if len(stored) == 0 {
		return errtypes.BadRequest("access_types cannot be set on update; they are immutable after creation")
	}
	if len(stored) != len(incoming) {
		return errtypes.BadRequest("access_types are immutable after creation")
	}
	for i := range stored {
		if stored[i] != incoming[i] {
			return errtypes.BadRequest("access_types are immutable after creation")
		}
	}
	return nil
}

func (m *mgr) ListShares(ctx context.Context, user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	var ss []*ocm.Share

	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	for _, share := range m.model.Shares {
		if utils.UserEqual(user.Id, share.Owner) || utils.UserEqual(user.Id, share.Creator) {
			// no filter we return earlier
			if len(filters) == 0 {
				ss = append(ss, share)
			} else {
				// check filters
				// TODO(labkode): add the rest of filters.
				for _, f := range filters {
					if f.Type == ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID {
						if utils.ResourceIDEqual(share.ResourceId, f.GetResourceId()) {
							ss = append(ss, share)
						}
					}
				}
			}
		}
	}
	return ss, nil
}

func (m *mgr) StoreReceivedShare(ctx context.Context, share *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	share.Id = &ocm.ShareId{
		OpaqueId: genID(),
	}
	share.Ctime = ts
	share.Mtime = ts

	m.model.ReceivedShares[share.Id.OpaqueId] = proto.Clone(share).(*ocm.ReceivedShare)
	if err := m.save(); err != nil {
		return nil, err
	}

	return share, nil
}

func (m *mgr) ListReceivedShares(ctx context.Context, user *userpb.User, filters []*ocm.ListReceivedOCMSharesRequest_Filter) ([]*ocm.ReceivedShare, error) {
	var rss []*ocm.ReceivedShare
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	for _, share := range m.model.ReceivedShares {
		if utils.UserEqual(user.Id, share.Owner) || utils.UserEqual(user.Id, share.Creator) {
			// omit shares created by me
			continue
		}

		if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, share.Grantee.GetUserId()) {
			rss = append(rss, share)
		}
	}
	return rss, nil
}

func (m *mgr) GetReceivedShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}

	for _, share := range m.model.ReceivedShares {
		if receivedShareEqual(ref, share) {
			if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, share.Grantee.GetUserId()) {
				return share, nil
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

// maxTransferGeneration matches the SQL driver bound.
const maxTransferGeneration = math.MaxInt64

// receivedOf returns the stored received share of the user, under the lock.
func (m *mgr) receivedOf(user *userpb.User, id *ocm.ShareId) (*ocm.ReceivedShare, error) {
	if id == nil {
		return nil, share.ErrShareNotFound
	}
	rs, ok := m.model.ReceivedShares[id.OpaqueId]
	if !ok || rs.Grantee.Type != provider.GranteeType_GRANTEE_TYPE_USER || !utils.UserEqual(user.Id, rs.Grantee.GetUserId()) {
		return nil, errtypes.NotFound(id.String())
	}
	return rs, nil
}

func (m *mgr) bumpGeneration(id string) (uint64, error) {
	gen := m.model.TransferGenerations[id]
	if gen >= maxTransferGeneration {
		return 0, share.ErrTransferGenerationExhausted
	}
	gen++
	m.model.TransferGenerations[id] = gen
	return gen, nil
}

func touch(rs *ocm.ReceivedShare) {
	now := time.Now().UnixNano()
	rs.Mtime = &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}
}

// UpdateReceivedShare updates the state under the lock and returns the values
// before and after. A state change invalidates the running transfer attempt.
func (m *mgr) UpdateReceivedShare(ctx context.Context, user *userpb.User, s *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*share.ReceivedShareUpdate, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}
	rs, err := m.receivedOf(user, s.Id)
	if err != nil {
		return nil, err
	}
	before := proto.Clone(rs).(*ocm.ReceivedShare)

	for _, mask := range fieldMask.GetPaths() {
		switch mask {
		case "state":
			if rs.State != s.State {
				if _, err := m.bumpGeneration(rs.Id.OpaqueId); err != nil {
					return nil, err
				}
			}
			rs.State = s.State
		// TODO case "mount_point":
		default:
			return nil, errtypes.NotSupported("updating " + mask + " is not supported")
		}
	}
	touch(rs)

	if err := m.save(); err != nil {
		return nil, errors.Wrap(err, "error saving model")
	}
	return &share.ReceivedShareUpdate{Before: before, After: proto.Clone(rs).(*ocm.ReceivedShare)}, nil
}

// StartReceivedShareTransfer moves a share that is not transferring to
// TRANSFERRING and starts a new attempt.
func (m *mgr) StartReceivedShareTransfer(ctx context.Context, user *userpb.User, s *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*share.TransferStart, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}
	rs, err := m.receivedOf(user, s.Id)
	if err != nil {
		return nil, err
	}
	for _, mask := range fieldMask.GetPaths() {
		if mask != "state" {
			return nil, errtypes.NotSupported("updating " + mask + " is not supported")
		}
	}
	if rs.State == ocm.ShareState_SHARE_STATE_TRANSFERRING {
		return nil, share.ErrTransferInProgress
	}
	gen, err := m.bumpGeneration(rs.Id.OpaqueId)
	if err != nil {
		return nil, err
	}
	rs.State = ocm.ShareState_SHARE_STATE_TRANSFERRING
	touch(rs)
	if err := m.save(); err != nil {
		return nil, errors.Wrap(err, "error saving model")
	}
	return &share.TransferStart{Share: proto.Clone(rs).(*ocm.ReceivedShare), Generation: gen}, nil
}

// CompleteReceivedShareTransfer writes the final state and mtime only, and
// only while the same attempt is still running.
func (m *mgr) CompleteReceivedShareTransfer(ctx context.Context, user *userpb.User, id *ocm.ShareId, generation uint64, state ocm.ShareState) (*share.ReceivedShareUpdate, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.load(); err != nil {
		return nil, err
	}
	rs, err := m.receivedOf(user, id)
	if err != nil {
		return nil, err
	}
	before := proto.Clone(rs).(*ocm.ReceivedShare)
	if rs.State != ocm.ShareState_SHARE_STATE_TRANSFERRING || m.model.TransferGenerations[rs.Id.OpaqueId] != generation {
		return &share.ReceivedShareUpdate{Before: before, After: before}, nil
	}
	rs.State = state
	touch(rs)
	if err := m.save(); err != nil {
		return nil, errors.Wrap(err, "error saving model")
	}
	return &share.ReceivedShareUpdate{Before: before, After: proto.Clone(rs).(*ocm.ReceivedShare)}, nil
}

func (m *mgr) GetEmbeddedPayload(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare) (string, error) {
	return "", errtypes.NotSupported("operation not supported")
}
