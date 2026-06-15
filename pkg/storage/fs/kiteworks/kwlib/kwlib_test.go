package kwlib_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/owncloud/reva/v2/pkg/storage/fs/kiteworks/kwlib"
	"github.com/rs/zerolog"
)

// --- Time.MarshalJSON ---

func TestTimeMarshalJSON_nil(t *testing.T) {
	var tp *kwlib.Time
	b, err := json.Marshal(tp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != "null" {
		t.Fatalf("want null, got %s", b)
	}
}

func TestTimeMarshalJSON_roundtrip(t *testing.T) {
	original := kwlib.Time(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	b, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got kwlib.Time
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if time.Time(original) != time.Time(got) {
		t.Fatalf("want %v, got %v", time.Time(original), time.Time(got))
	}
}

// --- decode error propagation ---

func badJSONServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
}

func newClient(t *testing.T, srv *httptest.Server) *kwlib.APIClient {
	t.Helper()
	f := kwlib.NewClientFactory(srv.URL, "", false)
	nop := zerolog.Nop()
	return f.Build("", "", "", "tok", &nop)
}

func TestDecodeError_returnsNil(t *testing.T) {
	srv := badJSONServer()
	defer srv.Close()
	c := newClient(t, srv)

	cases := []struct {
		name string
		call func() (any, error)
	}{
		{"GetTopFolders", func() (any, error) { return c.GetTopFolders() }},
		{"GetFolderByID", func() (any, error) { return c.GetFolderByID("x") }},
		{"GetFileByID", func() (any, error) { return c.GetFileByID("x") }},
		{"ListFolderContents", func() (any, error) { v, e := c.ListFolderContents("x"); return v, e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.call()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// --- SendRequest: 4xx/5xx is error ---

func serverWith(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestSendRequest_404_isError(t *testing.T) {
	srv := serverWith(http.StatusNotFound, `{"error":"nope"}`)
	defer srv.Close()
	c := newClient(t, srv)
	req, _ := c.NewGetRequest("/rest/folders/top")
	_, err := c.SendRequest(req)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var ce *kwlib.ClientError
	if !errors.As(err, &ce) || ce.StatusCode != http.StatusNotFound {
		t.Fatalf("expected ClientError(404), got %v", err)
	}
}

func TestSendRequest_500_isError(t *testing.T) {
	srv := serverWith(http.StatusInternalServerError, `oops`)
	defer srv.Close()
	c := newClient(t, srv)
	req, _ := c.NewGetRequest("/rest/folders/top")
	_, err := c.SendRequest(req)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}
