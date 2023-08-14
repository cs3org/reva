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

package providercache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/r3labs/diff/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// name is the Tracer name used to identify this instrumentation library.
const tracerName = "providercache"

// Cache holds share information structured by provider and space
type Cache struct {
	lockMap sync.Map

	Providers map[string]*Spaces

	storage metadata.Storage
	ttl     time.Duration
}

// Spaces holds the share information for provider
type Spaces struct {
	Spaces map[string]*Shares
}

// Shares holds the share information of one space
type Shares struct {
	Shares   map[string]*collaboration.Share
	Mtime    time.Time
	etag     string
	nextSync time.Time
}

// UnmarshalJSON overrides the default unmarshaling
// Shares are tricky to unmarshal because they contain an interface (Grantee) which makes the json Unmarshal bail out
// To work around that problem we unmarshal into json.RawMessage in a first step and then try to manually unmarshal
// into the specific types in a second step.
func (s *Shares) UnmarshalJSON(data []byte) error {
	tmp := struct {
		Shares map[string]json.RawMessage
		Mtime  time.Time
	}{}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

	s.Mtime = tmp.Mtime
	s.Shares = make(map[string]*collaboration.Share, len(tmp.Shares))
	for id, genericShare := range tmp.Shares {
		userShare := &collaboration.Share{
			Grantee: &provider.Grantee{Id: &provider.Grantee_UserId{}},
		}
		err = json.Unmarshal(genericShare, userShare) // is this a user share?
		if err == nil && userShare.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
			s.Shares[id] = userShare
			continue
		}

		groupShare := &collaboration.Share{
			Grantee: &provider.Grantee{Id: &provider.Grantee_GroupId{}},
		}
		err = json.Unmarshal(genericShare, groupShare) // try to unmarshal to a group share if the user share unmarshalling failed
		if err != nil {
			return err
		}
		s.Shares[id] = groupShare
	}

	return nil
}

// LockSpace locks the cache for a given space and returns an unlock function
func (c *Cache) LockSpace(spaceID string) func() {
	v, _ := c.lockMap.LoadOrStore(spaceID, &sync.Mutex{})
	lock := v.(*sync.Mutex)

	lock.Lock()
	return func() { lock.Unlock() }
}

// New returns a new Cache instance
func New(s metadata.Storage, ttl time.Duration) Cache {
	return Cache{
		Providers: map[string]*Spaces{},
		storage:   s,
		ttl:       ttl,
		lockMap:   sync.Map{},
	}
}

// Add adds a share to the cache
func (c *Cache) Add(ctx context.Context, storageID, spaceID, shareID string, share *collaboration.Share) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.LockSpace(spaceID)
	span.End()
	span.SetAttributes(attribute.String("cs3.spaceid", spaceID))
	defer unlock()

	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		err := c.syncWithLock(ctx, storageID, spaceID)
		if err != nil {
			return err
		}
	}

	ctx, span = appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Add")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID), attribute.String("cs3.shareid", shareID))

	switch {
	case storageID == "":
		return fmt.Errorf("missing storage id")
	case spaceID == "":
		return fmt.Errorf("missing space id")
	case shareID == "":
		return fmt.Errorf("missing share id")
	}

	c.initializeIfNeeded(storageID, spaceID)

	beforeMTime := c.Providers[storageID].Spaces[spaceID].Mtime
	beforeShares := []string{}
	for _, s := range c.Providers[storageID].Spaces[spaceID].Shares {
		beforeShares = append(beforeShares, s.Id.OpaqueId)
	}

	persistFunc := func() error {
		c.Providers[storageID].Spaces[spaceID].Shares[shareID] = share

		return c.Persist(ctx, storageID, spaceID)
	}
	err := persistFunc()

	log := appctx.GetLogger(ctx).With().
		Str("hostname", os.Getenv("HOSTNAME")).
		Str("storageID", storageID).
		Str("spaceID", spaceID).
		Str("shareID", share.Id.OpaqueId).Logger()

	// if _, ok := err.(errtypes.IsPreconditionFailed); ok {
	if err != nil {
		log.Info().Msg("persisting failed. Retrying...")
		if err := c.syncWithLock(ctx, storageID, spaceID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			return err
		}

		err = persistFunc()
	}
	if err != nil {
		log.Error().Err(err).Msg("persisting failed unexpectedly")
	}

	afterMTime := c.Providers[storageID].Spaces[spaceID].Mtime
	afterShares := []string{}
	for _, s := range c.Providers[storageID].Spaces[spaceID].Shares {
		afterShares = append(afterShares, s.Id.OpaqueId)
	}
	log.Info().Interface("before", beforeShares).Interface("after", afterShares).Interface("beforeMTime", beforeMTime).Interface("afterMTime", afterMTime).Msg("providercache diff")

	return err
}

