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
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"strconv"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/crypto"
)

// ErrNotBuilt is the answer of the parts of the data plane that are not built yet.
var ErrNotBuilt = errors.New("not available yet in this version of the CLI")

// Plane implements backend.DataPlane over HTTP.
type Plane struct {
	client *http.Client
	loc    backend.Locator
}

var _ backend.DataPlane = (*Plane)(nil)

// New returns the data plane. The client has no timeout of its own: the
// context of the command sets the deadline.
func New(client *http.Client, loc backend.Locator) *Plane {
	return &Plane{client: client, loc: loc}
}

func (p *Plane) Download(ctx context.Context, ref *provider.Reference, offset int64) (io.ReadCloser, int64, error) {
	t, err := p.loc.DownloadTarget(ctx, ref)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return nil, 0, err
	}
	maps.Copy(req.Header, t.Header)
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	res, err := p.client.Do(req)
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
	return nil, 0, transferError("download: GET", res)
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
	t, err := p.loc.UploadTarget(ctx, ref, size, ifMatch)
	if err != nil {
		return err
	}
	u, err := url.Parse(t.URL)
	if err != nil {
		return err
	}
	if checksum != "" && t.Checksums != nil {
		xsType, err := guessXS(checksum, t.Checksums)
		if err != nil {
			return err
		}
		xs, err := computeXS(xsType, open)
		if err != nil {
			return err
		}
		q := u.Query()
		q.Add("xs", xs)
		q.Add("xs_type", storageprovider.GRPC2PKGXS(xsType).String())
		u.RawQuery = q.Encode()
	}
	body, err := open()
	if err != nil {
		return err
	}
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	maps.Copy(req.Header, t.Header)
	res, err := p.client.Do(req)
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

func (p *Plane) UploadResumable(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string) error {
	return fmt.Errorf("TUS upload: %w", ErrNotBuilt)
}

func (p *Plane) Archive(ctx context.Context, refs []*provider.Reference, format string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("archive: %w", ErrNotBuilt)
}

func (p *Plane) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	return p.loc.OpenInApp(ctx, ref, opts)
}

func transferError(op string, res *http.Response) error {
	return errors.New(op + " request returned " + res.Status)
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
