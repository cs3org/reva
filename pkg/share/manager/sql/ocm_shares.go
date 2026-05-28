// Copyright 2018-2025 CERN
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

package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/studio-b12/gowebdav"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	_ "github.com/go-sql-driver/mysql"
)

type mgr struct {
	c  *Config
	db *gorm.DB
}

type crate struct {
	Graph []crateEntity `json:"@graph"`
}

type crateEntity struct {
	ID             string               `json:"@id"`
	Type           json.RawMessage      `json:"@type"`
	URL            json.RawMessage      `json:"url"`
	Name           string               `json:"name"`
	ContentSize    string               `json:"contentSize"`
	EncodingFormat string               `json:"encodingFormat"`
	Description    string               `json:"description"`
	Distribution   []zenodoDistribution `json:"distribution"`
}

type zenodoDistribution struct {
	Type           string `json:"@type"`
	ContentURL     string `json:"contentUrl"`
	EncodingFormat string `json:"encodingFormat"`
}

type transferEntry struct {
	srcURL         string
	name           string
	sizeHint       int64
	encodingFormat string
}

type idRef struct {
	ID string `json:"@id"`
}

func NewOCMShareManager(ctx context.Context, m map[string]any) (share.Repository, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("config", m).Msg("creating OCM share manager")
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	db, err := getDb(c)
	if err != nil {
		log.Debug().Err(err).Msg("error getting db client")
		return nil, err
	}

	err = db.AutoMigrate(&model.OcmShare{}, &model.OcmShareProtocol{},
		&model.OcmReceivedShare{}, &model.OcmReceivedShareProtocol{})
	if err != nil {
		log.Debug().Err(err).Msg("error migrating database")
		return nil, err
	}

	mgr := &mgr{
		c:  &c,
		db: db,
	}
	return mgr, nil
}

func formatUserID(u *userpb.UserId) string {
	return fmt.Sprintf("%s@%s", u.OpaqueId, u.Idp)
}

func (m *mgr) StoreShare(ctx context.Context, s *ocm.Share) (*ocm.Share, error) {

	id, err := createID(m.db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create id for OCM share")
	}
	err = m.db.Transaction(func(tx *gorm.DB) error {

		share := &model.OcmShare{
			Token:         s.Token,
			Instance:      s.ResourceId.StorageId,
			Inode:         s.ResourceId.OpaqueId,
			Name:          s.Name,
			ShareWith:     formatUserID(s.Grantee.GetUserId()),
			Owner:         s.Owner.OpaqueId,
			Initiator:     s.Creator.OpaqueId,
			Ctime:         s.Ctime.Seconds,
			Mtime:         s.Mtime.Seconds,
			RecipientType: convertFromCS3OCMShareType(s.RecipientType),
		}
		if s.Expiration != nil {
			share.Expiration = datatypes.NullTime{
				V:     time.Unix(int64(s.Expiration.Seconds), 0),
				Valid: true,
			}
		}
		share.Id = id
		share.ShareId = model.ShareID{ID: id}
		if err := tx.Create(share).Error; err != nil {
			return errors.Wrap(err, "failed to create OCM share")
		}
		for _, m := range s.AccessMethods {
			switch r := m.Term.(type) {
			case *ocm.AccessMethod_WebdavOptions:
				if err := storeWebDAVAccessMethod(tx, id, r); err != nil {
					return err
				}
			case *ocm.AccessMethod_WebappOptions:
				if err := storeWebappAccessMethod(tx, id, r); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.Id = &ocm.ShareId{OpaqueId: strconv.FormatInt(int64(id), 10)}
	return s, nil
}

func storeWebDAVAccessMethod(tx *gorm.DB, shareID uint, o *ocm.AccessMethod_WebdavOptions) error {
	accessMethod := &model.OcmShareProtocol{
		OcmShareID:   uint(shareID),
		Type:         model.WebDAVProtocol,
		Permissions:  int(permissions.OCSFromCS3Permission(o.WebdavOptions.Permissions)),
		AccessTypes:  accessTypesToInt(o.WebdavOptions.AccessTypes),
		Requirements: requirementsToJSON(o.WebdavOptions.Requirements),
	}

	err := tx.Create(accessMethod).Error
	if err != nil {
		return errors.Wrap(err, "failed to store webdav access method")
	}
	return nil
}

func storeWebappAccessMethod(tx *gorm.DB, shareID uint, o *ocm.AccessMethod_WebappOptions) error {
	accessMethod := &model.OcmShareProtocol{
		OcmShareID:  uint(shareID),
		Type:        model.WebappProtocol,
		Permissions: viewModeToInt(o.WebappOptions.ViewMode),
	}

	err := tx.Create(accessMethod).Error
	if err != nil {
		return errors.Wrap(err, "failed to store webapp access method")
	}
	return nil
}

func (m *mgr) GetShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.Share, error) {
	var (
		s   *ocm.Share
		err error
	)
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, user, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, user, ref.GetKey())
	case ref.GetToken() != "":
		s, err = m.getByToken(ctx, ref.GetToken())
	default:
		err = errtypes.NotFound(ref.String())
	}

	return s, err
}

func (m *mgr) DeleteShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) error {
	switch {
	case ref.GetId() != nil:
		return m.deleteByID(ctx, user, ref.GetId())
	case ref.GetKey() != nil:
		return m.deleteByKey(ctx, user, ref.GetKey())
	default:
		return errtypes.NotFound(ref.String())
	}
}

