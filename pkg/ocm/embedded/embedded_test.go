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

package embedded

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/rs/zerolog"
	"github.com/studio-b12/gowebdav"
)

func testEmbeddedSec() ocmd.UntrustedClientSecurity {
	var s ocmd.UntrustedClientSecurity
	s.ApplyDefaults()
	if err := s.Compile(); err != nil {
		panic(err)
	}
	return s
}

func testLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

func parseGraph(t *testing.T, payload string) []crateEntity {
	t.Helper()
	var c crate
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	return c.Graph
}

func TestConfigApplyDefaults(t *testing.T) {
	t.Run("empty config gets defaults", func(t *testing.T) {
		c := Config{}
		c.ApplyDefaults()
		if c.Timeout != 3600 {
			t.Errorf("Timeout = %d, want 3600", c.Timeout)
		}
		if c.IdleTimeout != 120 {
			t.Errorf("IdleTimeout = %d, want 120", c.IdleTimeout)
		}
		if c.Retries != 3 {
			t.Errorf("Retries = %d, want 3", c.Retries)
		}
	})

	t.Run("explicit values preserved", func(t *testing.T) {
		c := Config{WebDAVURL: "https://dav.example.org/", Timeout: 10, IdleTimeout: 5, Retries: 1}
		c.ApplyDefaults()
		if c.Timeout != 10 || c.IdleTimeout != 5 || c.Retries != 1 {
			t.Errorf("explicit values were overwritten: %+v", c)
		}
		if c.WebDAVURL != "https://dav.example.org/" {
			t.Errorf("WebDAVURL = %q, want preserved", c.WebDAVURL)
		}
	})
}

func TestNewAppliesDefaults(t *testing.T) {
	tr, err := New(context.Background(), map[string]any{"webdav_url": "https://dav.example.org/"})
	if err != nil {
		t.Fatalf("New: unexpected error %v", err)
	}
	d := tr.(*driver)
	if d.c.Timeout != 3600 || d.c.IdleTimeout != 120 || d.c.Retries != 3 {
		t.Errorf("New did not apply defaults: %+v", d.c)
	}
}

func TestNewRejectsHatch(t *testing.T) {
	t.Parallel()

	t.Run("allow_http", func(t *testing.T) {
		_, err := New(context.Background(), map[string]any{
			"webdav_url": "https://dav.example.org/",
			"ocm_client_security": map[string]any{
				"allow_http": true,
			},
		})
		if err == nil {
			t.Fatal("expected embedded.New to reject allow_http")
		}
		if !errors.Is(err, ocmd.ErrHatchAllowHTTP) {
			t.Fatalf("got %v, want ocmd.ErrHatchAllowHTTP", err)
		}
	})

	t.Run("allowed_cidrs", func(t *testing.T) {
		_, err := New(context.Background(), map[string]any{
			"webdav_url": "https://dav.example.org/",
			"ocm_client_security": map[string]any{
				"allowed_cidrs": []string{"10.0.0.0/8"},
			},
		})
		if err == nil {
			t.Fatal("expected embedded.New to reject allowed_cidrs")
		}
		if !errors.Is(err, ocmd.ErrHatchAllowedCIDRs) {
			t.Fatalf("got %v, want ocmd.ErrHatchAllowedCIDRs", err)
		}
	})
}

func innerUntrustedTransport(t *testing.T, rt http.RoundTripper) *http.Transport {
	t.Helper()
	if tr, ok := rt.(*http.Transport); ok {
		return tr
	}
	v := reflect.ValueOf(rt)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		f := v.FieldByName("Transport")
		if f.IsValid() && f.CanInterface() {
			if tr, ok := f.Interface().(*http.Transport); ok && tr != nil {
				return tr
			}
		}
	}
	t.Fatalf("untrusted transport: got %T, want an *http.Transport wrapper", rt)
	return nil
}

