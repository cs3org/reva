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

package ocm

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"time"

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

// Logged access modes. The shared secret and the access token are never
// written to log fields.
const (
	modeExchange = "exchange"
	modeBearer   = "bearer"
	modeLegacy   = "legacy"
	modeStrict   = "strict"
)

// Drop the cached access token this long before expires_in so a request
// does not race the sender's expiry. A one-hour token is then refreshed
// about once an hour.
const exchangeTTLSafetyMargin = 60 * time.Second

// Skip opportunistic exchange for this long after discovery, the token
// endpoint, or the sender's WebDAV rejects the exchanged token. Long enough
// that a broken sender is not hit on every request, short enough to notice
// a sender that recovers. Busy shares still reuse a cached legacy client.
var exchangeFailureTTL = 5 * time.Minute

// errExchangeNotOffered means the sender did not advertise exchange-token
// with a token endpoint. Callers fall back to the shared secret and do not
// remember this as a failure.
var errExchangeNotOffered = errors.New("ocm token exchange is not offered")

type senderOffer struct {
	tokenEndpoint string
	capabilities  []string
}

func exchangeCacheTTL(expiresIn int64) time.Duration {
	if expiresIn < 0 {
		expiresIn = 0
	}
	ttl := time.Duration(expiresIn)*time.Second - exchangeTTLSafetyMargin
	if ttl < time.Second {
		return time.Second
	}
	return ttl
}

func offersTokenExchange(offer *senderOffer) bool {
	if offer == nil || offer.tokenEndpoint == "" {
		return false
	}
	return slices.Contains(offer.capabilities, "exchange-token")
}

// lookupSenderOffer reads the sender discovery cache. Offers are stored
// under the discovery cache TTL, including a negative entry whose
// tokenEndpoint is empty when the sender has no tokenEndPoint.
// offersTokenExchange treats that entry as not offered.
func (d *driver) lookupSenderOffer(ctx context.Context, share *ocmpb.ReceivedShare) (*senderOffer, error) {
	idp := strings.TrimSpace(share.GetCreator().GetIdp())
	if idp == "" {
		return nil, errtypes.NotFound("share has no sender idp")
	}
	if entry, err := d.discoveryCache.Get(idp); err == nil {
		return entry.(*senderOffer), nil
	}

	disco, err := d.ocmClient.Discover(ctx, idp)
	if err != nil {
		return nil, errors.Wrap(err, "OCM discovery failed for "+idp)
	}
	offer := &senderOffer{}
	if disco != nil {
		offer.tokenEndpoint = disco.TokenEndPoint
		offer.capabilities = append([]string{}, disco.Capabilities...)
	}
	_ = d.discoveryCache.Set(idp, offer)
	return offer, nil
}

func (d *driver) tryExchange(
	ctx context.Context,
	share *ocmpb.ReceivedShare,
	endpoint, secret string,
) (*cachedClient, error) {
	offer, err := d.lookupSenderOffer(ctx, share)
	if err != nil {
		if errors.Is(err, client.ErrPolicyViolation) {
			return nil, err
		}
		if _, ok := err.(errtypes.IsNotFound); ok {
			return nil, errExchangeNotOffered
		}
		return nil, err
	}
	if !offersTokenExchange(offer) {
		return nil, errExchangeNotOffered
	}

	accessToken, expiresIn, err := d.exchangeAccessToken(ctx, offer.tokenEndpoint, secret)
	if err != nil {
		return nil, err
	}
	ttl := exchangeCacheTTL(expiresIn)
	header := "Bearer " + accessToken
	return &cachedClient{
		client:     d.newWebDAVClient(endpoint, map[string]string{"Authorization": header}),
		share:      share,
		authHeader: header,
		mode:       modeExchange,
		expiresAt:  time.Now().Add(ttl),
	}, nil
}

// resolveOpportunistic caches an exchanged client when the sender offers a
// working token endpoint. A nil client and nil error means the caller should
// use the legacy shared secret. Policy violations are returned unchanged.
func (d *driver) resolveOpportunistic(
	ctx context.Context,
	share *ocmpb.ReceivedShare,
	endpoint, secret, shareID string,
) (*cachedClient, error) {
	if d.exchangeRememberedDown(shareID) {
		return nil, nil
	}
	cc, err := d.tryExchange(ctx, share, endpoint, secret)
	if err != nil {
		if errors.Is(err, client.ErrPolicyViolation) {
			return nil, err
		}
		if errors.Is(err, errExchangeNotOffered) {
			return nil, nil
		}
		d.rememberExchangeFailure(shareID)
		appctx.GetLogger(ctx).Info().
			Str("shareId", shareID).
			Msg("OCM token exchange failed, trying shared secret")
		return nil, nil
	}
	d.storeClient(shareID, cc, time.Until(cc.expiresAt))
	return cc, nil
}

func (d *driver) storeClient(shareID string, cc *cachedClient, ttl time.Duration) {
	if ttl < time.Second {
		ttl = time.Second
	}
	_ = d.ccache.SetWithTTL(shareID, cc, ttl)
}

func (d *driver) cachedShareClient(shareID string) (*cachedClient, bool) {
	if d.ccache == nil || shareID == "" {
		return nil, false
	}
	entry, err := d.ccache.Get(shareID)
	if err != nil {
		return nil, false
	}
	cc, ok := entry.(*cachedClient)
	if !ok || cc == nil {
		return nil, false
	}
	if cc.mode == modeExchange && !cc.expiresAt.IsZero() && !time.Now().Before(cc.expiresAt) {
		_ = d.ccache.Remove(shareID)
		return nil, false
	}
	return cc, true
}