func (m *mgr) UpdateShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	switch {
	case ref.GetId() != nil:
		return m.updateShareByID(ctx, user, ref.GetId(), f...)
	case ref.GetKey() != nil:
		return m.updateShareByKey(ctx, user, ref.GetKey(), f...)
	default:
		return nil, errtypes.NotFound(ref.String())
	}
}

func (m *mgr) ListShares(ctx context.Context, user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	query := m.db.WithContext(ctx).Where("initiator = ? OR owner = ?", user.Id.OpaqueId, user.Id.OpaqueId)

	if len(filters) > 0 {
		filterQuery, filterParams, err := translateShareFilters(filters)
		if err != nil {
			return nil, err
		}
		if filterQuery != "" {
			query = query.Where(filterQuery, filterParams...)
		}
	}

	var shareModels []model.OcmShare
	if err := query.Find(&shareModels).Error; err != nil {
		return nil, err
	}

	shares := []*ocm.Share{}
	var ids []any
	for _, s := range shareModels {
		if s.DeletedAt.Valid {
			continue
		}
		share := convertToCS3OCMShare(&s, nil)
		shares = append(shares, share)
		ids = append(ids, s.Id)
	}

	am, err := m.getAccessMethodsByIds(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, share := range shares {
		if methods, ok := am[share.Id.OpaqueId]; ok {
			share.AccessMethods = methods
		}
	}

	return shares, nil
}

func (e crateEntity) URLString() string {
	if len(e.URL) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(e.URL, &s); err == nil {
		return s
	}

	var ref idRef
	if err := json.Unmarshal(e.URL, &ref); err == nil {
		return ref.ID
	}

	return ""
}

func (e crateEntity) HasType(want string) bool {
	if len(e.Type) == 0 {
		return false
	}

	var single string
	if err := json.Unmarshal(e.Type, &single); err == nil {
		return single == want
	}

	var many []string
	if err := json.Unmarshal(e.Type, &many); err == nil {
		for _, t := range many {
			if t == want {
				return true
			}
		}
	}

	return false
}

func (e crateEntity) IsTransferable() bool {
	if e.URLString() == "" {
		return false
	}

	return e.HasType("File") || e.HasType("ComputationalWorkflow") || e.HasType("SoftwareSourceCode")
}

// progressReader wraps a reader to track how many bytes have been read and when
// the last non-empty read happened, so a watchdog can log throughput and detect
// a stalled transfer. It is safe for concurrent use: the wrapped reader is read
// by net/http's body goroutine while the watchdog reads the counters.
type progressReader struct {
	r        io.Reader
	total    atomic.Int64
	lastData atomic.Int64 // unix nanos of the last read that returned data
}

func newProgressReader(r io.Reader) *progressReader {
	pr := &progressReader{r: r}
	pr.lastData.Store(time.Now().UnixNano())
	return pr
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.total.Add(int64(n))
		pr.lastData.Store(time.Now().UnixNano())
	}
	return n, err
}

// monitorTransfer periodically logs transfer progress and aborts the transfer
// (via cancel) if no data has flowed for idleTimeout. It exits when stop closes.
func monitorTransfer(log *zerolog.Logger, pr *progressReader, remotePath string, idleTimeout time.Duration, cancel context.CancelFunc, stop <-chan struct{}) {
	const interval = 15 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastTotal int64
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			total := pr.total.Load()
			rate := float64(total-lastTotal) / interval.Seconds() / (1024 * 1024)
			lastTotal = total
			idle := time.Since(time.Unix(0, pr.lastData.Load()))

			log.Debug().
				Str("remote", remotePath).
				Int64("bytes", total).
				Float64("mb_per_s", rate).
				Msg("Embedded transfer progress")

			if idle > idleTimeout {
				log.Warn().
					Str("remote", remotePath).
					Dur("idle", idle).
					Msg("embedded transfer: no data received, aborting attempt (will retry)")
				cancel()
				return
			}
		}
	}
}

func uploadURLToWebDAV(ctx context.Context, log *zerolog.Logger, httpClient *http.Client, dav *gowebdav.Client, srcURL, remotePath string, sizeHint int64, idleTimeout time.Duration) error {
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srcURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Only a 2xx with a body is the actual file. Non-success responses (e.g.
	// Zenodo's structured JSON for 401/403/404/429) must never be written to
	// the destination as if they were data.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := fmt.Errorf("GET %s failed: %s: %s", srcURL, resp.Status, string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			return &retryableError{err: err, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
		}
		return err
	}

	log.Debug().
		Str("remote", remotePath).
		Str("content_type", resp.Header.Get("Content-Type")).
		Str("content_disposition", resp.Header.Get("Content-Disposition")).
		Int64("content_length", resp.ContentLength).
		Msg("Embedded transfer: download response")

	pr := newProgressReader(resp.Body)
	stop := make(chan struct{})
	defer close(stop)
	go monitorTransfer(log, pr, remotePath, idleTimeout, cancel, stop)

	// With a known length gowebdav sets the request ContentLength, so net/http
	// streams exactly that many bytes and errors if the download is truncated —
	// a short read surfaces as an error here and is retried, never stored as a
	// complete file.
	length := sizeHint
	if length < 0 {
		length = resp.ContentLength
	}
	if length >= 0 {
		return dav.WriteStreamWithLength(remotePath, pr, length, 0644)
	}
	return dav.WriteStream(remotePath, pr, 0644)
}

