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

package node

import (
	"context"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/storage/utils/ace"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs/prefixes"
)

// ListGrantees lists the grantees of the current node
// We don't want to wast time and memory by creating grantee objects.
// The function will return a list of opaque strings that can be used to make a ReadGrant call
func (n *Node) ListGrantees(ctx context.Context) (grantees []string, err error) {
	attrs, err := n.Xattrs()
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("node", n.ID).Msg("error listing attributes")
		return nil, err
	}
	for name := range attrs {
		if strings.HasPrefix(name, prefixes.GrantPrefix) {
			grantees = append(grantees, name)
		}
	}
	return
}

// ReadGrant reads a CS3 grant
func (n *Node) ReadGrant(ctx context.Context, grantee string) (g *provider.Grant, err error) {
	xattr, err := n.Xattr(grantee)
	if err != nil {
		return nil, err
	}
	var e *ace.ACE
	if e, err = ace.Unmarshal(strings.TrimPrefix(grantee, prefixes.GrantPrefix), []byte(xattr)); err != nil {
		return nil, err
	}
	return e.Grant(), nil
}

// ListGrants lists all grants of the current node.
func (n *Node) ListGrants(ctx context.Context) ([]*provider.Grant, error) {
	grantees, err := n.ListGrantees(ctx)
	if err != nil {
		return nil, err
	}

	grants := make([]*provider.Grant, 0, len(grantees))
	for _, g := range grantees {
		grant, err := n.ReadGrant(ctx, g)
		if err != nil {
			appctx.GetLogger(ctx).
				Error().
				Err(err).
				Str("node", n.ID).
				Str("grantee", g).
				Msg("error reading grant")
			continue
		}
		grants = append(grants, grant)
	}
	return grants, nil
}

// SetGrant sets a grant on the node
func (n *Node) SetGrant(g *provider.Grant) error {
	// set the grant
	e := ace.FromGrant(g)
	principal, value := e.Marshal()
	return n.SetXattr(prefixes.GrantPrefix+principal, string(value))
}

// RemoveGrant removes a grant from the node
func (n *Node) RemoveGrant(ctx context.Context, g *provider.Grant) error {
	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = prefixes.GrantGroupAcePrefix + g.Grantee.GetGroupId().OpaqueId
	} else {
		attr = prefixes.GrantUserAcePrefix + g.Grantee.GetUserId().OpaqueId
	}
	return n.RemoveXattr(attr)
}