// Remove removes a share from the cache
func (c *Cache) Remove(ctx context.Context, storageID, spaceID, shareID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.LockSpace(spaceID)
	span.End()
	span.SetAttributes(attribute.String("cs3.spaceid", spaceID))
	defer unlock()

	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		err := c.syncWithLock(ctx, storageID, spaceID)
		if err != nil {
			return err
		}
	}

	ctx, span = appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Remove")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID), attribute.String("cs3.shareid", shareID))

	persistFunc := func() error {
		if c.Providers[storageID] == nil ||
			c.Providers[storageID].Spaces[spaceID] == nil {
			return nil
		}
		delete(c.Providers[storageID].Spaces[spaceID].Shares, shareID)

		return c.Persist(ctx, storageID, spaceID)
	}
	err := persistFunc()
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := c.syncWithLock(ctx, storageID, spaceID); err != nil {
			return err
		}
		err = persistFunc()
	}

	return err
}

// Get returns one entry from the cache
func (c *Cache) Get(ctx context.Context, storageID, spaceID, shareID string) (*collaboration.Share, error) {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.LockSpace(spaceID)
	span.End()
	span.SetAttributes(attribute.String("cs3.spaceid", spaceID))
	defer unlock()

	// sync cache, maybe our data is outdated
	err := c.syncWithLock(ctx, storageID, spaceID)
	if err != nil {
		return nil, err
	}

	if c.Providers[storageID] == nil ||
		c.Providers[storageID].Spaces[spaceID] == nil {
		return nil, nil
	}
	return c.Providers[storageID].Spaces[spaceID].Shares[shareID], nil
}

// ListSpace returns the list of shares in a given space
func (c *Cache) ListSpace(ctx context.Context, storageID, spaceID string) (*Shares, error) {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.LockSpace(spaceID)
	span.End()
	span.SetAttributes(attribute.String("cs3.spaceid", spaceID))
	defer unlock()

	// sync cache, maybe our data is outdated
	err := c.syncWithLock(ctx, storageID, spaceID)
	if err != nil {
		return nil, err
	}

	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		return &Shares{}, nil
	}
	return c.Providers[storageID].Spaces[spaceID], nil
}

