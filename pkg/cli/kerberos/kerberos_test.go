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

package kerberos

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/iana/etypeID"
	"github.com/jcmturner/gokrb5/v8/iana/flags"
	"github.com/jcmturner/gokrb5/v8/iana/nametype"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/messages"
	"github.com/jcmturner/gokrb5/v8/service"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/jcmturner/gokrb5/v8/types"
)

const (
	realm = "TEST.REVA"
	spn   = "HTTP/reva.test"
)

func TestCCachePath(t *testing.T) {
	uid := strconv.Itoa(os.Getuid())
	cases := []struct {
		name, flag, env, want, wantErr string
	}{
		{name: "default", want: "/tmp/krb5cc_" + uid},
		{name: "env file", env: "FILE:/run/user/cc", want: "/run/user/cc"},
		{name: "env plain path", env: "/var/tmp/cc", want: "/var/tmp/cc"},
		{name: "flag before env", flag: "/home/a/cc", env: "FILE:/run/user/cc", want: "/home/a/cc"},
		{name: "flag file", flag: "FILE:/home/a/cc", want: "/home/a/cc"},
		{name: "keyring", env: "KEYRING:persistent:1000", wantErr: "KEYRING: cache"},
		{name: "kcm", env: "KCM:", wantErr: "KCM: cache"},
		{name: "dir", flag: "DIR:/run/user/1000/krb5cc", wantErr: "--ccache=DIR:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("KRB5CCNAME", c.env)
			got, err := (&Provider{CCache: c.flag}).CCachePath()
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) || !strings.Contains(err.Error(), "kinit") {
					t.Fatalf("got %q, %v; want an error with %q and kinit", got, err, c.wantErr)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestNegotiate(t *testing.T) {
	kt := keytab.New()
	for _, p := range []string{spn, "krbtgt/" + realm} {
		if err := kt.AddEntry(p, realm, "secret of "+p, time.Now(), 1, etypeID.AES256_CTS_HMAC_SHA1_96); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	dir := t.TempDir()
	cc := writeCCache(t, dir, kt, now.Add(10*time.Hour))
	p := &Provider{CCache: "FILE:" + cc, Config: writeKrb5Conf(t, dir)}

	ticket, err := p.Ticket()
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Principal != "alice@"+realm || ticket.Username() != "alice" || ticket.CachePath != cc {
		t.Fatalf("wrong ticket: %+v", ticket)
	}

	tok, err := p.Negotiate(spn)
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		t.Fatal(err)
	}

	// The same checks as the kerberos auth manager of reva.
	var st spnego.SPNEGOToken
	if err := st.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	ok, ctx, status := spnego.SPNEGOService(kt, service.DecodePAC(false), service.KeytabPrincipal(spn)).AcceptSecContext(&st)
	if !ok {
		t.Fatalf("token refused: %v", status)
	}
	id, _ := ctx.Value("github.com/jcmturner/gokrb5/v8/ctxCredentials").(*credentials.Credentials)
	if id == nil || id.UserName() != "alice" || id.Domain() != realm {
		t.Fatalf("wrong identity: %+v", id)
	}

	var mech spnego.KRB5Token
	if err := mech.Unmarshal(st.NegTokenInit.MechTokenBytes); err != nil {
		t.Fatal(err)
	}
	if types.IsFlagSet(&mech.APReq.APOptions, flags.APOptionMutualRequired) {
		t.Error("the AP-REQ asks for mutual authentication")
	}
	if err := mech.APReq.Ticket.DecryptEncPart(kt, nil); err != nil {
		t.Fatal(err)
	}
	if err := mech.APReq.DecryptAuthenticator(mech.APReq.Ticket.DecryptedEncPart.Key); err != nil {
		t.Fatal(err)
	}
	// RFC 4121 4.1.1: the context flags are bytes 20 to 23 of the checksum.
	if f := binary.LittleEndian.Uint32(mech.APReq.Authenticator.Cksum.Checksum[20:24]); f&uint32(gssapi.ContextFlagMutual) != 0 {
		t.Errorf("the token sets GSS_C_MUTUAL_FLAG (flags %#x)", f)
	}
}

func TestTicketRefused(t *testing.T) {
	kt := keytab.New()
	if err := kt.AddEntry("krbtgt/"+realm, realm, "x", time.Now(), 1, etypeID.AES256_CTS_HMAC_SHA1_96); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	expired := writeCCache(t, dir, kt, time.Now().Add(-time.Minute))
	damaged := filepath.Join(dir, "damaged")
	if err := os.WriteFile(damaged, []byte{5, 4, 0, 0, 0, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ path, want string }{
		"expired": {expired, "expired at"},
		"missing": {filepath.Join(dir, "none"), "no Kerberos ticket at"},
		"damaged": {damaged, "damaged"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&Provider{CCache: c.path}).Ticket()
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "kinit") {
				t.Fatalf("got %v; want an error with %q and kinit", err, c.want)
			}
		})
	}
}

func writeKrb5Conf(t *testing.T, dir string) string {
	t.Helper()
	// No KDC answers here: the test fails if the client asks one.
	conf := "[libdefaults]\n default_realm = " + realm + "\n\n[realms]\n " + realm + " = {\n  kdc = 127.0.0.1:1\n }\n"
	p := filepath.Join(dir, "krb5.conf")
	if err := os.WriteFile(p, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeCCache writes the credential cache that kinit, then a request for spn,
// leave: a TGT for alice, and a service ticket for spn when kt has its key.
func writeCCache(t *testing.T, dir string, kt *keytab.Keytab, end time.Time) string {
	t.Helper()
	alice := types.NewPrincipalName(nametype.KRB_NT_PRINCIPAL, "alice")
	start := end.Add(-12 * time.Hour)

	var b bytes.Buffer
	w := func(v any) {
		if err := binary.Write(&b, binary.BigEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	data := func(d []byte) { w(uint32(len(d))); b.Write(d) }
	principal := func(pn types.PrincipalName) {
		w(uint32(pn.NameType))
		w(uint32(len(pn.NameString)))
		data([]byte(realm))
		for _, s := range pn.NameString {
			data([]byte(s))
		}
	}

	// Version 4 of the file format, with no header fields.
	w(uint16(0x0504))
	w(uint16(0))
	principal(alice)
	for _, s := range []string{"krbtgt/" + realm, spn} {
		sname := types.NewPrincipalName(nametype.KRB_NT_SRV_INST, s)
		if _, _, err := kt.GetEncryptionKey(sname, realm, 0, etypeID.AES256_CTS_HMAC_SHA1_96); err != nil {
			continue
		}
		tkt, key, err := messages.NewTicket(alice, realm, sname, realm, types.NewKrbFlags(), kt,
			etypeID.AES256_CTS_HMAC_SHA1_96, 1, start, start, end, end)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := tkt.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		principal(alice)
		principal(sname)
		w(uint16(key.KeyType))
		data(key.KeyValue)
		for _, ts := range []time.Time{start, start, end, end} {
			w(uint32(ts.Unix()))
		}
		w(uint8(0))  // not a session-key ticket
		w(uint32(0)) // ticket flags
		w(uint32(0)) // addresses
		w(uint32(0)) // authorization data
		data(raw)
		data(nil) // second ticket
	}

	p := filepath.Join(dir, "krb5cc")
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
