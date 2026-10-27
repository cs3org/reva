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

package httpsig

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func writePKCS8(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The key and its thumbprint are from RFC 8037, Appendix A.
func TestLoadKeyRFC8037(t *testing.T) {
	seed, _ := base64.RawURLEncoding.DecodeString("nWGxne_9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A")
	key, err := LoadKey(writePKCS8(t, ed25519.NewKeyFromSeed(seed)))
	if err != nil {
		t.Fatal(err)
	}
	if want := "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k"; key.ID != want {
		t.Errorf("ID: got %s, want %s", key.ID, want)
	}

	data, err := key.JWKS()
	if err != nil {
		t.Fatal(err)
	}
	var set struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	if len(set.Keys) != 1 {
		t.Fatalf("got %d keys, want 1: %s", len(set.Keys), data)
	}
	want := map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo",
		"kid": key.ID,
		"alg": "Ed25519",
	}
	for k, v := range want {
		if set.Keys[0][k] != v {
			t.Errorf("%s: got %q, want %q", k, set.Keys[0][k], v)
		}
	}
	if _, ok := set.Keys[0]["d"]; ok {
		t.Error("the JWK Set must not contain the private key")
	}
}

func TestLoadKeyRejectsOtherKeyTypes(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(writePKCS8(t, ec)); err == nil {
		t.Error("expected an error for a P-256 key")
	}
}

// The body, the headers and the JWK Set are from Appendix B of the OCM spec.
const appendixBBody = `{
  "shareWith": "marie@receiver.example.org",
  "name": "spec.yaml",
  "providerId": "7c084226-d9a1-11e6-bf26-cec0c932ce01",
  "owner": "einstein@sender.example.org",
  "sender": "einstein@sender.example.org",
  "ownerDisplayName": "Albert Einstein",
  "senderDisplayName": "Albert Einstein",
  "shareType": "user",
  "resourceType": "file",
  "protocol": {
    "name": "multi",
    "webdav": {
      "uri": "spec.yaml",
      "sharedSecret": "hfiuhworzwnur98d3wjiwhr",
      "permissions": ["read", "write"]
    }
  }
}`

const appendixBJWKS = `{"keys": [{"kty": "OKP", "crv": "Ed25519", "alg": "Ed25519", "kid": "sender.example.org#key1", "x": "AzeStPAdQ-vbi6bJVxcQjBPF2I7fDZdfDZ_XZ3J4Azs"}]}`

func TestVerifyAppendixB(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Until(time.Unix(1785060500, 0)))

		// The request as reva gets it behind a TLS proxy: rhttp removed the
		// service prefix from the path.
		r := httptest.NewRequest(http.MethodPost, "/ocm/shares", strings.NewReader(appendixBBody))
		r.URL.Path = "/shares"
		r.Host = "receiver.example.org"
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Content-Digest", "sha-256=:/Uz47cfD1HOdMTIqcWjd84iMLQ4gVJdC7ZFACf1ViqU=:")
		r.Header.Set("Content-Length", "521")
		r.Header.Set("Signature-Input", `sig1=("@method" "@target-uri" "content-digest" "content-length");created=1785060500;keyid="sender.example.org#key1";alg="ed25519";tag="ocm"`)
		r.Header.Set("Signature", "sig1=:Epax4XFqvbrrDzQUAaNl+uECasEKbArgz8AlEkXspvYX2fSkQFlCkaXImZnYU898C2t0IdmbFhjR8bkN0hZgCg==:")
		jwks := func() ([]byte, error) { return []byte(appendixBJWKS), nil }

		if err := Verify(r, []byte(appendixBBody), jwks); err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		time.Sleep(maxAge + time.Second)
		if err := Verify(r, []byte(appendixBBody), jwks); err == nil {
			t.Error("expected an error for a signature older than maxAge")
		}
	})
}

func TestVerify(t *testing.T) {
	newKey := func() *Key {
		_, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		key, err := LoadKey(writePKCS8(t, private))
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	key, other := newKey(), newKey()
	jwks := func() ([]byte, error) { return key.JWKS() }
	body := []byte(`{"shareWith":"marie@cernbox.example.org"}`)
	signed := func(t *testing.T) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "https://cernbox.example.org/ocm/shares", bytes.NewReader(body))
		if err := key.Sign(r, body); err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("valid signature", func(t *testing.T) {
		if err := Verify(signed(t), body, jwks); err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
	})
	t.Run("no signature", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "https://cernbox.example.org/ocm/shares", bytes.NewReader(body))
		if err := Verify(r, body, jwks); !errors.Is(err, ErrUnsigned) {
			t.Fatalf("Verify() error = %v, want ErrUnsigned", err)
		}
	})
	t.Run("changed body", func(t *testing.T) {
		if err := Verify(signed(t), []byte(`{"shareWith":"eve@cernbox.example.org"}`), jwks); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("other endpoint", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "https://cernbox.example.org/ocm/notifications", bytes.NewReader(body))
		r.Header = signed(t).Header
		if err := Verify(r, body, jwks); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("unknown kid", func(t *testing.T) {
		if err := Verify(signed(t), body, func() ([]byte, error) { return other.JWKS() }); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("two OCM signatures", func(t *testing.T) {
		r := signed(t)
		for _, h := range []string{"Signature-Input", "Signature"} {
			v := r.Header.Get(h)
			r.Header.Set(h, v+", "+strings.Replace(v, tag+"=", "second=", 1))
		}
		if err := Verify(r, body, jwks); err == nil || errors.Is(err, ErrUnsigned) {
			t.Fatalf("Verify() error = %v, want a rejection", err)
		}
	})
}
