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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
)

var tracer trace.Tracer

func init() {
	tracer = otel.Tracer("github.com/cs3org/reva/pkg/share/manager/jsoncs3/providercache")
}

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
	Shares map[string]*collaboration.Share

	Etag string
}

// UnmarshalJSON overrides the default unmarshaling
// Shares are tricky to unmarshal because they contain an interface (Grantee) which makes the json Unmarshal bail out
// To work around that problem we unmarshal into json.RawMessage in a first step and then try to manually unmarshal
// into the specific types in a second step.
func (s *Shares) UnmarshalJSON(data []byte) error {
	tmp := struct {
		Shares map[string]json.RawMessage
	}{}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

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
		err = json.Unmarshal(genericShare, groupShare) // is this a group share?
		if err == nil && groupShare.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
			s.Shares[id] = groupShare
			continue
		}

		invalidShare := &collaboration.Share{}
		err = json.Unmarshal(genericShare, invalidShare) // invalid
		if err == nil {
			s.Shares[id] = invalidShare
			continue
		}

		return err
	}

	return nil
}

// LockProvider locks the cache for a given space and returns an unlock function
func (c *Cache) LockProvider(providerID string) func() {
	v, _ := c.lockMap.LoadOrStore(providerID, &sync.Mutex{})
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

func (c *Cache) isSpaceCached(storageID, spaceID string) bool {
	return c.Providers[storageID] != nil && c.Providers[storageID].Spaces[spaceID] != nil
}

// Add adds a share to the cache
func (c *Cache) Add(ctx context.Context, storageID, spaceID, shareID string, share *collaboration.Share) error {
	ctx, span := tracer.Start(ctx, "Add")
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

	unlock := c.LockProvider(storageID)
	defer unlock()
	span.AddEvent("got lock")

	var err error
	if !c.isSpaceCached(storageID, spaceID) {
		err = c.syncWithLock(ctx, storageID, spaceID)
		if err != nil {
			return err
		}
	}

	persistFunc := func() error {
		c.Providers[storageID].Spaces[spaceID].Shares[shareID] = share

		return c.Persist(ctx, storageID, spaceID)
	}

	log := appctx.GetLogger(ctx).With().
		Str("hostname", os.Getenv("HOSTNAME")).
		Str("storageID", storageID).
		Str("spaceID", spaceID).
		Str("shareID", shareID).Logger()

	for retries := 100; retries > 0; retries-- {
		err = persistFunc()
		if err != nil {
			log.Debug().Msg("persisting failed. Retrying...")
			if err := c.syncWithLock(ctx, storageID, spaceID); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())

				return err
			}
		} else {
			break
		}
	}
	if err != nil {
		log.Error().Err(err).Msg("persisting failed. giving up.")
	}

	return err
}

// Remove removes a share from the cache
func (c *Cache) Remove(ctx context.Context, storageID, spaceID, shareID string) error {
	ctx, span := tracer.Start(ctx, "Remove")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID), attribute.String("cs3.shareid", shareID))

	unlock := c.LockProvider(storageID)
	defer unlock()
	span.AddEvent("got lock")

	if !c.isSpaceCached(storageID, spaceID) {
		err := c.syncWithLock(ctx, storageID, spaceID)
		if err != nil {
			return err
		}
	}

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
func (c *Cache) Get(ctx context.Context, storageID, spaceID, shareID string, skipSync bool) (*collaboration.Share, error) {
	ctx, span := tracer.Start(ctx, "Get")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID), attribute.String("cs3.shareid", shareID))

	unlock := c.LockProvider(storageID)
	defer unlock()
	span.AddEvent("got lock")

	if !skipSync {
		// sync cache, maybe our data is outdated
		err := c.syncWithLock(ctx, storageID, spaceID)
		if err != nil {
			return nil, err
		}
	}

	if c.Providers[storageID] == nil ||
		c.Providers[storageID].Spaces[spaceID] == nil {
		return nil, nil
	}
	return c.Providers[storageID].Spaces[spaceID].Shares[shareID], nil
}

// ListSpace returns the list of shares in a given space
func (c *Cache) ListSpace(ctx context.Context, storageID, spaceID string) (*Shares, error) {
	ctx, span := tracer.Start(ctx, "ListSpace")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID))

	unlock := c.LockProvider(storageID)
	defer unlock()
	span.AddEvent("got lock")

	// sync cache, maybe our data is outdated
	err := c.syncWithLock(ctx, storageID, spaceID)
	if err != nil {
		return nil, err
	}

	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		return &Shares{}, nil
	}

	shares := &Shares{
		Shares: maps.Clone(c.Providers[storageID].Spaces[spaceID].Shares),
		Etag:   c.Providers[storageID].Spaces[spaceID].Etag,
	}
	return shares, nil
}