// retryableError wraps an error whose operation should be retried, optionally
// after a server-suggested delay (e.g. from an HTTP 429 Retry-After header).
type retryableError struct {
	err        error
	retryAfter time.Duration
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// parseRetryAfter interprets an HTTP Retry-After header, which may be either a
// number of seconds or an HTTP date. Returns 0 if absent or unparseable.
func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func (m *mgr) ProcessEmbeddedShare(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().
		Interface("share", share).
		Str("destination", share.Destination).
		Msg("Processing received share")

	protocols, err := m.getProtocolsByIds(ctx, []any{share.Id.OpaqueId})
	if err != nil {
		return nil, err
	}

	pList, ok := protocols[share.Id.OpaqueId]
	if !ok {
		return nil, errtypes.NotFound("share not found")
	}

	var entries []transferEntry
	found := false
	for _, protocol := range pList {
		embedded, ok := protocol.Term.(*ocm.Protocol_EmbeddedOptions)
		if !ok {
			continue
		}
		found = true

		var c crate
		if err := json.Unmarshal([]byte(embedded.EmbeddedOptions.Payload), &c); err != nil {
			return nil, fmt.Errorf("unmarshal embedded payload: %w", err)
		}
		entries = crateEntries(log, c.Graph)
		break
	}
	if !found {
		return nil, errtypes.NotFound("protocol not found")
	}

	// The payload can reference many GB across dozens of files. Transferring it
	// inline would block (and could be cancelled by) the share-accept request,
	// so run it in the background and return immediately. The request token is
	// captured for WebDAV auth; a very long transfer may outlive it, in which
	// case the remaining files fail auth and are skipped (best-effort).
	token := appctx.ContextMustGetToken(ctx)
	timeout := time.Duration(m.c.EmbeddedTransferTimeout) * time.Second
	go m.transferEmbeddedEntries(log, token, share.Destination, entries, timeout)

	return share, nil
}

// embeddedRetryBaseBackoff is the base delay between per-file retry attempts;
// it grows exponentially with each subsequent attempt.
const embeddedRetryBaseBackoff = 2 * time.Second

// transferEmbeddedEntries streams each entry from its source URL to the WebDAV
// destination. It runs detached from the request context. Each file is retried
// a few times with backoff and, if it still fails, is logged and skipped so one
// bad file does not abort the whole dataset; each attempt is bounded by timeout
// so a stalled connection cannot hang forever.
func (m *mgr) transferEmbeddedEntries(log *zerolog.Logger, token, destination string, entries []transferEntry, timeout time.Duration) {
	httpClient := &http.Client{}
	// e.g. "https://cbox-ocisdev-rasmus.cern.ch/webdav/"
	// We authenticate via the bearer token set in the interceptor below, so we
	// use a preemptive authorizer rather than gowebdav's default auto-auth. The
	// auto-auth authorizer tees every non-seekable upload body into an in-memory
	// buffer (to be able to replay it on an auth challenge), which would copy
	// each multi-GB file fully into RAM and peg CPU on GC. The preemptive
	// authorizer leaves the body untouched, so uploads truly stream.
	dav := gowebdav.NewAuthClient(m.c.WebDAVURL, gowebdav.NewPreemptiveAuth(&gowebdav.BasicAuth{}))
	dav.SetTimeout(timeout)
	dav.SetInterceptor(func(method string, rq *http.Request) {
		rq.Header.Set("Authorization", "Bearer "+token)
		if method == http.MethodPut {
			rq.Header.Set("Content-Type", "application/octet-stream")
		}
	})

	if err := dav.Connect(); err != nil {
		log.Error().Err(err).Msg("embedded transfer: failed to connect to WebDAV")
		return
	}

	if err := dav.MkdirAll(destination, 0755); err != nil {
		log.Error().Err(err).Str("destination", destination).Msg("embedded transfer: failed to create destination directory")
		return
	}

	idleTimeout := time.Duration(m.c.EmbeddedTransferIdleTimeout) * time.Second

	log.Debug().
		Str("dest_path", destination).
		Int("entries", len(entries)).
		Msg("Starting embedded share transfer")

	var transferred, failed int
	for _, e := range entries {
		remotePath := path.Join(destination, e.name)

		log.Debug().
			Str("src", e.srcURL).
			Str("remote", remotePath).
			Int64("size_hint", e.sizeHint).
			Str("encoding_format", e.encodingFormat).
			Msg("Streaming embedded file to WebDAV")

		if err := m.uploadEntryWithRetry(log, httpClient, dav, e, remotePath, timeout, idleTimeout); err != nil {
			failed++
			log.Error().Err(err).
				Str("src", e.srcURL).
				Str("remote", remotePath).
				Msg("embedded transfer: skipping file after exhausting retries")
			continue
		}
		transferred++
	}

	log.Info().
		Str("destination", destination).
		Int("transferred", transferred).
		Int("failed", failed).
		Int("total", len(entries)).
		Msg("Finished embedded share transfer")
}

// uploadEntryWithRetry transfers a single entry, retrying on failure up to
// EmbeddedTransferRetries attempts with exponential backoff. Each attempt gets a
// fresh timeout-bounded context. It returns the last error if all attempts fail.
func (m *mgr) uploadEntryWithRetry(log *zerolog.Logger, httpClient *http.Client, dav *gowebdav.Client, e transferEntry, remotePath string, timeout, idleTimeout time.Duration) error {
	var err error
	for attempt := 1; attempt <= m.c.EmbeddedTransferRetries; attempt++ {
		fileCtx, cancel := context.WithTimeout(context.Background(), timeout)
		err = uploadURLToWebDAV(fileCtx, log, httpClient, dav, e.srcURL, remotePath, e.sizeHint, idleTimeout)
		cancel()
		if err == nil {
			return nil
		}

		if attempt < m.c.EmbeddedTransferRetries {
			backoff := embeddedRetryBaseBackoff << (attempt - 1)
			// Honor a server-requested delay (e.g. HTTP 429 Retry-After).
			var re *retryableError
			if errors.As(err, &re) && re.retryAfter > backoff {
				backoff = re.retryAfter
			}
			log.Warn().Err(err).
				Str("src", e.srcURL).
				Str("remote", remotePath).
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Msg("embedded transfer: file failed, retrying")
			time.Sleep(backoff)
		}
	}
	return err
}

// crateEntries walks the RO-Crate @graph and collects every transferable file.
// It handles two flavors that share the same graph envelope:
//   - ScienceMesh: a graph entity that is itself a File with a direct url.
//   - Zenodo: a graph entity (a schema.org Dataset) carrying a distribution[]
//     of DataDownload entries, each with a contentUrl.
func crateEntries(log *zerolog.Logger, graph []crateEntity) []transferEntry {
	var entries []transferEntry
	for _, e := range graph {
		if e.IsTransferable() {
			if entry, ok := scienceMeshEntry(log, e); ok {
				entries = append(entries, entry)
			}
		}
		for _, d := range e.Distribution {
			if entry, ok := zenodoEntry(log, d); ok {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func scienceMeshEntry(log *zerolog.Logger, e crateEntity) (transferEntry, bool) {
	srcURL := e.URLString()
	if srcURL == "" {
		return transferEntry{}, false
	}

	name := strings.TrimSpace(e.Name)
	if name == "" {
		name = path.Base(srcURL)
	}
	if name == "" || name == "." || name == "/" {
		log.Warn().
			Str("entity_id", e.ID).
			Str("src", srcURL).
			Msg("Skipping entity with unusable destination name")
		return transferEntry{}, false
	}

	size := int64(-1)
	if e.ContentSize != "" {
		if parsed, err := strconv.ParseInt(e.ContentSize, 10, 64); err == nil {
			size = parsed
		}
	}

	return transferEntry{
		srcURL:         srcURL,
		name:           name,
		sizeHint:       size,
		encodingFormat: e.EncodingFormat,
	}, true
}

func zenodoEntry(log *zerolog.Logger, d zenodoDistribution) (transferEntry, bool) {
	if d.Type != "DataDownload" || d.ContentURL == "" {
		return transferEntry{}, false
	}

	name := zenodoFilename(d.ContentURL)
	if name == "" || name == "." || name == "/" {
		log.Warn().
			Str("src", d.ContentURL).
			Msg("Skipping Zenodo distribution with unusable destination name")
		return transferEntry{}, false
	}

	return transferEntry{
		srcURL:         d.ContentURL,
		name:           name,
		sizeHint:       -1,
		encodingFormat: d.EncodingFormat,
	}, true
}

func zenodoFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return path.Base(rawURL)
	}
	return path.Base(strings.TrimSuffix(u.Path, "/content"))
}

func (m *mgr) StoreReceivedShare(ctx context.Context, s *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	if err := m.db.Transaction(func(tx *gorm.DB) error {

		receivedShare := &model.OcmReceivedShare{
			Name:          s.Name,
			RemoteShareID: s.RemoteShareId,
			ItemType:      convertFromCS3ResourceType(s.SharedResourceType),
			ShareWith:     s.Grantee.GetUserId().OpaqueId,
			Owner:         formatUserID(s.Owner),
			Initiator:     formatUserID(s.Creator),
			Ctime:         s.Ctime.Seconds,
			Mtime:         s.Mtime.Seconds,
			RecipientType: convertFromCS3OCMShareType(s.RecipientType),
			State:         convertFromCS3OCMShareState(s.State),
		}
		if s.Expiration != nil {
			receivedShare.Expiration = datatypes.NullTime{
				V:     time.Unix(int64(s.Expiration.Seconds), 0),
				Valid: true,
			}
		}

		id := tx.Create(receivedShare)
		err := id.Error
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				return share.ErrShareAlreadyExisting
			}
			return err
		}

		for _, p := range s.Protocols {
			switch r := p.Term.(type) {
			case *ocm.Protocol_WebdavOptions:
				if err := storeWebDAVProtocol(tx, int64(receivedShare.ID), r); err != nil {
					return err
				}
			case *ocm.Protocol_WebappOptions:
				if err := storeWebappProtocol(tx, int64(receivedShare.ID), r); err != nil {
					return err
				}
			case *ocm.Protocol_EmbeddedOptions:
				if err := storeEmbeddedProtocol(tx, int64(receivedShare.ID), r); err != nil {
					return err
				}
			}
		}

		s.Id = &ocm.ShareId{OpaqueId: fmt.Sprintf("%d", receivedShare.ID)}
		return nil
	}); err != nil {
		return nil, err
	}
	return s, nil
}

func storeWebDAVProtocol(tx *gorm.DB, shareID int64, o *ocm.Protocol_WebdavOptions) error {
	protocol := &model.OcmReceivedShareProtocol{
		OcmReceivedShareID: uint(shareID),
		Type:               model.WebDAVProtocol,
		Uri:                o.WebdavOptions.Uri,
		SharedSecret:       o.WebdavOptions.SharedSecret,
		Permissions:        int(permissions.OCSFromCS3Permission(o.WebdavOptions.Permissions.Permissions)),
		AccessTypes:        accessTypesToInt(o.WebdavOptions.AccessTypes),
		Requirements:       requirementsToJSON(o.WebdavOptions.Requirements),
	}

	if err := tx.Create(protocol).Error; err != nil {
		return err
	}
	return nil
}

func storeWebappProtocol(tx *gorm.DB, shareID int64, o *ocm.Protocol_WebappOptions) error {
	protocol := &model.OcmReceivedShareProtocol{
		OcmReceivedShareID: uint(shareID),
		Type:               model.WebappProtocol,
		Uri:                o.WebappOptions.Uri,
		SharedSecret:       o.WebappOptions.SharedSecret,
		Permissions:        viewModeToInt(o.WebappOptions.ViewMode),
	}

	if err := tx.Create(protocol).Error; err != nil {
		return err
	}
	return nil
}
func storeEmbeddedProtocol(tx *gorm.DB, shareID int64, o *ocm.Protocol_EmbeddedOptions) error {
	protocol := &model.OcmReceivedShareProtocol{
		OcmReceivedShareID: uint(shareID),
		Type:               model.EmbeddedProtocol,
		Payload:            datatypes.JSON([]byte(o.EmbeddedOptions.Payload)),
	}
	if err := tx.Create(protocol).Error; err != nil {
		return err
	}
	return nil
}

func requirementsToJSON(reqs []string) datatypes.JSON {
	if len(reqs) == 0 {
		return nil
	}
	b, err := json.Marshal(reqs)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func accessTypesToInt(at []ocm.AccessType) model.OcmAccessType {
	var bitmask model.OcmAccessType
	for _, t := range at {
		bitmask |= model.OcmAccessType(t)
	}
	return bitmask
}

func (m *mgr) ListReceivedShares(ctx context.Context, user *userpb.User, filters []*ocm.ListReceivedOCMSharesRequest_Filter) ([]*ocm.ReceivedShare, error) {
	query := m.db.WithContext(ctx).Where("share_with = ?", user.Id.OpaqueId)

	if len(filters) > 0 {
		filterQuery, filterParams, err := translateReceivedShareFilters(filters)
		if err != nil {
			return nil, err
		}
		if filterQuery != "" {
			query = query.Where(filterQuery, filterParams...)
		}
	}

	var receivedShareModels []model.OcmReceivedShare
	if err := query.Find(&receivedShareModels).Error; err != nil {
		return nil, err
	}
	shares := []*ocm.ReceivedShare{}
	var ids []any
	for _, s := range receivedShareModels {
		share := convertToCS3OCMReceivedShare(&s, nil)
		shares = append(shares, share)
		ids = append(ids, s.ID)
	}
	p, err := m.getProtocolsByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, share := range shares {
		if protocols, ok := p[share.Id.OpaqueId]; ok {
			share.Protocols = protocols
		}
	}

	return shares, nil
}

func (m *mgr) GetReceivedShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	var (
		s   *ocm.ReceivedShare
		err error
	)
	switch {
	case ref.GetId() != nil:
		s, err = m.getReceivedByID(ctx, user, ref.GetId())
	default:
		err = errtypes.NotFound(ref.String())
	}

	return s, err
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, user *userpb.User, s *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error) {
	shareID, err := strconv.Atoi(s.Id.OpaqueId)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	updates, updatedShare, err := m.translateUpdateFieldMask(s, fieldMask)
	if err != nil {
		return nil, err
	}

	result := m.db.WithContext(ctx).
		Model(&model.OcmReceivedShare{}).
		Where("id = ? AND share_with = ?", shareID, user.Id.OpaqueId).
		Updates(updates)

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, share.ErrShareNotFound
	}

	return updatedShare, nil
}

