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

// Note that the StorageProvider does not expose any option to set Quota,
// but this method is exposed by the drivers, and can be called by external
// management tools (like cernboxcop)

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) GetQuota(ctx context.Context, ref *provider.Reference) (totalbytes, usedbytes uint64, err error) {
	log := appctx.GetLogger(ctx)

	u, err := utils.GetUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "eosfs: no user in ctx")
	}

	if utils.IsLightweightUser(u) {
		return 0, 0, errors.Wrap(err, "eosfs: lightweight users do not have quota")
	}

	userAuth, err := extractUIDAndGID(u)
	if err != nil {
		return 0, 0, err
	}
	sysAuth := getSystemAuth()

	if ref.Path != fs.conf.QuotaNode && ref.Path != "" {
		ref.Path = fs.wrap(ctx, ref.Path)
	}

	qi, err := fs.c.GetQuota(ctx, userAuth, sysAuth, ref.Path)
	log.Debug().Any("ref", ref).Any("quota", qi).Str("user", u.Id.OpaqueId).Err(err).Msgf("GetQuota")
	if err != nil {
		err := errors.Wrap(err, "eosfs: error getting quota")
		return 0, 0, err
	}

	return qi.TotalBytes, qi.UsedBytes, nil
}