func (d *driver) takeCachedClient(shareID string) (*cachedClient, bool) {
	cc, ok := d.cachedShareClient(shareID)
	if !ok {
		return nil, false
	}
	if cc.mode == modeExchange && d.exchangeRememberedDown(shareID) {
		_ = d.ccache.Remove(shareID)
		return nil, false
	}
	return cc, true
}

func (d *driver) sessionMode(shareID string, protocols []*ocmpb.Protocol) string {
	if requiresExchange(protocols) {
		return modeStrict
	}
	if cc, ok := d.takeCachedClient(shareID); ok && cc.mode != "" {
		return cc.mode
	}
	return ""
}

func (d *driver) exchangeRememberedDown(shareID string) bool {
	if d.exchangeMiss == nil || shareID == "" {
		return false
	}
	_, err := d.exchangeMiss.Get(shareID)
	return err == nil
}

func (d *driver) rememberExchangeFailure(shareID string) {
	if d.exchangeMiss == nil || shareID == "" {
		return
	}
	_ = d.exchangeMiss.SetWithTTL(shareID, true, exchangeFailureTTL)
}

func (d *driver) legacyWebDAVClient(
	ctx context.Context,
	shareID string,
	share *ocmpb.ReceivedShare,
	endpoint, secret, rel string,
) (*gowebdav.Client, *ocmpb.ReceivedShare, string, error) {
	log := appctx.GetLogger(ctx)
	bearerHdr := "Bearer " + secret
	c := d.newWebDAVClient(endpoint, map[string]string{"Authorization": bearerHdr})
	_, err := c.Stat("")
	if errors.Is(err, client.ErrPolicyViolation) {
		return nil, nil, "", err
	}
	authHdr := bearerHdr
	mode := modeBearer
	if err != nil {
		basicHdr := "Basic " + base64.StdEncoding.EncodeToString([]byte(secret+":"))
		c = d.newWebDAVClient(endpoint, map[string]string{"Authorization": basicHdr})
		_, err2 := c.Stat("")
		if errors.Is(err2, client.ErrPolicyViolation) {
			return nil, nil, "", err2
		}
		if err2 != nil {
			log.Info().
				Any("former_error", err).
				Err(err2).
				Str("endpoint", endpoint).
				Str("shareId", share.GetId().GetOpaqueId()).
				Msg("failed accessing OCM share")
			return nil, nil, "", errtypes.InvalidCredentials("error accessing OCM share: " + err2.Error())
		}
		authHdr = basicHdr
		mode = modeLegacy
		log.Info().
			Str("endpoint", endpoint).
			Str("shareId", share.GetId().GetOpaqueId()).
			Str("mode", mode).
			Msg("access to remote OCM share succeeded")
	} else {
		log.Info().
			Str("endpoint", endpoint).
			Str("shareId", share.GetId().GetOpaqueId()).
			Str("mode", mode).
			Msg("access to remote OCM share succeeded")
	}
	d.storeClient(shareID, &cachedClient{
		client:     c,
		share:      share,
		authHeader: authHdr,
		mode:       mode,
	}, time.Hour)
	return c, share, rel, nil
}

// openAttempt resolves a WebDAV client. dropCache forces a new client, which
// re-exchanges when the share is not in the failure memory. rejectedMode
// modeExchange remembers the failure first so the new client is legacy.
func (d *driver) openAttempt(
	ctx context.Context,
	ref *provider.Reference,
	dropCache bool,
	rejectedMode string,
) (*gowebdav.Client, *ocmpb.ReceivedShare, string, string, error) {
	id, _ := shareInfoFromReference(ref)
	shareID := id.GetOpaqueId()
	if rejectedMode == modeExchange {
		d.rememberExchangeFailure(shareID)
		appctx.GetLogger(ctx).Info().
			Str("shareId", shareID).
			Msg("OCM access token was rejected, trying shared secret")
	}
	if dropCache || rejectedMode == modeExchange {
		_ = d.ccache.Remove(shareID)
	}
	c, share, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, nil, "", "", err
	}
	return c, share, rel, d.sessionMode(shareID, share.GetProtocols()), nil
}

// retryUnauthorized runs fn and, on HTTP 401, drops the cached client and
// tries once more. An exchanged token that is still rejected falls back to
// the legacy shared secret for one further attempt. A 401 from that legacy
// attempt stays invalid credentials. Any other error, including a policy
// violation, is returned as-is.
func retryUnauthorized[T any](
	d *driver,
	ctx context.Context,
	ref *provider.Reference,
	denied string,
	fn func(*gowebdav.Client, string) (T, error),
) (T, *ocmpb.ReceivedShare, string, error) {
	var zero T
	c, share, rel, _, err := d.openAttempt(ctx, ref, false, "")
	if err != nil {
		return zero, nil, "", err
	}
	got, err := fn(c, rel)
	if err == nil {
		return got, share, rel, nil
	}
	if !isWebDAV401(err) {
		return zero, nil, "", err
	}

	c, share, rel, mode, err := d.openAttempt(ctx, ref, true, "")
	if err != nil {
		return zero, nil, "", err
	}
	got, err = fn(c, rel)
	if err == nil {
		return got, share, rel, nil
	}
	if !isWebDAV401(err) {
		return zero, nil, "", err
	}
	if mode != modeExchange {
		return zero, nil, "", errtypes.InvalidCredentials(denied)
	}

	c, share, rel, _, err = d.openAttempt(ctx, ref, true, modeExchange)
	if err != nil {
		return zero, nil, "", err
	}
	got, err = fn(c, rel)
	if err != nil {
		if isWebDAV401(err) {
			return zero, nil, "", errtypes.InvalidCredentials(denied)
		}
		return zero, nil, "", err
	}
	return got, share, rel, nil
}