func (m *mgr) translateUpdateFieldMask(share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (map[string]any, *ocm.ReceivedShare, error) {
	updates := make(map[string]any)
	newShare := proto.Clone(share).(*ocm.ReceivedShare)

	for _, mask := range fieldMask.Paths {
		switch mask {
		case "state":
			updates["state"] = convertFromCS3OCMShareState(share.State)
			newShare.State = share.State
		default:
			return nil, nil, errtypes.NotSupported("updating " + mask + " is not supported")
		}
	}

	now := time.Now().Unix()
	updates["mtime"] = now
	newShare.Mtime = &typesv1beta1.Timestamp{
		Seconds: uint64(now),
	}

	return updates, newShare, nil
}

func (m *mgr) getByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) (*ocm.Share, error) {
	shareID, err := strconv.Atoi(id.OpaqueId)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	shareWith := formatUserID(user.Id)

	var shareModel model.OcmShare
	if err := m.db.WithContext(ctx).
		Where("id = ? AND (initiator = ? OR owner = ? OR share_with = ?)", shareID, user.Id.OpaqueId, user.Id.OpaqueId, shareWith).
		First(&shareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, int(shareModel.Id))
	if err != nil {
		return nil, err
	}

	if shareModel.DeletedAt.Valid {
		return nil, share.ErrShareNotFound
	}

	return convertToCS3OCMShare(&shareModel, am), nil

}

