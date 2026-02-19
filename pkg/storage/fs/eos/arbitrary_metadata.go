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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	if len(md.Metadata) == 0 {
		return errtypes.BadRequest("eosfs: no metadata set")
	}

	fn, _, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	cboxAuth := utils.GetEmptyAuth()

	for k, v := range md.Metadata {
		if k == "" || v == "" {
			return errtypes.BadRequest(fmt.Sprintf("eosfs: key or value is empty: key:%s, value:%s", k, v))
		}

		// do not allow to override system-reserved keys
		if k == lockPayloadKey || k == eosLockKey || k == lwShareAttrKey || k == refTargetAttrKey {
			return errtypes.BadRequest(fmt.Sprintf("eosfs: key %s is reserved", k))
		}

		attr := &eosclient.Attribute{
			Type: UserAttr,
			Key:  k,
			Val:  v,
		}

		// TODO(labkode): SetArbitraryMetadata does not have semantics for recursivity.
		// We set it to false
		err := fs.c.SetAttr(ctx, cboxAuth, attr, false, false, fn, "")
		if err != nil {
			return errors.Wrap(err, "eosfs: error setting xattr in eos driver")
		}
	}
	return nil
}

func (fs *Eosfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	if len(keys) == 0 {
		return errtypes.BadRequest("eosfs: no keys set")
	}

	fn, _, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	cboxAuth := utils.GetEmptyAuth()

	for _, k := range keys {
		if k == "" {
			return errtypes.BadRequest("eosfs: key is empty")
		}

		attr := &eosclient.Attribute{
			Type: UserAttr,
			Key:  k,
		}

		err := fs.c.UnsetAttr(ctx, cboxAuth, attr, false, fn, "")

		if err != nil {
			if errors.Is(err, eosclient.AttrNotExistsError) {
				continue
			}
			return errors.Wrap(err, "eosfs: error unsetting xattr in eos driver")
		}
	}
	return nil
}
