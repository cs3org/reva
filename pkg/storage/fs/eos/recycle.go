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
	"path"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	md, err := fs.GetMD(ctx, &provider.Reference{Path: basePath}, nil)
	if err != nil {
		return err
	}

	recycleid, _, err := fs.getRecycleIdAndAuth(ctx, u, md)
	sysAuth := getSystemAuth()

	return fs.c.PurgeDeletedEntries(ctx, recycleid, sysAuth, []string{key})
}

func (fs *Eosfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("eosfs: EmptyRecycle: operation not supported")
}

func (fs *Eosfs) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *types.Timestamp) ([]*provider.RecycleItem, error) {
	var auth eosclient.Authorization
	var recycleid string
	var err error

	log := appctx.GetLogger(ctx)
	log.Debug().Str("basePath", basePath).Str("key", key).Str("relativePath", relativePath).Msgf("ListRecycle")

	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.PermissionDenied("no user found in context for ListRecycle")
	}

	// there are two types of recycle bins ownerless (with a recycle id),
	// or owned by an account (primary account for users, service account for projects)

	// for ownerless recycle bins, a special attribute is set: so let's stat the base path
	md, err := fs.GetMD(ctx, &provider.Reference{Path: basePath}, nil)
	if err != nil {
		log.Error().Err(err).Msgf("ListRecycle")
		return nil, err
	}
	if !md.PermissionSet.ListRecycle {
		log.Error().Err(fmt.Errorf("No permission")).Msgf("ListRecycle")
		return nil, errtypes.PermissionDenied("eosfs: user doesn't have permissions to list the recycle bin")
	}

	recycleid, auth, err = fs.getRecycleIdAndAuth(ctx, u, md)

	var dateFrom, dateTo time.Time
	if from != nil && to != nil {
		dateFrom = time.Unix(int64(from.Seconds), 0)
		dateTo = time.Unix(int64(to.Seconds), 0)
		if dateFrom.AddDate(0, 0, fs.conf.MaxDaysInRecycleList).Before(dateTo) {
			return nil, errtypes.BadRequest("eosfs: too many days requested in listing the recycle bin")
		}
	} else {
		// if no date range was given, list up to two days ago
		dateTo = time.Now()
		dateFrom = dateTo.AddDate(0, 0, -2)
	}

	sublog := appctx.GetLogger(ctx).With().Logger()
	sublog.Debug().Time("from", dateFrom).Time("to", dateTo).Msg("executing ListDeletedEntries")
	eosDeletedEntries, err := fs.c.ListDeletedEntries(ctx, auth, recycleid, fs.conf.MaxRecycleEntries, dateFrom, dateTo)

	log.Debug().Any("entries", eosDeletedEntries).Err(err).Msgf("ListRecycle deleted entries")

	if err != nil {
		switch err.(type) {
		case errtypes.IsBadRequest:
			return nil, errtypes.BadRequest("eosfs: too many entries found in listing the recycle bin")
		default:
			return nil, errors.Wrap(err, "eosfs: error listing deleted entries")
		}
	}
	recycleEntries := []*provider.RecycleItem{}
	for _, entry := range eosDeletedEntries {
		if !fs.conf.ShowHiddenSysFiles {
			base := path.Base(entry.RestorePath)
			if hiddenReg.MatchString(base) {
				continue
			}
		}
		if recycleItem, err := fs.convertToRecycleItem(ctx, entry); err == nil {
			recycleEntries = append(recycleEntries, recycleItem)
		} else {
			log.Error().Err(err).Msgf("Failed to convert to recycle item")
		}
	}
	return recycleEntries, nil
}

func (fs *Eosfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	var auth eosclient.Authorization
	var err error

	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return errtypes.PermissionDenied("no user found in context for RestoreRecycleItem")
	}

	// there are two types of recycle bins ownerless (with a recycle id),
	// or owned by an account (primary account for users, service account for projects)

	// for ownerless recycle bins, a special attribute is set: so let's stat the base path
	md, err := fs.GetMD(ctx, &provider.Reference{Path: basePath}, nil)
	if err != nil {
		return err
	}
	if !md.PermissionSet.RestoreRecycleItem {
		return errtypes.PermissionDenied("eosfs: user doesn't have permissions to restore recycled items")
	}
	// ownerless project: use recycle id
	if _, ok := md.ArbitraryMetadata.Metadata["recycleid"]; ok {
		auth, err = extractUIDAndGID(u)
		if err != nil {
			return err
		}
	} else {
		// project owned by an account: we impersonate the account
		if md.Owner != nil {
			auth, err = fs.getUIDGateway(ctx, md.Owner)
		} else {
			auth, err = extractUIDAndGID(u)
		}
		if err != nil {
			return err
		}
	}

	return fs.c.RestoreDeletedEntry(ctx, auth, key)
}

func (fs *Eosfs) getRecycleIdAndAuth(ctx context.Context, u *userpb.User, md *provider.ResourceInfo) (recycleid string, auth eosclient.Authorization, err error) {
	// ownerless project: use recycle id
	if value, ok := md.ArbitraryMetadata.Metadata["recycleid"]; ok {
		recycleid = value
		auth, err = extractUIDAndGID(u)
		if err != nil {
			return "", eosclient.Authorization{}, err
		}
	} else {
		// space owned by an account: we impersonate the account
		// no recycleid
		recycleid = ""
		if md.Owner != nil {
			auth, err = fs.getUIDGateway(ctx, md.Owner)
		} else {
			auth, err = extractUIDAndGID(u)
		}
		if err != nil {
			return "", eosclient.Authorization{}, err
		}
	}
	return recycleid, auth, nil
}

func (fs *Eosfs) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) (*provider.RecycleItem, error) {
	path, err := fs.unwrap(ctx, eosDeletedItem.RestorePath)
	if err != nil {
		return nil, err
	}
	recycleItem := &provider.RecycleItem{
		Ref:          &provider.Reference{Path: path},
		Key:          eosDeletedItem.RestoreKey,
		Size:         eosDeletedItem.Size,
		DeletionTime: &types.Timestamp{Seconds: eosDeletedItem.DeletionMTime},
	}
	if eosDeletedItem.IsDir {
		recycleItem.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	} else {
		// TODO(labkode): if eos returns more types oin the future we need to map them.
		recycleItem.Type = provider.ResourceType_RESOURCE_TYPE_FILE
	}
	return recycleItem, nil
}
