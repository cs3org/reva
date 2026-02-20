// Copyright 2018-2026 CERN
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

package eos

import (
	"context"
	"fmt"
	"strings"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	"github.com/pkg/errors"
)

func (fs *Eosfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	log := appctx.GetLogger(ctx)
	log.Info().Any("ref", ref).Any("grant", g).Msgf("AddGrant")

	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return err
	}
	log.Info().Any("ref", ref).Str("path", fn).Msgf("AddGrant - resolved ref")

	sysAuth := getSystemAuth()
	userAuth, err := fs.getUserAuth(ctx)
	if err != nil {
		return err
	}

	eosACL, err := fs.getEosACL(ctx, g)
	if err != nil {
		return err
	}

	if eosACL.Type == acl.TypeLightweight {
		// The ACLs for a lightweight are not understandable by EOS
		// directly, but only from reva. So we have to store them
		// in an xattr named sys.reva.lwshare.<lw_account>, with value
		// the permissions.
		attr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  fmt.Sprintf("%s.%s", lwShareAttrKey, eosACL.Qualifier),
			Val:  eosACL.Permissions,
		}

		if err := fs.c.SetAttr(ctx, sysAuth, attr, false, true, fn, ""); err != nil {
			return errors.Wrap(err, "eosfs: error adding acl for lightweight account")
		}
		return nil
	}

	err = fs.c.AddACL(ctx, userAuth, sysAuth, fn, eosclient.StartPosition, eosACL)
	if err != nil {
		return errors.Wrap(err, "eosfs: error adding acl")
	}
	return nil
}

func (fs *Eosfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return err
	}

	sysAuth := getSystemAuth()
	userAuth, err := fs.getUserAuth(ctx)
	if err != nil {
		return err
	}

	position := eosclient.EndPosition

	// empty permissions => deny
	grant := &provider.Grant{
		Grantee:     g,
		Permissions: &provider.ResourcePermissions{},
	}

	eosACL, err := fs.getEosACL(ctx, grant)
	if err != nil {
		return err
	}

	err = fs.c.AddACL(ctx, userAuth, sysAuth, fn, position, eosACL)
	if err != nil {
		return errors.Wrap(err, "eosfs: error adding acl")
	}
	return nil
}

func (fs *Eosfs) getEosACL(ctx context.Context, g *provider.Grant) (*acl.Entry, error) {
	permissions, err := grants.GetACLPerm(g.Permissions)
	if err != nil {
		return nil, err
	}
	t, err := grants.GetACLType(g.Grantee.Type)
	if err != nil {
		return nil, err
	}

	var qualifier string
	if t == acl.TypeUser {
		// if the grantee is a lightweight account, we need to set it accordingly
		if g.Grantee.GetUserId().Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT ||
			g.Grantee.GetUserId().Type == userpb.UserType_USER_TYPE_FEDERATED {
			t = acl.TypeLightweight
			qualifier = g.Grantee.GetUserId().OpaqueId
		} else {
			// since EOS Citrine ACLs are stored with uid, we need to convert username to
			// uid only for users.
			auth, err := fs.getUIDGateway(ctx, g.Grantee.GetUserId())
			if err != nil {
				return nil, err
			}
			qualifier = auth.Role.UID
		}
	} else {
		qualifier = g.Grantee.GetGroupId().OpaqueId
	}

	eosACL := &acl.Entry{
		Qualifier:   qualifier,
		Permissions: permissions,
		Type:        t,
	}
	return eosACL, nil
}

func (fs *Eosfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return err
	}

	sysAuth := getSystemAuth()
	userAuth, err := fs.getUserAuth(ctx)
	if err != nil {
		return err
	}

	eosACL, err := fs.getEosACL(ctx, g)
	if err != nil {
		return err
	}

	if eosACL.Type == acl.TypeLightweight {
		attr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  fmt.Sprintf("%s.%s", lwShareAttrKey, eosACL.Qualifier),
		}

		if err := fs.c.UnsetAttr(ctx, sysAuth, attr, true, fn, ""); err != nil {
			return errors.Wrap(err, "eosfs: error removing acl for lightweight account")
		}
		return nil
	}

	err = fs.c.RemoveACL(ctx, userAuth, sysAuth, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eosfs: error removing acl")
	}
	return nil
}

func (fs *Eosfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

func (fs *Eosfs) convertACLsToGrants(ctx context.Context, acls *acl.ACLs) ([]*provider.Grant, error) {
	res := make([]*provider.Grant, 0, len(acls.Entries))
	for _, a := range acls.Entries {
		var grantee *provider.Grantee
		switch {
		case a.Type == acl.TypeUser:
			// EOS Citrine ACLs are stored with uid for users.
			// This needs to be resolved to the user opaque ID.
			qualifier, err := fs.getUserIDGateway(ctx, a.Qualifier)
			if err != nil {
				return nil, err
			}
			grantee = &provider.Grantee{
				Id:   &provider.Grantee_UserId{UserId: qualifier},
				Type: grants.GetGranteeType(a.Type),
			}
		case a.Type == acl.TypeGroup:
			grantee = &provider.Grantee{
				Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: a.Qualifier}},
				Type: grants.GetGranteeType(a.Type),
			}
		default:
			return nil, errtypes.InternalError(fmt.Sprintf("eosfs: acl type %s not recognised", a.Type))
		}
		res = append(res, &provider.Grant{
			Grantee:     grantee,
			Permissions: grants.GetGrantPermissionSet(a.Permissions),
		})
	}
	return res, nil
}

func isSysACLs(a *eosclient.Attribute) bool {
	return a.Type == SystemAttr && a.Key == "sys"
}

func isLightweightACL(a *eosclient.Attribute) bool {
	return a.Type == SystemAttr && strings.HasPrefix(a.Key, lwShareAttrKey)
}

func parseLightweightACL(a *eosclient.Attribute) *provider.Grant {
	qualifier := strings.TrimPrefix(a.Key, lwShareAttrKey+".")
	return &provider.Grant{
		Grantee: &provider.Grantee{
			Id: &provider.Grantee_UserId{UserId: &userpb.UserId{
				// FIXME: idp missing, maybe get the user_id from the user provider?
				Type:     userpb.UserType_USER_TYPE_LIGHTWEIGHT,
				OpaqueId: qualifier,
			}},
			Type: grants.GetGranteeType(acl.TypeLightweight),
		},
		Permissions: grants.GetGrantPermissionSet(a.Val),
	}
}

func (fs *Eosfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}

	userAuth, err := fs.getUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	// This is invoked just to see if it fails: the user should
	// have acces
	_, err = fs.c.GetAttrs(ctx, userAuth, fn)
	if err != nil {
		return nil, err
	}

	// Now we get the real info (since users cannot get all the attrs)
	sysAuth := getSystemAuth()

	attrs, err := fs.c.GetAttrs(ctx, sysAuth, fn)
	if err != nil {
		return nil, err
	}

	grantList := []*provider.Grant{}
	for _, a := range attrs {
		switch {
		case isSysACLs(a):
			// EOS ACLs
			acls, err := acl.Parse(a.Val, acl.ShortTextForm)
			if err != nil {
				return nil, err
			}
			grants, err := fs.convertACLsToGrants(ctx, acls)
			if err != nil {
				return nil, err
			}
			grantList = append(grantList, grants...)
		case isLightweightACL(a):
			grantList = append(grantList, parseLightweightACL(a))
		}
	}

	return grantList, nil
}