// Persist persists the data of one space
func (c *Cache) Persist(ctx context.Context, storageID, spaceID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Persist")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID))

	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		span.SetStatus(codes.Ok, "no shares in provider or space")
		return nil
	}

	oldMtime := c.Providers[storageID].Spaces[spaceID].Mtime
	c.Providers[storageID].Spaces[spaceID].Mtime = time.Now()

	// FIXME there is a race when between this time now and the below Uploed another process also updates the file -> we need a lock
	createdBytes, err := json.Marshal(c.Providers[storageID].Spaces[spaceID])
	if err != nil {
		c.Providers[storageID].Spaces[spaceID].Mtime = oldMtime
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	jsonPath := spaceJSONPath(storageID, spaceID)
	if err := c.storage.MakeDirIfNotExist(ctx, path.Dir(jsonPath)); err != nil {
		c.Providers[storageID].Spaces[spaceID].Mtime = oldMtime
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err = c.storage.Upload(ctx, metadata.UploadRequest{
		Path:        jsonPath,
		Content:     createdBytes,
		IfMatchEtag: c.Providers[storageID].Spaces[spaceID].etag,
		// IfUnmodifiedSince: oldMtime,
		// MTime:             c.Providers[storageID].Spaces[spaceID].Mtime,
	}); err != nil {
		c.Providers[storageID].Spaces[spaceID].Mtime = oldMtime
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

// Sync updates the in-memory data with the data from the storage if it is outdated
func (c *Cache) Sync(ctx context.Context, storageID, spaceID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Grab lock")
	unlock := c.LockSpace(spaceID)
	span.End()
	span.SetAttributes(attribute.String("cs3.spaceid", spaceID))
	defer unlock()

	return c.syncWithLock(ctx, storageID, spaceID)
}

func (c *Cache) syncWithLock(ctx context.Context, storageID, spaceID string) error {
	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "Sync")
	defer span.End()

	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID))

	log := appctx.GetLogger(ctx).With().Str("storageID", storageID).Str("spaceID", spaceID).Logger()

	c.initializeIfNeeded(storageID, spaceID)

	jsonPath := spaceJSONPath(storageID, spaceID)
	// check mtime of /users/{userid}/created.json
	span.AddEvent("updating cache")
	//  - update cached list of created shares for the user in memory if changed
	dlres, err := c.storage.Download(ctx, metadata.DownloadRequest{
		Path:        jsonPath,
		IfNoneMatch: []string{c.Providers[storageID].Spaces[spaceID].etag},
	})
	switch err.(type) {
	case nil:
		// continue
	case errtypes.NotFound:
		span.SetStatus(codes.Ok, "")
		return nil
	default:
		span.SetStatus(codes.Error, fmt.Sprintf("Failed to download the provider cache: %s", err.Error()))
		log.Error().Err(err).Msg("Failed to download the provider cache")
		return err
	}
	newShares := &Shares{}
	err = json.Unmarshal(dlres.Content, newShares)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("Failed to unmarshal the provider cache: %s", err.Error()))
		log.Error().Err(err).Msg("Failed to unmarshal the provider cache")
		return err
	}
	//newShares.Mtime = utils.TSToTime(info.Mtime) this can only overwrite whatever mtime was in the data with the one from stat ... that seems fishy
	// we need to use the last-modified date of the json file and set that as the in memory mtime
	newShares.Mtime = dlres.Mtime
	newShares.etag = dlres.Etag

	if len(newShares.Shares) < len(c.Providers[storageID].Spaces[spaceID].Shares) {
		serverShares := []string{}
		localShares := []string{}
		for _, s := range newShares.Shares {
			serverShares = append(serverShares, s.Id.OpaqueId)
		}
		for _, s := range c.Providers[storageID].Spaces[spaceID].Shares {
			localShares = append(localShares, s.Id.OpaqueId)
		}
		changelog, err := diff.Diff(localShares, serverShares)
		if err != nil {
			log.Error().Err(err).Str("storageID", storageID).Str("spaceID", spaceID).Msg("providercache diff failed")
		} else {
			log.Debug().Str("storageID", storageID).Str("spaceID", spaceID).Interface("changelog", changelog).Msg("providercache diff")
		}
	}

	c.Providers[storageID].Spaces[spaceID] = newShares
	span.SetStatus(codes.Ok, "")
	return nil
}

func (c *Cache) initializeIfNeeded(storageID, spaceID string) {
	if c.Providers[storageID] == nil {
		c.Providers[storageID] = &Spaces{
			Spaces: map[string]*Shares{},
		}
	}
	if c.Providers[storageID].Spaces[spaceID] == nil {
		c.Providers[storageID].Spaces[spaceID] = &Shares{
			Shares: map[string]*collaboration.Share{},
		}
	}
}

func spaceJSONPath(storageID, spaceID string) string {
	return filepath.Join("/storages", storageID, spaceID+".json")
}
