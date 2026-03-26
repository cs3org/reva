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
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) GetQuota(ctx context.Context, ref *provider.Reference) (totalbytes, usedbytes uint64, err error) {
	log := appctx.GetLogger(ctx)

	u, err := utils.GetUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "eosfs: no user in ctx")
	}
	// lightweight accounts don't have quota nodes, so we're passing an empty string as path
	userAuth, err := fs.getUserAuth(ctx, u, "")
	if err != nil {
		return 0, 0, err
	}
	cboxAuth := utils.GetEmptyAuth()

	if ref.Path != fs.conf.QuotaNode && ref.Path != "" {
		ref.Path = fs.wrap(ctx, ref.Path)
	}

	if fs.quotaCache != nil {
		key := ref.Path
		if entry, ok := fs.quotaCache.get(key); ok {
			if time.Since(entry.fetchedAt) > fs.quotaCache.ttl {
				// TTL expired: trigger a background refresh if none is already running
				if fs.quotaCache.tryMarkRefreshing(key) {
					go fs.refreshQuotaCache(key, userAuth, cboxAuth, ref.Path)
				}
			}
			log.Debug().Any("ref", ref).Any("quota", entry.info).Str("user", u.Id.OpaqueId).Msgf("GetQuota (cached)")
			return entry.info.TotalBytes, entry.info.UsedBytes, nil
		}
	}

	qi, err := fs.c.GetQuota(ctx, userAuth, cboxAuth, ref.Path)
	log.Debug().Any("ref", ref).Any("quota", qi).Str("user", u.Id.OpaqueId).Err(err).Msgf("GetQuota")
	if err != nil {
		return 0, 0, errors.Wrap(err, "eosfs: error getting quota")
	}

	if fs.quotaCache != nil {
		fs.quotaCache.set(ref.Path, qi)
		log.Info().Str("path", ref.Path).Str("user", u.Id.OpaqueId).Uint64("total", qi.TotalBytes).Uint64("used", qi.UsedBytes).Msg("FINDME: quota cache populated")
	}

	return qi.TotalBytes, qi.UsedBytes, nil
}

// refreshQuotaCache fetches fresh quota data from EOS and updates the cache.
// Intended to be run as a goroutine. Uses a background context since the
// original request context may already be done.
func (fs *Eosfs) refreshQuotaCache(key string, userAuth, cboxAuth eosclient.Authorization, path string) {
	qi, err := fs.c.GetQuota(context.Background(), userAuth, cboxAuth, path)
	if err != nil {
		// Leave the stale entry intact so subsequent requests can still return it;
		// clear the refreshing flag so the next TTL expiry will retry.
		fs.quotaCache.clearRefreshing(key)
		return
	}
	fs.quotaCache.set(key, qi)
}
