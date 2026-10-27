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

// Package data is the data plane of the CLI, for all transports. The
// backend of the transport gives the targets of the transfers (a Locator).
package data

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/crypto"
)

// DefaultChunkSize is the size of a TUS part when nothing sets another one.
const DefaultChunkSize = 8 << 20

// Plane implements backend.DataPlane.
type Plane struct {
	client *http.Client
	loc    backend.Locator
	// ChunkSize is the size of a TUS part. The server can make it smaller.
	ChunkSize int64
	// Renew, when set, replaces the credentials after a 401.
	Renew func(ctx context.Context) error
}

var _ backend.DataPlane = (*Plane)(nil)

// New returns the data plane. The client has no timeout of its own: the
// context of the command sets the deadline.
func New(client *http.Client, loc backend.Locator) *Plane {
	return &Plane{client: client, loc: loc}
}

// send retries once after a 401, with renewed credentials and a new target.
func (p *Plane) send(ctx context.Context, target func() (*backend.Target, error), build func(*backend.Target) (*http.Request, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		t, err := target()
		if err != nil {
			return nil, err
		}
		req, err := build(t)
		if err != nil {
			return nil, err
		}
		for k, v := range t.Header {
			if req.Header.Get(k) == "" {
				req.Header[k] = v
			}
		}
		res, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusUnauthorized || p.Renew == nil || attempt > 0 {
			return res, nil
		}
		res.Body.Close()
		if err := p.Renew(ctx); err != nil {
			return nil, err
		}
	}
}

func (p *Plane) get(ctx context.Context, op string, target func() (*backend.Target, error), offset int64) (io.ReadCloser, int64, error) {
	res, err := p.send(ctx, target, func(t *backend.Target) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
		if err == nil && offset > 0 {
			req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		}
		return req, err
	})
	if err != nil {
		return nil, 0, err
	}
	switch {
	case res.StatusCode == http.StatusPartialContent && offset > 0:
		return res.Body, res.ContentLength, nil
	case res.StatusCode == http.StatusOK:
		size := res.ContentLength
		if offset > 0 {
			// The server ignored the range.
			if _, err := io.CopyN(io.Discard, res.Body, offset); err != nil {
				res.Body.Close()
				return nil, 0, err
			}
			if size >= 0 {
				size -= offset
			}
		}
		return res.Body, size, nil
	}
	res.Body.Close()
	return nil, 0, transferError(op+": GET", res)
}

func (p *Plane) Download(ctx context.Context, ref *provider.Reference, offset int64) (io.ReadCloser, int64, error) {
	return p.get(ctx, "download", func() (*backend.Target, error) { return p.loc.DownloadTarget(ctx, ref) }, offset)
}

func (p *Plane) DownloadVersion(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, int64, error) {
	return p.get(ctx, "download version", func() (*backend.Target, error) { return p.loc.VersionTarget(ctx, ref, key) }, 0)
}

func (p *Plane) Upload(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string) error {
	return p.put(ctx, ref, open, size, checksum, "")
}

func (p *Plane) UploadIfUnchanged(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, etag string) error {
	return p.put(ctx, ref, open, size, "", etag)
}

func (p *Plane) UploadStream(ctx context.Context, ref *provider.Reference, r io.Reader, size int64) error {
	once := false
	return p.put(ctx, ref, func() (io.ReadCloser, error) {
		if once {
			return nil, errors.New("upload: a stream can be read only once")
		}
		once = true
		return io.NopCloser(r), nil
	}, size, "", "")
}

func (p *Plane) put(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum, ifMatch string) error {
	res, err := p.send(ctx, func() (*backend.Target, error) { return p.loc.UploadTarget(ctx, ref, size, ifMatch) },
		func(t *backend.Target) (*http.Request, error) {
			u, err := url.Parse(t.URL)
			if err != nil {
				return nil, err
			}
			var xsHeader string
			if checksum != "" && t.Checksums != nil {
				xsType, err := guessXS(checksum, t.Checksums)
				if err != nil {
					return nil, err
				}
				xs, err := computeXS(xsType, open)
				if err != nil {
					return nil, err
				}
				name := storageprovider.GRPC2PKGXS(xsType).String()
				if t.ChecksumHeader {
					xsHeader = strings.ToUpper(name) + ":" + xs
				} else {
					q := u.Query()
					q.Add("xs", xs)
					q.Add("xs_type", name)
					u.RawQuery = q.Encode()
				}
			}
			body, err := open()
			if err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), body)
			if err != nil {
				body.Close()
				return nil, err
			}
			req.ContentLength = size
			if size == 0 {
				// A zero length with a body would make net/http send it chunked.
				body.Close()
				req.Body = http.NoBody
			}
			if xsHeader != "" {
				req.Header.Set("OC-Checksum", xsHeader)
			}
			return req, nil
		})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusPreconditionFailed:
		return &backend.StatusError{Code: rpc.Code_CODE_FAILED_PRECONDITION, Msg: "the file changed since it was read"}
	}
	return transferError("upload: PUT", res)
}

func (p *Plane) UploadResumable(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string, progress func(backend.Upload)) error {
	t, err := p.loc.ResumableTarget(ctx, ref, size, checksum)
	if err != nil {
		return err
	}
	return p.sendParts(ctx, backend.Upload{URL: t.URL, Path: ref.GetPath(), Size: size}, t.Header, open, progress)
}

