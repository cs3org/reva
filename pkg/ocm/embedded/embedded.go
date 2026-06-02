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

// Package embedded transfers the contents of an OCM embedded share (an RO-Crate
// JSON payload listing plain or Zenodo files) to a WebDAV destination,
// streaming each referenced file in the background.
package embedded

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/studio-b12/gowebdav"
)

// Config holds the settings for transferring embedded share payloads.
type Config struct {
	// WebDAVURL is the destination WebDAV endpoint files are uploaded to.
	WebDAVURL string `mapstructure:"webdav_url"`
	// Timeout is the per-file ceiling, in seconds.
	Timeout int `mapstructure:"embedded_transfer_timeout"`
	// IdleTimeout, in seconds, aborts a file attempt if no bytes flow for that long.
	IdleTimeout int `mapstructure:"embedded_transfer_idle_timeout"`
	// Retries is the number of attempts per file (1 = no retry).
	Retries int `mapstructure:"embedded_transfer_retries"`
}

// ApplyDefaults fills in sensible defaults for any unset setting.
func (c *Config) ApplyDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 3600
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 120
	}
	if c.Retries <= 0 {
		c.Retries = 3
	}
}

// Transferrer processes an embedded share payload, transferring its referenced
// files to a destination. Drivers register an implementation via Register.
type Transferrer interface {
	Process(ctx context.Context, payload, destination string) error
}

// NewFunc builds a Transferrer from its driver configuration.
type NewFunc func(ctx context.Context, m map[string]any) (Transferrer, error)

// NewFuncs holds the registered embedded-transfer drivers.
var NewFuncs = map[string]NewFunc{}

// Register adds an embedded-transfer driver. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}

func init() {
	Register("webdav", New)
}

// driver is the built-in Transferrer: it downloads each file over HTTP and
// streams it to the configured WebDAV endpoint.
type driver struct {
	c Config
}

// New builds the built-in WebDAV embedded-transfer driver.
func New(ctx context.Context, m map[string]any) (Transferrer, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return &driver{c: c}, nil
}

// embeddedRetryBaseBackoff is the base per-file retry delay; it grows exponentially.
const embeddedRetryBaseBackoff = 2 * time.Second

// Process parses the RO-Crate payload and streams its files to destination in the
// background, using the bearer token from ctx for WebDAV auth. It errors only on a
// malformed payload.
func (d *driver) Process(ctx context.Context, payload, destination string) error {
	log := appctx.GetLogger(ctx)

	var c crate
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		return fmt.Errorf("unmarshal embedded payload: %w", err)
	}

	entries := crateEntries(log, c.Graph)
	if len(entries) == 0 {
		log.Debug().Str("destination", destination).Msg("embedded transfer: no transferable entries in payload")
		return nil
	}

	// Run detached so a large transfer doesn't block the accept request. The
	// captured token may expire mid-transfer, failing later files (best-effort).
	token := appctx.ContextMustGetToken(ctx)
	timeout := time.Duration(d.c.Timeout) * time.Second
	go d.transferEntries(log, token, destination, entries, timeout)

	return nil
}

// transferEntries streams each entry to the WebDAV destination. A file that keeps
// failing is skipped so one bad file doesn't abort the whole dataset.
func (d *driver) transferEntries(log *zerolog.Logger, token, destination string, entries []transferEntry, timeout time.Duration) {
	httpClient := &http.Client{}
	// Preemptive auth, not gowebdav's default auto-auth: auto-auth buffers each
	// upload body in memory to replay on an auth challenge, blowing up RAM on
	// multi-GB files. We pass the bearer token via the interceptor below.
	dav := gowebdav.NewAuthClient(d.c.WebDAVURL, gowebdav.NewPreemptiveAuth(&gowebdav.BasicAuth{}))
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

	idleTimeout := time.Duration(d.c.IdleTimeout) * time.Second

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

		if err := d.uploadEntryWithRetry(log, httpClient, dav, e, remotePath, timeout, idleTimeout); err != nil {
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

// uploadEntryWithRetry transfers one entry, retrying up to Retries times with
// exponential backoff. It returns the last error if all attempts fail.
func (d *driver) uploadEntryWithRetry(log *zerolog.Logger, httpClient *http.Client, dav *gowebdav.Client, e transferEntry, remotePath string, timeout, idleTimeout time.Duration) error {
	var err error
	for attempt := 1; attempt <= d.c.Retries; attempt++ {
		fileCtx, cancel := context.WithTimeout(context.Background(), timeout)
		err = uploadURLToWebDAV(fileCtx, log, httpClient, dav, e.srcURL, remotePath, e.sizeHint, idleTimeout)
		cancel()
		if err == nil {
			return nil
		}

		if attempt < d.c.Retries {
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

	// Never write a non-2xx body to the destination (e.g. Zenodo's JSON error
	// for 401/403/404/429) as if it were file data.
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

	// A known length lets net/http enforce it and surface a truncated download
	// as an error (retried) instead of storing a short file.
	length := sizeHint
	if length < 0 {
		length = resp.ContentLength
	}
	if length >= 0 {
		return dav.WriteStreamWithLength(remotePath, pr, length, 0644)
	}
	return dav.WriteStream(remotePath, pr, 0644)
}

// progressReader counts bytes read and timestamps the last non-empty read, so the
// watchdog can log throughput and detect a stall. Safe for concurrent use.
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