func (m *mgr) getByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey) (*ocm.Share, error) {
	var shareModel model.OcmShare
	if err := m.db.WithContext(ctx).
		Where("owner = ? AND instance = ? AND inode = ? AND share_with = ? AND (initiator = ? OR owner = ?)",
			key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId, formatUserID(key.Grantee.GetUserId()), user.Id.OpaqueId, user.Id.OpaqueId).
		First(&shareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, int(shareModel.Id))
	if err != nil {
		return nil, err
	}

	if shareModel.DeletedAt.Valid {
		return nil, share.ErrShareNotFound
	}

	return convertToCS3OCMShare(&shareModel, am), nil
}

func (m *mgr) getByToken(ctx context.Context, token string) (*ocm.Share, error) {
	var shareModel model.OcmShare
	if err := m.db.WithContext(ctx).
		Where("token = ?", token).
		First(&shareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, int(shareModel.Id))
	if err != nil {
		return nil, err
	}

	if shareModel.DeletedAt.Valid {
		return nil, share.ErrShareNotFound
	}

	return convertToCS3OCMShare(&shareModel, am), nil
}

func (m *mgr) getAccessMethods(ctx context.Context, id int) ([]*ocm.AccessMethod, error) {
	var modelAMs []model.OcmShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_share_id = ?", id).
		Find(&modelAMs).Error; err != nil {
		return nil, err
	}

	var methods []*ocm.AccessMethod
	for _, am := range modelAMs {
		methods = append(methods, convertToCS3AccessMethod(&am))
	}

	return methods, nil
}