func (p *Plane) ResumeUpload(ctx context.Context, up backend.Upload, open func() (io.ReadCloser, error), progress func(backend.Upload)) error {
	h, err := p.loc.AuthHeader(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, up.URL, nil)
	if err != nil {
		return err
	}
	maps.Copy(req.Header, h)
	req.Header.Set("Tus-Resumable", tusVersion)
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK, http.StatusNoContent:
	case http.StatusNotFound, http.StatusGone, http.StatusForbidden:
		return &backend.StatusError{Code: rpc.Code_CODE_NOT_FOUND, Msg: "the server does not know the upload any more"}
	default:
		return transferError("resume upload: HEAD", res)
	}
	if up.Offset, err = strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64); err != nil {
		return errors.New("resume upload: the server gives no offset")
	}
	return p.sendParts(ctx, up, h, open, progress)
}

const tusVersion = "1.0.0"

func (p *Plane) sendParts(ctx context.Context, up backend.Upload, h http.Header, open func() (io.ReadCloser, error), progress func(backend.Upload)) error {
	if progress == nil {
		progress = func(backend.Upload) {}
	}
	progress(up)
	if up.Offset >= up.Size {
		return nil
	}
	r, err := open()
	if err != nil {
		return err
	}
	defer r.Close()
	if s, ok := r.(io.Seeker); ok {
		_, err = s.Seek(up.Offset, io.SeekStart)
	} else {
		_, err = io.CopyN(io.Discard, r, up.Offset)
	}
	if err != nil {
		return err
	}
	chunk := cmp.Or(p.ChunkSize, DefaultChunkSize)
	if c, err := p.loc.Capabilities(ctx); err == nil && c.MaxChunkSize > 0 {
		chunk = min(chunk, c.MaxChunkSize)
	}
	for up.Offset < up.Size {
		n := min(chunk, up.Size-up.Offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, up.URL, io.NopCloser(io.LimitReader(r, n)))
		if err != nil {
			return err
		}
		req.ContentLength = n
		maps.Copy(req.Header, h)
		req.Header.Set("Tus-Resumable", tusVersion)
		req.Header.Set("Upload-Offset", strconv.FormatInt(up.Offset, 10))
		req.Header.Set("Content-Type", "application/offset+octet-stream")
		res, err := p.client.Do(req)
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			return transferError("upload: PATCH", res)
		}
		next := up.Offset + n
		if v, err := strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64); err == nil {
			next = v
		}
		if next <= up.Offset {
			return errors.New("upload: the server stopped taking data")
		}
		up.Offset = next
		progress(up)
	}
	return nil
}

func (p *Plane) Archive(ctx context.Context, refs []*provider.Reference, format string) (io.ReadCloser, error) {
	body, _, err := p.get(ctx, "archive", func() (*backend.Target, error) { return p.loc.ArchiveTarget(ctx, refs, format) }, 0)
	return body, err
}

func (p *Plane) Capabilities(ctx context.Context) (*backend.Capabilities, error) {
	return p.loc.Capabilities(ctx)
}

func (p *Plane) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	return p.loc.OpenInApp(ctx, ref, opts)
}

// statusCodes maps the HTTP answers that commands test by CS3 code.
var statusCodes = map[int]rpc.Code{
	http.StatusUnauthorized:        rpc.Code_CODE_UNAUTHENTICATED,
	http.StatusForbidden:           rpc.Code_CODE_PERMISSION_DENIED,
	http.StatusNotFound:            rpc.Code_CODE_NOT_FOUND,
	http.StatusLocked:              rpc.Code_CODE_LOCKED,
	http.StatusInsufficientStorage: rpc.Code_CODE_INSUFFICIENT_STORAGE,
}

func transferError(op string, res *http.Response) error {
	msg := op + " request returned " + res.Status
	if code, ok := statusCodes[res.StatusCode]; ok {
		return &backend.StatusError{Code: code, Msg: msg}
	}
	return errors.New(msg)
}

func computeXS(t provider.ResourceChecksumType, open func() (io.ReadCloser, error)) (string, error) {
	if t == provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET {
		return "", nil
	}
	r, err := open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	switch t {
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32:
		return crypto.ComputeAdler32XS(r)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5:
		return crypto.ComputeMD5XS(r)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:
		return crypto.ComputeSHA1XS(r)
	default:
		return "", fmt.Errorf("invalid checksum: %s", t)
	}
}

func guessXS(xsFlag string, availableXS []*provider.ResourceChecksumPriority) (provider.ResourceChecksumType, error) {
	invalid := provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID
	if xsFlag != "negotiate" {
		wanted := storageprovider.PKG2GRPCXS(xsFlag)
		if wanted == invalid {
			return invalid, fmt.Errorf("desired checksum is invalid: %s", xsFlag)
		}
		for _, xs := range availableXS {
			if xs.Type == wanted {
				return wanted, nil
			}
		}
		return invalid, fmt.Errorf("checksum not available server side: %s", xsFlag)
	}
	if len(availableXS) == 0 {
		return invalid, errors.New("no available xs for negotiating")
	}
	// A lower number is a higher priority.
	var chosen provider.ResourceChecksumType
	var maxPrio uint32 = math.MaxUint32
	for _, xs := range availableXS {
		if xs.Priority < maxPrio {
			maxPrio = xs.Priority
			chosen = xs.Type
		}
	}
	return chosen, nil
}
