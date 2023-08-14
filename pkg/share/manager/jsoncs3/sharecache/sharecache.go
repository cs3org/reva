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

package sharecache

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/shareid"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/r3labs/diff/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// name is the Tracer name used to identify this instrumentation library.
const tracerName = "sharecache"

// Cache caches the list of share ids for users/groups
// It functions as an in-memory cache with a persistence layer
// The storage is sharded by user/group
type Cache struct {
	lockMap sync.Map

	UserShares map[string]*UserShareCache

	storage   metadata.Storage
	namespace string
	filename  string
	ttl       time.Duration
}

// UserShareCache holds the space/share map for one user
type UserShareCache struct {
	Mtime      time.Time
	UserShares map[string]*SpaceShareIDs

	etag     string
	nextSync time.Time
}

// SpaceShareIDs holds the unique list of share ids for a space
type SpaceShareIDs struct {
	Mtime time.Time
	IDs   map[string]struct{}
}

func (c *Cache) lockUser(userID string) func() {
	v, _ := c.lockMap.LoadOrStore(userID, &sync.Mutex{})
	lock := v.(*sync.Mutex)

	lock.Lock()
	return func() { lock.Unlock() }
}

// New returns a new Cache instance
func New(s metadata.Storage, namespace, filename string, ttl time.Duration) Cache {
	return Cache{
		UserShares: map[string]*UserShareCache{},
		storage:    s,
		namespace:  namespace,
		filename:   filename,
		ttl:        ttl,
		lockMap:    sync.Map{},
	}
}

// Add adds a share to the cache
func (c *Cache) Add(ctx context.Context, userid, shareID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.lockUser(userid)
	span.End()
	span.SetAttributes(attribute.String("cs3.userid", userid))
	defer unlock()

	if c.UserShares[userid] == nil {
		err := c.syncWithLock(ctx, userid)
		if err != nil {
			return err
		}
	}

	ctx, span = appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Add")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.userid", userid), attribute.String("cs3.shareid", shareID))

	storageid, spaceid, _ := shareid.Decode(shareID)
	ssid := storageid + shareid.IDDelimiter + spaceid

	persistFunc := func() error {
		c.initializeIfNeeded(userid, ssid)

		now := time.Now()
		// add share id
		c.UserShares[userid].UserShares[ssid].Mtime = now
		c.UserShares[userid].UserShares[ssid].IDs[shareID] = struct{}{}
		return c.Persist(ctx, userid)
	}

	err := persistFunc()
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := c.syncWithLock(ctx, userid); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			return err
		}

		err = persistFunc()
		// TODO try more often?
	}
	return err
}

// Remove removes a share for the given user
func (c *Cache) Remove(ctx context.Context, userid, shareID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.lockUser(userid)
	span.End()
	span.SetAttributes(attribute.String("cs3.userid", userid))
	defer unlock()

	if c.UserShares[userid] == nil {
		err := c.syncWithLock(ctx, userid)
		if err != nil {
			return err
		}
	}

	ctx, span = appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Remove")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.userid", userid), attribute.String("cs3.shareid", shareID))

	storageid, spaceid, _ := shareid.Decode(shareID)
	ssid := storageid + shareid.IDDelimiter + spaceid

	persistFunc := func() error {
		if c.UserShares[userid] == nil {
			c.UserShares[userid] = &UserShareCache{
				UserShares: map[string]*SpaceShareIDs{},
			}
		}

		if c.UserShares[userid].UserShares[ssid] != nil {
			// remove share id
			c.UserShares[userid].UserShares[ssid].Mtime = time.Now()
			delete(c.UserShares[userid].UserShares[ssid].IDs, shareID)
		}

		return c.Persist(ctx, userid)
	}

	err := persistFunc()
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := c.syncWithLock(ctx, userid); err != nil {
			return err
		}
		err = persistFunc()
	}

	return err
}