func TestNewTLSMinVersion(t *testing.T) {
	t.Parallel()

	t.Run("configured 1.3 reaches the transport", func(t *testing.T) {
		t.Parallel()
		tr, err := New(context.Background(), map[string]any{
			"webdav_url":                   "https://dav.example.org/",
			"embedded_src_tls_min_version": "1.3",
		})
		if err != nil {
			t.Fatal(err)
		}
		d := tr.(*driver)
		if d.tlsMinVersion != tls.VersionTLS13 {
			t.Fatalf("tlsMinVersion = %#x, want TLS 1.3", d.tlsMinVersion)
		}
		client := newEmbeddedSrcClient(
			5*time.Second,
			false,
			d.sec,
			d.tlsMinVersion,
		)
		got := innerUntrustedTransport(t, client.Transport)
		if got.TLSClientConfig == nil || got.TLSClientConfig.MinVersion != tls.VersionTLS13 {
			t.Fatalf("src client MinVersion = %v, want TLS 1.3", got.TLSClientConfig)
		}
	})

	t.Run("invalid value fails service start", func(t *testing.T) {
		t.Parallel()
		_, err := New(context.Background(), map[string]any{
			"webdav_url":                   "https://dav.example.org/",
			"embedded_src_tls_min_version": "1.0",
		})
		if err == nil {
			t.Fatal("expected embedded.New to reject TLS min version 1.0")
		}
		if !errors.Is(err, ocmd.ErrInvalidTLSMinVersion) {
			t.Fatalf("got %v, want ocmd.ErrInvalidTLSMinVersion", err)
		}
	})
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty", "", 0},
		{"seconds", "120", 120 * time.Second},
		{"zero seconds", "0", 0},
		{"negative seconds", "-5", 0},
		{"garbage", "abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.in); got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}

	t.Run("future http date", func(t *testing.T) {
		h := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
		got := parseRetryAfter(h)
		if got <= 0 || got > time.Hour {
			t.Errorf("parseRetryAfter(%q) = %v, want in (0, 1h]", h, got)
		}
	})

	t.Run("past http date", func(t *testing.T) {
		h := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
		if got := parseRetryAfter(h); got != 0 {
			t.Errorf("parseRetryAfter(past) = %v, want 0", got)
		}
	})
}

func TestZenodoFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://zenodo.org/records/1/files/data.csv/content", "data.csv"},
		{"https://example.org/path/file.bin", "file.bin"},
		{"https://example.org/file.txt/content", "file.txt"},
		{"plainname", "plainname"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := zenodoFilename(tt.in); got != tt.want {
				t.Errorf("zenodoFilename(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCrateEntityURLString(t *testing.T) {
	tests := []struct {
		name string
		obj  string
		want string
	}{
		{"string url", `{"url":"https://x/y.txt"}`, "https://x/y.txt"},
		{"id object url", `{"url":{"@id":"https://x/z.bin"}}`, "https://x/z.bin"},
		{"absent url", `{"name":"foo"}`, ""},
		{"numeric url", `{"url":123}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e crateEntity
			if err := json.Unmarshal([]byte(tt.obj), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.URLString(); got != tt.want {
				t.Errorf("URLString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCrateEntityHasType(t *testing.T) {
	tests := []struct {
		name    string
		obj     string
		want    string
		hasType bool
	}{
		{"single match", `{"@type":"File"}`, "File", true},
		{"single no match", `{"@type":"Dataset"}`, "File", false},
		{"array match", `{"@type":["Dataset","File"]}`, "File", true},
		{"array no match", `{"@type":["Dataset","Image"]}`, "File", false},
		{"absent", `{"name":"x"}`, "File", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e crateEntity
			if err := json.Unmarshal([]byte(tt.obj), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.HasType(tt.want); got != tt.hasType {
				t.Errorf("HasType(%q) = %v, want %v", tt.want, got, tt.hasType)
			}
		})
	}
}

func TestCrateEntityIsTransferable(t *testing.T) {
	tests := []struct {
		name string
		obj  string
		want bool
	}{
		{"file with url", `{"@type":"File","url":"https://x/y"}`, true},
		{"workflow with url", `{"@type":"ComputationalWorkflow","url":"https://x/y"}`, true},
		{"file without url", `{"@type":"File"}`, false},
		{"dataset with url", `{"@type":"Dataset","url":"https://x/y"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e crateEntity
			if err := json.Unmarshal([]byte(tt.obj), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.IsTransferable(); got != tt.want {
				t.Errorf("IsTransferable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCrateEntries(t *testing.T) {
	log := testLogger()

	tests := []struct {
		name    string
		payload string
		want    []transferEntry
	}{
		{
			name: "plain file with explicit name and size",
			payload: `{"@graph":[
				{"@id":"f1","@type":"File","url":"https://src.example.org/f1.txt","name":"f1.txt","contentSize":"1234","encodingFormat":"text/plain"}
			]}`,
			want: []transferEntry{
				{srcURL: "https://src.example.org/f1.txt", name: "f1.txt", sizeHint: 1234, encodingFormat: "text/plain"},
			},
		},
		{
			name: "plain id-object url, name derived from path, unknown size",
			payload: `{"@graph":[
				{"@id":"f2","@type":["File","Thing"],"url":{"@id":"https://src.example.org/path/f2.bin"}}
			]}`,
			want: []transferEntry{
				{srcURL: "https://src.example.org/path/f2.bin", name: "f2.bin", sizeHint: -1, encodingFormat: ""},
			},
		},
		{
			name: "zenodo dataset distributions",
			payload: `{"@graph":[
				{"@id":"ds","@type":"Dataset","distribution":[
					{"@type":"DataDownload","contentUrl":"https://zenodo.org/records/1/files/data.csv/content","encodingFormat":"text/csv"},
					{"@type":"DataDownload","contentUrl":"https://zenodo.org/records/1/files/img.png/content"}
				]}
			]}`,
			want: []transferEntry{
				{srcURL: "https://zenodo.org/records/1/files/data.csv/content", name: "data.csv", sizeHint: -1, encodingFormat: "text/csv"},
				{srcURL: "https://zenodo.org/records/1/files/img.png/content", name: "img.png", sizeHint: -1, encodingFormat: ""},
			},
		},
		{
			name: "skips non-DataDownload and empty contentUrl distributions",
			payload: `{"@graph":[
				{"@id":"ds","@type":"Dataset","distribution":[
					{"@type":"DataDownload","contentUrl":""},
					{"@type":"WebPage","contentUrl":"https://zenodo.org/records/1"}
				]}
			]}`,
			want: nil,
		},
		{
			name: "skips non-transferable entity without url",
			payload: `{"@graph":[
				{"@id":"d","@type":"Dataset","name":"no url here"}
			]}`,
			want: nil,
		},
		{
			name: "mixed plain and zenodo, order preserved",
			payload: `{"@graph":[
				{"@id":"f1","@type":"File","url":"https://src.example.org/a.txt"},
				{"@id":"ds","@type":"Dataset","distribution":[
					{"@type":"DataDownload","contentUrl":"https://zenodo.org/records/1/files/b.csv/content"}
				]}
			]}`,
			want: []transferEntry{
				{srcURL: "https://src.example.org/a.txt", name: "a.txt", sizeHint: -1, encodingFormat: ""},
				{srcURL: "https://zenodo.org/records/1/files/b.csv/content", name: "b.csv", sizeHint: -1, encodingFormat: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crateEntries(log, parseGraph(t, tt.payload))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("crateEntries() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestProgressReader(t *testing.T) {
	const data = "hello world"
	before := time.Now().UnixNano()
	pr := newProgressReader(strings.NewReader(data))

	buf := make([]byte, 4)
	var total int64
	for {
		n, err := pr.Read(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}

	if total != int64(len(data)) {
		t.Fatalf("read %d bytes, want %d", total, len(data))
	}
	if got := pr.total.Load(); got != int64(len(data)) {
		t.Errorf("pr.total = %d, want %d", got, len(data))
	}
	if pr.lastData.Load() < before {
		t.Errorf("lastData was not updated")
	}
}

func newTestTransferrer(t *testing.T) Transferrer {
	t.Helper()
	tr, err := New(context.Background(), map[string]any{"webdav_url": "https://dav.example.org/"})
	if err != nil {
		t.Fatalf("New: unexpected error %v", err)
	}
	return tr
}

func TestProcessMalformedPayload(t *testing.T) {
	tr := newTestTransferrer(t)
	called := false
	if err := tr.Process(context.Background(), "{not valid json", "/dest", func(error) { called = true }); err == nil {
		t.Error("Process with malformed payload: expected error, got nil")
	}
	if called {
		t.Error("Process with malformed payload: onComplete must not be called")
	}
}

func TestProcessNoTransferableEntries(t *testing.T) {
	tr := newTestTransferrer(t)
	// A valid payload with nothing transferable must be a no-op: it returns nil
	// and does not require a token (no background transfer is started). The
	// onComplete callback is still invoked synchronously with a nil error.
	payload := `{"@graph":[{"@id":"d","@type":"Dataset","name":"nothing to transfer"}]}`
	var gotErr error
	called := false
	if err := tr.Process(context.Background(), payload, "/dest", func(err error) {
		called = true
		gotErr = err
	}); err != nil {
		t.Errorf("Process with no transferable entries: unexpected error %v", err)
	}
	if !called {
		t.Error("Process with no transferable entries: onComplete was not called")
	}
	if gotErr != nil {
		t.Errorf("Process with no transferable entries: onComplete error = %v, want nil", gotErr)
	}
}

func firstPublicIPv4() (string, bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP == nil {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			continue
		}
		if ip[0] == 100 && ip[1]&0xc0 == 64 {
			continue
		}
		return ip.String(), true
	}
	return "", false
}

func startPublicTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ip, ok := firstPublicIPv4()
	if !ok {
		t.Skip("no public IPv4 address available for public-only httptest")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Skipf("cannot listen on public IP %s: %v", ip, err)
	}
	srv := httptest.NewUnstartedServer(h)
	if srv.Listener != nil {
		_ = srv.Listener.Close()
	}
	srv.Listener = ln
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func startDestWebDAV(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, "MKCOL":
			if r.Body != nil {
				_, _ = io.Copy(io.Discard, r.Body)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dummyDAV() *gowebdav.Client {
	return gowebdav.NewClient("https://127.0.0.1:1", "", "")
}

type hostDialTrace struct {
	mu          sync.Mutex
	established []string
}

func contextWithHostDialTrace(ctx context.Context) (context.Context, *hostDialTrace) {
	tr := &hostDialTrace{established: []string{}}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		ConnectDone: func(_, addr string, err error) {
			if err != nil {
				return
			}
			tr.mu.Lock()
			tr.established = append(tr.established, addr)
			tr.mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Conn == nil {
				return
			}
			tr.mu.Lock()
			tr.established = append(tr.established, info.Conn.RemoteAddr().String())
			tr.mu.Unlock()
		},
	})
	return ctx, tr
}

func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
		return ip.String()
	}
	return host
}

func (tr *hostDialTrace) establishedHost(host string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for _, addr := range tr.established {
		if hostOf(addr) == host {
			return true
		}
	}
	return false
}

func assertNoEstablishedHost(t *testing.T, tr *hostDialTrace, host string) {
	t.Helper()
	if tr.establishedHost(host) {
		t.Fatalf("embedded srcURL fetch established a connection to %s", host)
	}
}

func TestEmbeddedSrcURLFetchPublicHTTPSHostSucceeds(t *testing.T) {
	const body = "embedded-src"
	contacted := false
	src := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	dest := startDestWebDAV(t)
	dav := gowebdav.NewClient(dest.URL, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := uploadURLToWebDAV(
		ctx,
		testLogger(),
		newEmbeddedSrcClient(5*time.Second, true, testEmbeddedSec(), 0),
		dav,
		src.URL+"/file.txt",
		"/dest/file.txt",
		int64(len(body)),
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("embedded srcURL fetch to a public HTTPS host: %v", err)
	}
	if !contacted {
		t.Fatal("expected embedded srcURL fetch to reach the public HTTPS host")
	}
}

func TestEmbeddedSrcURLFetchRefusesPrivateHost(t *testing.T) {
	t.Run("metadata host is refused at dial", func(t *testing.T) {
		ctx, tr := contextWithHostDialTrace(context.Background())
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := uploadURLToWebDAV(
			ctx,
			testLogger(),
			newEmbeddedSrcClient(5*time.Second, true, testEmbeddedSec(), 0),
			dummyDAV(),
			"https://169.254.169.254/file.txt",
			"/dest/file.txt",
			-1,
			5*time.Second,
		)
		assertNoEstablishedHost(t, tr, "169.254.169.254")
		if err == nil {
			t.Fatal("expected embedded srcURL fetch to refuse the metadata host at dial")
		}
		if !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("got %v, want ocmd.ErrNonPublicAddr", err)
		}
	})

	t.Run("reachable private host is not contacted", func(t *testing.T) {
		contacted := false
		src := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contacted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("secret"))
		}))
		defer src.Close()

		// insecure=true so TLS cannot short-circuit before the untrusted dial
		// guard. The old default client would then reach this loopback server.
		ctx, tr := contextWithHostDialTrace(context.Background())
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := uploadURLToWebDAV(
			ctx,
			testLogger(),
			newEmbeddedSrcClient(5*time.Second, true, testEmbeddedSec(), 0),
			dummyDAV(),
			src.URL+"/file.txt",
			"/dest/file.txt",
			-1,
			5*time.Second,
		)
		if contacted {
			t.Fatal("embedded srcURL fetch contacted a private host; the untrusted transport must refuse it at dial")
		}
		assertNoEstablishedHost(t, tr, hostOf(src.Listener.Addr().String()))
		if err == nil {
			t.Fatal("expected embedded srcURL fetch to refuse the private host at dial")
		}
		if !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("got %v, want ocmd.ErrNonPublicAddr", err)
		}
	})
}

func TestEmbeddedSrcURLFetchRefusesHTTP(t *testing.T) {
	contacted := false
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("secret"))
	}))
	defer src.Close()

	dest := startDestWebDAV(t)
	d := &driver{c: Config{
		WebDAVURL:   dest.URL,
		Timeout:     5,
		IdleTimeout: 2,
		Retries:     1,
	}}
	err := d.transferEntries(
		testLogger(),
		"tok",
		"/dest",
		[]transferEntry{{srcURL: src.URL + "/file.txt", name: "file.txt", sizeHint: -1}},
		5*time.Second,
	)
	if contacted {
		t.Fatal("embedded srcURL fetch contacted an http host; the untrusted transport must refuse it")
	}
	if err != nil {
		t.Fatalf("transferEntries returned %v, want nil (per-file failures are skipped)", err)
	}
}