func (m *mgr) deleteByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) error {
	shareID, err := strconv.Atoi(id.OpaqueId)
	if err != nil {
		return errtypes.BadRequest("invalid share ID")
	}

	result := m.db.WithContext(ctx).
		Where("id = ? AND (owner = ? OR initiator = ?)", shareID, user.Id.OpaqueId, user.Id.OpaqueId).
		Delete(&model.OcmShare{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return share.ErrShareNotFound
	}

	return nil
}

func (m *mgr) deleteByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey) error {
	result := m.db.WithContext(ctx).
		Where("owner = ? AND instance = ? AND inode = ? AND share_with = ? AND (initiator = ? OR owner = ?)",
			key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId, formatUserID(key.Grantee.GetUserId()), user.Id.OpaqueId, user.Id.OpaqueId).
		Delete(&model.OcmShare{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return share.ErrShareNotFound
	}

	return nil
}

func (m *mgr) queriesUpdatesOnShare(ctx context.Context, id *ocm.ShareId, f ...*ocm.UpdateOCMShareRequest_UpdateField) (map[string]any, []func(*gorm.DB) error, error) {
	var updates map[string]any
	var accessMethodUpdates []func(*gorm.DB) error

	for _, field := range f {
		switch u := field.Field.(type) {
		case *ocm.UpdateOCMShareRequest_UpdateField_Expiration:
			if updates == nil {
				updates = make(map[string]any)
			}
			updates["expiration"] = u.Expiration.Seconds
		case *ocm.UpdateOCMShareRequest_UpdateField_AccessMethods:
			switch t := u.AccessMethods.Term.(type) {
			case *ocm.AccessMethod_WebdavOptions:
				accessMethodUpdates = append(accessMethodUpdates, func(tx *gorm.DB) error {
					return tx.Model(&model.OcmShareProtocol{}).
						Where("ocm_share_id = ? AND type = ?", id.OpaqueId, model.WebDAVProtocol).
						Update("permissions", int(permissions.RoleFromResourcePermissions(t.WebdavOptions.Permissions).OCSPermissions())).Error
				})
			case *ocm.AccessMethod_WebappOptions:
				accessMethodUpdates = append(accessMethodUpdates, func(tx *gorm.DB) error {
					return tx.Model(&model.OcmShareProtocol{}).
						Where("ocm_share_id = ? AND type = ?", id.OpaqueId, model.WebappProtocol).
						Update("permissions", int(t.WebappOptions.ViewMode)).Error
				})
			}
		}
	}

	return updates, accessMethodUpdates, nil
}

func (m *mgr) updateShareByID(ctx context.Context, user *userpb.User, id *ocm.ShareId, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	shareID, err := strconv.Atoi(id.OpaqueId)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	currentMethods, err := m.getAccessMethods(ctx, shareID)
	if err != nil {
		return nil, err
	}

	if err := validateImmutableFields(currentMethods, f...); err != nil {
		return nil, err
	}

	updates, accessMethodUpdates, err := m.queriesUpdatesOnShare(ctx, id, f...)
	if err != nil {
		return nil, err
	}

	if updates == nil {
		updates = make(map[string]any)
	}

	now := time.Now().Unix()
	updates["mtime"] = now

	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.OcmShare{}).
			Where("id = ? AND (initiator = ? OR owner = ?)", shareID, user.Id.OpaqueId, user.Id.OpaqueId).
			Updates(updates)

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return share.ErrShareNotFound
		}

		for _, updateFunc := range accessMethodUpdates {
			if err := updateFunc(tx); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return m.getByID(ctx, user, id)
}