// List return the list of spaces/shares for the given user/group
func (c *Cache) List(ctx context.Context, userid string) (map[string]SpaceShareIDs, error) {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.lockUser(userid)
	span.End()
	span.SetAttributes(attribute.String("cs3.userid", userid))
	defer unlock()
	if err := c.syncWithLock(ctx, userid); err != nil {
		return nil, err
	}

	r := map[string]SpaceShareIDs{}
	if c.UserShares[userid] == nil {
		return r, nil
	}

	for ssid, cached := range c.UserShares[userid].UserShares {
		r[ssid] = SpaceShareIDs{
			Mtime: cached.Mtime,
			IDs:   cached.IDs,
		}
	}
	return r, nil
}

func (c *Cache) syncWithLock(ctx context.Context, userID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Sync")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.userid", userID))

	log := appctx.GetLogger(ctx).With().Str("userID", userID).Logger()

	c.initializeIfNeeded(userID, "")

	userCreatedPath := c.userCreatedPath(userID)
	span.AddEvent("updating cache")
	//  - update cached list of created shares for the user in memory if changed
	dlres, err := c.storage.Download(ctx, metadata.DownloadRequest{
		Path:        userCreatedPath,
		IfNoneMatch: []string{c.UserShares[userID].etag},
	})

	switch err.(type) {
	case nil:
		// continue
	case errtypes.NotFound:
		span.SetStatus(codes.Ok, "")
		return nil
	default:
		span.SetStatus(codes.Error, fmt.Sprintf("Failed to download the share cache: %s", err.Error()))
		log.Error().Err(err).Msg("Failed to download the share cache")
		return err
	}
	newShareCache := &UserShareCache{}
	err = json.Unmarshal(dlres.Content, newShareCache)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("Failed to unmarshal the share cache: %s", err.Error()))
		log.Error().Err(err).Msg("Failed to unmarshal the share cache")
		return err
	}
	//newShareCache.Mtime = utils.TSToTime(info.Mtime)
	newShareCache.Mtime = dlres.Mtime
	newShareCache.etag = dlres.Etag
	_, err = diff.Diff(c.UserShares[userID], newShareCache)
	if err != nil {
		log.Error().Str("userid", userID).Err(err).Msg("sharecache diff failed")
	} else {
		// log.Debug().Str("userid", userID).Interface("changelog", changelog).Msg("sharecache diff")
	}
	c.UserShares[userID] = newShareCache
	span.SetStatus(codes.Ok, "")
	return nil
}

// Persist persists the data for one user/group to the storage
func (c *Cache) Persist(ctx context.Context, userid string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Persist")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.userid", userid))

	oldMtime := c.UserShares[userid].Mtime
	c.UserShares[userid].Mtime = time.Now()

	createdBytes, err := json.Marshal(c.UserShares[userid])
	if err != nil {
		c.UserShares[userid].Mtime = oldMtime
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	jsonPath := c.userCreatedPath(userid)
	if err := c.storage.MakeDirIfNotExist(ctx, path.Dir(jsonPath)); err != nil {
		c.UserShares[userid].Mtime = oldMtime
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err = c.storage.Upload(ctx, metadata.UploadRequest{
		Path:        jsonPath,
		Content:     createdBytes,
		IfMatchEtag: c.UserShares[userid].etag,
		//IfUnmodifiedSince: oldMtime,
		//MTime:             c.UserShares[userid].Mtime,
	}); err != nil {
		c.UserShares[userid].Mtime = oldMtime
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

func (c *Cache) userCreatedPath(userid string) string {
	return filepath.Join("/", c.namespace, userid, c.filename)
}

func (c *Cache) initializeIfNeeded(userid, ssid string) {
	if c.UserShares[userid] == nil {
		c.UserShares[userid] = &UserShareCache{
			UserShares: map[string]*SpaceShareIDs{},
		}
	}
	if ssid != "" && c.UserShares[userid].UserShares[ssid] == nil {
		c.UserShares[userid].UserShares[ssid] = &SpaceShareIDs{
			IDs: map[string]struct{}{},
		}
	}
}
