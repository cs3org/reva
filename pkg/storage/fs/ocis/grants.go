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

package ocis

import (
	"context"
	"path/filepath"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/ace"
	"github.com/pkg/xattr"
)

func (fs *ocisfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("ref", ref).Interface("grant", g).Msg("AddGrant()")
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	np := filepath.Join(fs.pw.Root, "nodes", node.ID)
	e := ace.FromGrant(g)
	principal, value := e.Marshal()
	if err := xattr.Set(np, sharePrefix+principal, value); err != nil {
		return err
	}
	return fs.tp.Propagate(ctx, node)
}

func (fs *ocisfs) ListGrants(ctx context.Context, ref *provider.Reference) (grants []*provider.Grant, err error) {
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}
	log := appctx.GetLogger(ctx)
	np := filepath.Join(fs.pw.Root, "nodes", node.ID)
	var attrs []string
	if attrs, err = xattr.List(np); err != nil {
		log.Error().Err(err).Msg("error listing attributes")
		return nil, err
	}

	log.Debug().Interface("attrs", attrs).Msg("read attributes")

	aces := extractACEsFromAttrs(ctx, np, attrs)

	grants = make([]*provider.Grant, 0, len(aces))
	for i := range aces {
		grants = append(grants, aces[i].Grant())
	}

	return grants, nil
}

func (fs *ocisfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + g.Grantee.Id.OpaqueId
	} else {
		attr = sharePrefix + "u:" + g.Grantee.Id.OpaqueId
	}

	np := filepath.Join(fs.pw.Root, "nodes", node.ID)
	if err = xattr.Remove(np, attr); err != nil {
		return
	}

	return fs.tp.Propagate(ctx, node)
}

func (fs *ocisfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

// extractACEsFromAttrs reads ACEs in the list of attrs from the node
func extractACEsFromAttrs(ctx context.Context, fsfn string, attrs []string) (entries []*ace.ACE) {
	log := appctx.GetLogger(ctx)
	entries = []*ace.ACE{}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], sharePrefix) {
			var value []byte
			var err error
			if value, err = xattr.Get(fsfn, attrs[i]); err != nil {
				log.Error().Err(err).Str("attr", attrs[i]).Msg("could not read attribute")
				continue
			}
			var e *ace.ACE
			principal := attrs[i][len(sharePrefix):]
			if e, err = ace.Unmarshal(principal, value); err != nil {
				log.Error().Err(err).Str("principal", principal).Str("attr", attrs[i]).Msg("could unmarshal ace")
				continue
			}
			entries = append(entries, e)
		}
	}
	return
}
