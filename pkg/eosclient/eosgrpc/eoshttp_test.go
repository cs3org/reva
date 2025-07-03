package eosgrpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/eosclient"
)

// Test that, when the PUTFile method is called with disableVersioning
// set to true, the url for the EOS endpoint contains the right query param
func TestDisableVersioningLeadsToCorrectQueryParams(t *testing.T) {

	stream := io.NopCloser(strings.NewReader("Hello world!"))
	length := int64(12)
	app := "my-app"
	urlpath := "/my-file.txt?queryparam=1"
	token := "my-secret-token"

	// Create fake HTTP server that acts as the EOS endpoint
	calls := 0
	mockServerUpload := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				calls++
				queryValues := r.URL.Query()
				if queryValues.Get("eos.versioning") == "" {
					t.Errorf("Query parameter eos.versioning not set")
				}
				if q := queryValues.Get("eos.versioning"); q != strconv.Itoa(0) {
					t.Errorf("Query parameter eos.versioning set to wrong value; got %s, expected 0", q)
				}
			},
		),
	)

	// Create EOS HTTP Client
	// TODO: right now, expects files to be on the FS
	client, err := NewEOSHTTPClient(&HTTPOptions{
		BaseURL: mockServerUpload.URL,
	})
	if err != nil {
		t.Errorf("Failed to construct client: %s", err.Error())
	}

	// Test actual PUTFile call
	client.PUTFile(context.Background(), "remote-user", eosclient.Authorization{
		Token: token}, urlpath, stream, length, app, true)

	// If no connection was made to the EOS endpoint, something is wrong
	if calls == 0 {
		t.Errorf("EOS endpoint was not called. ")
	}
}