// Persist persists the data of one space
func (c *Cache) Persist(ctx context.Context, storageID, spaceID string) error {
	ctx, span := tracer.Start(ctx, "Persist")
	defer span.End()
	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID))

	if c.Providers[storageID] == nil || c.Providers[storageID].Spaces[spaceID] == nil {
		span.AddEvent("nothing to persist")
		span.SetStatus(codes.Ok, "")
		return nil
	}
	span.SetAttributes(attribute.String("BeforeEtag", c.Providers[storageID].Spaces[spaceID].Etag))
	log := appctx.GetLogger(ctx).With().Str("storageID", storageID).Str("spaceID", spaceID).Logger()
	log = log.With().Str("BeforeEtag", c.Providers[storageID].Spaces[spaceID].Etag).Logger()

	createdBytes, err := json.Marshal(c.Providers[storageID].Spaces[spaceID])
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	jsonPath := spaceJSONPath(storageID, spaceID)
	if err := c.storage.MakeDirIfNotExist(ctx, path.Dir(jsonPath)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.String("etag", c.Providers[storageID].Spaces[spaceID].Etag))

	ur := metadata.UploadRequest{
		Path:        jsonPath,
		Content:     createdBytes,
		IfMatchEtag: c.Providers[storageID].Spaces[spaceID].Etag,
	}
	// when there is no etag in memory make sure the file has not been created on the server, see https://www.rfc-editor.org/rfc/rfc9110#field.if-match
	// > If the field value is "*", the condition is false if the origin server has a current representation for the target resource.
	if c.Providers[storageID].Spaces[spaceID].Etag == "" {
		ur.IfNoneMatch = []string{"*"}
		log.Debug().Msg("setting IfNoneMatch to *")
	} else {
		log.Debug().Msg("setting IfMatchEtag")
	}
	res, err := c.storage.Upload(ctx, ur)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Debug().Err(err).Msg("persisting provider cache failed")
		return err
	}
	c.Providers[storageID].Spaces[spaceID].Etag = res.Etag
	// FIXME read etag from upload
	span.SetStatus(codes.Ok, "")
	shares := []string{}
	for _, s := range c.Providers[storageID].Spaces[spaceID].Shares {
		shares = append(shares, s.GetId().GetOpaqueId())
	}
	log.Debug().Str("AfterEtag", c.Providers[storageID].Spaces[spaceID].Etag).Interface("Shares", shares).Msg("persisted provider cache")
	return nil
}

func (c *Cache) syncWithLock(ctx context.Context, storageID, spaceID string) error {
	ctx, span := tracer.Start(ctx, "syncWithLock")
	defer span.End()

	c.initializeIfNeeded(storageID, spaceID)

	span.SetAttributes(attribute.String("cs3.storageid", storageID), attribute.String("cs3.spaceid", spaceID), attribute.String("etag", c.Providers[storageID].Spaces[spaceID].Etag))
	log := appctx.GetLogger(ctx).With().Str("storageID", storageID).Str("spaceID", spaceID).Str("etag", c.Providers[storageID].Spaces[spaceID].Etag).Str("hostname", os.Getenv("HOSTNAME")).Logger()

	dlreq := metadata.DownloadRequest{
		Path: spaceJSONPath(storageID, spaceID),
	}
	// when we know an etag, only download if it changed remotely
	if c.Providers[storageID].Spaces[spaceID].Etag != "" {
		dlreq.IfNoneMatch = []string{c.Providers[storageID].Spaces[spaceID].Etag}
	}

	var dlres *metadata.DownloadResponse
	var err error
	downloadFunc := func() error {
		dlres, err = c.storage.Download(ctx, dlreq)
		switch err.(type) {
		case nil:
			return nil
		case errtypes.NotFound:
			span.AddEvent("not found")
			return nil
		case errtypes.NotModified:
			span.AddEvent("not modified")
			return nil
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, "downloading provider cache failed")
			return err
		}
	}
	err = downloadFunc()
	if err != nil {
		err = downloadFunc()
		if err != nil {
			log.Error().Err(err).Msg("downloading provider cache failed")
			return err
		}
	}
	if dlres == nil {
		span.AddEvent("nothing to update")
		span.SetStatus(codes.Ok, "")
		return nil
	}
	span.AddEvent("updating local cache")
	newShares := &Shares{}
	err = json.Unmarshal(dlres.Content, newShares)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "unmarshaling provider cache failed")
		log.Error().Err(err).Msg("unmarshaling provider cache failed")
		return err
	}
	newShares.Etag = dlres.Etag

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