// validateImmutableFields checks that a partial update does not attempt to
// change Requirements or AccessTypes, which are immutable after creation.
// Omitted or empty slices mean "preserve stored value" and are allowed.
// Identical non-empty slices are a no-op. Different non-empty slices are rejected.
func validateImmutableFields(currentMethods []*ocm.AccessMethod, f ...*ocm.UpdateOCMShareRequest_UpdateField) error {
	for _, field := range f {
		u, ok := field.Field.(*ocm.UpdateOCMShareRequest_UpdateField_AccessMethods)
		if !ok {
			continue
		}
		wdav, ok := u.AccessMethods.Term.(*ocm.AccessMethod_WebdavOptions)
		if !ok {
			continue
		}

		var storedReqs []string
		var storedATs []ocm.AccessType
		for _, m := range currentMethods {
			if existing, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
				storedReqs = existing.WebdavOptions.Requirements
				storedATs = existing.WebdavOptions.AccessTypes
				break
			}
		}

		if err := checkImmutableStringSlice(storedReqs, wdav.WebdavOptions.Requirements, "requirements"); err != nil {
			return err
		}
		if err := checkImmutableAccessTypes(storedATs, wdav.WebdavOptions.AccessTypes, "access_types"); err != nil {
			return err
		}
	}
	return nil
}

func checkImmutableStringSlice(stored, incoming []string, field string) error {
	if len(incoming) == 0 {
		return nil
	}
	if len(stored) == 0 {
		return errtypes.BadRequest(field + " cannot be set on update; they are immutable after creation")
	}
	if len(stored) != len(incoming) {
		return errtypes.BadRequest(field + " are immutable after creation")
	}
	for i := range stored {
		if stored[i] != incoming[i] {
			return errtypes.BadRequest(field + " are immutable after creation")
		}
	}
	return nil
}

func checkImmutableAccessTypes(stored, incoming []ocm.AccessType, field string) error {
	if len(incoming) == 0 {
		return nil
	}
	if len(stored) == 0 {
		return errtypes.BadRequest(field + " cannot be set on update; they are immutable after creation")
	}
	if len(stored) != len(incoming) {
		return errtypes.BadRequest(field + " are immutable after creation")
	}
	for i := range stored {
		if stored[i] != incoming[i] {
			return errtypes.BadRequest(field + " are immutable after creation")
		}
	}
	return nil
}

func (m *mgr) updateShareByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	share, err := m.getByKey(ctx, user, key)
	if err != nil {
		return nil, err
	}
	return m.updateShareByID(ctx, user, share.Id, f...)
}

func translateShareFilters(filters []*ocm.ListOCMSharesRequest_Filter) (string, []any, error) {
	var (
		filterQuery strings.Builder
		params      []any
	)

	grouped := groupFiltersByType(filters)

	var count int
	for _, lst := range grouped {
		for n, f := range lst {
			switch filter := f.Term.(type) {
			case *ocm.ListOCMSharesRequest_Filter_ResourceId:
				filterQuery.WriteString("instance = ? AND inode = ?")
				params = append(params, filter.ResourceId.StorageId, filter.ResourceId.OpaqueId)
			case *ocm.ListOCMSharesRequest_Filter_Creator:
				filterQuery.WriteString("initiator = ?")
				params = append(params, filter.Creator.OpaqueId)
			case *ocm.ListOCMSharesRequest_Filter_Owner:
				filterQuery.WriteString("owner= ? ")
				params = append(params, filter.Owner.OpaqueId)
			default:
				return "", nil, errtypes.BadRequest("unknown filter")
			}

			if n != len(lst)-1 {
				filterQuery.WriteString(" OR ")
			}
		}
		if count != len(grouped)-1 {
			filterQuery.WriteString(" AND ")
		}
		count++
	}

	return filterQuery.String(), params, nil
}

