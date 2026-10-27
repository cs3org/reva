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

// Package httpsig implements the OCM profile of RFC 9421 HTTP Message Signatures.
package httpsig

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/yaronf/httpsign"
)

// tag marks a signature as the OCM signature of a request (RFC 9421, Section 2.3).
const tag = "ocm"

// A captured signature stays valid for maxAge. maxSkew allows for a signer with a fast clock.
const (
	maxAge  = 5 * time.Minute
	maxSkew = time.Minute
)

// components are the parts of a request that the OCM spec requires a signature to cover.
var components = httpsign.Headers("@method", "@target-uri", "content-digest", "content-length")

// httpSigAlgs maps JWS algorithms to their names in the
// HTTP Signature Algorithms registry (RFC 9421, Section 6.2).
var httpSigAlgs = map[string]string{
	"Ed25519": "ed25519",
	"EdDSA":   "ed25519",
	"ES256":   "ecdsa-p256-sha256",
	"ES384":   "ecdsa-p384-sha384",
	"PS512":   "rsa-pss-sha512",
	"RS256":   "rsa-v1_5-sha256",
}

// ErrUnsigned is returned by Verify for a request without an OCM signature.
var ErrUnsigned = errors.New("the request has no OCM signature")

// Key is the private key that this server uses to sign OCM requests.
type Key struct {
	// ID is the keyid of the signatures and the kid of the published key.
	ID      string
	private ed25519.PrivateKey
	public  jwk.Key
}

// LoadKey reads an Ed25519 private key from a PKCS#8 PEM file,
// such as the output of `openssl genpkey -algorithm ed25519`.
func LoadKey(path string) (*Key, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM data", path)
	}
	raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	private, ok := raw.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: the key is %T, but an Ed25519 key is required", path, raw)
	}

	public, err := jwk.Import[jwk.Key](private.Public())
	if err != nil {
		return nil, err
	}
	// The RFC 7638 thumbprint changes with the key, thus a new key gets a new kid without a config change.
	thumbprint, err := public.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(thumbprint)
	// The OCM spec requires kid and alg on each published key.
	if err := public.Set(jwk.KeyIDKey, id); err != nil {
		return nil, err
	}
	if err := public.Set(jwk.AlgorithmKey, jwa.EdDSAEd25519()); err != nil {
		return nil, err
	}
	return &Key{ID: id, private: private, public: public}, nil
}

// JWKS returns the JWK Set that publishes the public part of k.
func (k *Key) JWKS() ([]byte, error) {
	set := jwk.NewSet()
	if err := set.AddKey(k.public); err != nil {
		return nil, err
	}
	return json.Marshal(set)
}

// Sign adds an OCM signature to r. body must be the bytes that r sends.
func (k *Key) Sign(r *http.Request, body []byte) error {
	// The transport sends the length from r.ContentLength. The header is set only to be signed.
	r.Header.Set("Content-Length", strconv.Itoa(len(body)))
	rc := io.NopCloser(bytes.NewReader(body))
	digest, err := httpsign.GenerateContentDigestHeader(&rc, []string{httpsign.DigestSha256})
	if err != nil {
		return err
	}
	r.Header.Set("Content-Digest", digest)

	signer, err := httpsign.NewEd25519Signer(k.private, httpsign.NewSignConfig().SetKeyID(k.ID).SetTag(tag), components)
	if err != nil {
		return err
	}
	input, signature, err := httpsign.SignRequest(tag, *signer, r)
	if err != nil {
		return err
	}
	r.Header.Set("Signature-Input", input)
	r.Header.Set("Signature", signature)
	return nil
}

// Verify checks the OCM signature of r. body is the request body, which r no longer holds.
// jwks returns the JWK Set of the signer. Verify calls it only for a signed request,
// thus an unsigned request does not cause a fetch.
func Verify(r *http.Request, body []byte, jwks func() ([]byte, error)) error {
	sig, err := httpsign.RequestDetailsByTag(r, tag)
	if errors.Is(err, httpsign.ErrSignatureTagNotFound) {
		return ErrUnsigned
	}
	if err != nil {
		return err
	}
	if sig.KeyID == nil {
		return errors.New("the OCM signature has no keyid")
	}

	data, err := jwks()
	if err != nil {
		return err
	}
	set, err := jwk.Parse(data)
	if err != nil {
		return err
	}
	key, ok := set.LookupKeyID(*sig.KeyID)
	if !ok {
		return fmt.Errorf("the signer publishes no key with kid %q", *sig.KeyID)
	}
	alg, ok := key.Algorithm()
	if !ok {
		return fmt.Errorf("the key %q has no alg", *sig.KeyID)
	}
	// The key gives the algorithm. An alg parameter that names a different one
	// is an algorithm confusion (RFC 9421, Section 7.3.6).
	if sig.Alg != "" && sig.Alg != httpSigAlgs[alg.String()] {
		return fmt.Errorf("the alg parameter %q does not agree with the key algorithm %s", sig.Alg, alg)
	}
	// The OCM spec forbids none and the symmetric algorithms.
	allowed, err := httpsign.NewJWSAlgAllowlist(
		jwa.EdDSAEd25519(), jwa.EdDSA(),
		jwa.ES256(), jwa.ES384(), jwa.ES512(),
		jwa.PS256(), jwa.PS384(), jwa.PS512(),
		jwa.RS256(), jwa.RS384(), jwa.RS512(),
	)
	if err != nil {
		return err
	}
	config := httpsign.NewVerifyConfig().SetNotOlderThan(maxAge).SetNotNewerThan(maxSkew)
	verifier, err := httpsign.NewJWSVerifier(allowed, key, config, components)
	if err != nil {
		return err
	}

	rc := io.NopCloser(bytes.NewReader(body))
	if err := httpsign.ValidateContentDigestHeader(r.Header.Values("Content-Digest"), &rc, []string{httpsign.DigestSha256, httpsign.DigestSha512}); err != nil {
		return err
	}

	// rhttp removes the service prefix from r.URL, but the signer signed the full URI.
	target, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		return err
	}
	if target.Host == "" {
		target.Host = r.Host
		// Behind a TLS proxy r.TLS is nil. A false X-Forwarded-Proto changes only
		// the scheme, thus it cannot make a signature for a different URI valid.
		target.Scheme = r.Header.Get("X-Forwarded-Proto")
		if target.Scheme == "" {
			target.Scheme = "http"
			if r.TLS != nil {
				target.Scheme = "https"
			}
		}
	}
	req := r.Clone(r.Context())
	req.URL = target
	return httpsign.VerifyRequest(sig.Label, *verifier, req)
}