func translateReceivedShareFilters(filters []*ocm.ListReceivedOCMSharesRequest_Filter) (string, []any, error) {
	var (
		filterQuery strings.Builder
		params      []any
	)

	grouped := groupReceivedFiltersByType(filters)

	var count int
	for _, lst := range grouped {
		for n, f := range lst {
			switch filter := f.Term.(type) {
			case *ocm.ListReceivedOCMSharesRequest_Filter_SharedResourceType:
				filterQuery.WriteString("item_type = ?")
				params = append(params, translateSharedResourceTypeToItemType(filter.SharedResourceType))
			case *ocm.ListReceivedOCMSharesRequest_Filter_Creator:
				filterQuery.WriteString("initiator = ?")
				params = append(params, filter.Creator.OpaqueId)
			case *ocm.ListReceivedOCMSharesRequest_Filter_Owner:
				filterQuery.WriteString("owner = ?")
				params = append(params, filter.Owner.OpaqueId)
			default:
				return "", nil, errtypes.BadRequest("unknown filter")
			}

			if n != len(lst)-1 {
				filterQuery.WriteString(" OR ")
			}
		}
		if count != len(grouped)-1 {
			filterQuery.WriteString(" AND ")
		}
		count++
	}

	return filterQuery.String(), params, nil
}

func translateSharedResourceTypeToItemType(t ocm.SharedResourceType) model.ItemType {
	switch t {
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE:
		return model.ItemTypeFile
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER:
		return model.ItemTypeFolder
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED:
		return model.ItemTypeEmbedded
	default:
		return model.ItemTypeFile
	}
}

func groupFiltersByType(filters []*ocm.ListOCMSharesRequest_Filter) map[ocm.ListOCMSharesRequest_Filter_Type][]*ocm.ListOCMSharesRequest_Filter {
	m := make(map[ocm.ListOCMSharesRequest_Filter_Type][]*ocm.ListOCMSharesRequest_Filter)
	for _, f := range filters {
		m[f.Type] = append(m[f.Type], f)
	}
	return m
}

func groupReceivedFiltersByType(filters []*ocm.ListReceivedOCMSharesRequest_Filter) map[ocm.ListReceivedOCMSharesRequest_Filter_Type][]*ocm.ListReceivedOCMSharesRequest_Filter {
	m := make(map[ocm.ListReceivedOCMSharesRequest_Filter_Type][]*ocm.ListReceivedOCMSharesRequest_Filter)
	for _, f := range filters {
		m[f.Type] = append(m[f.Type], f)
	}
	return m
}
func (m *mgr) getAccessMethodsByIds(ctx context.Context, ids []any) (map[string][]*ocm.AccessMethod, error) {
	methods := make(map[string][]*ocm.AccessMethod)
	if len(ids) == 0 {
		return methods, nil
	}

	var mProtos []model.OcmShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_share_id IN ?", ids).
		Find(&mProtos).Error; err != nil {
		return nil, err
	}

	for _, p := range mProtos {
		method := convertToCS3AccessMethod(&p)
		shareID := strconv.FormatUint(uint64(p.OcmShareID), 10)
		methods[shareID] = append(methods[shareID], method)
	}

	return methods, nil
}

func (m *mgr) getProtocolsByIds(ctx context.Context, ids []any) (map[string][]*ocm.Protocol, error) {
	protocols := make(map[string][]*ocm.Protocol)
	if len(ids) == 0 {
		return protocols, nil
	}

	var mrProtos []model.OcmReceivedShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_received_share_id IN ?", ids).
		Find(&mrProtos).Error; err != nil {
		return nil, err
	}

	for _, p := range mrProtos {
		protocol := convertToCS3Protocol(&p)
		shareID := strconv.FormatUint(uint64(p.OcmReceivedShareID), 10)
		protocols[shareID] = append(protocols[shareID], protocol)
	}

	return protocols, nil
}

func (m *mgr) getReceivedByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) (*ocm.ReceivedShare, error) {
	shareID, err := strconv.Atoi(id.OpaqueId)

	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	var receivedShareModel model.OcmReceivedShare
	if err := m.db.WithContext(ctx).
		Where("id = ? AND share_with = ?", shareID, user.Id.OpaqueId).
		First(&receivedShareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}
	p, err := m.getProtocols(ctx, int(receivedShareModel.ID))
	if err != nil {
		return nil, err
	}

	return convertToCS3OCMReceivedShare(&receivedShareModel, p), nil
}

func (m *mgr) getProtocols(ctx context.Context, id int) ([]*ocm.Protocol, error) {
	var protocolModels []model.OcmReceivedShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_received_share_id = ?", id).
		Find(&protocolModels).Error; err != nil {
		return nil, err
	}

	var protocols []*ocm.Protocol
	for _, p := range protocolModels {
		protocols = append(protocols, convertToCS3Protocol(&p))
	}

	return protocols, nil
}
