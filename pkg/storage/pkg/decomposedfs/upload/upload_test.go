package upload_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/aspects"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/options"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/upload"
)

func TestServeContent(t *testing.T) {
	log := &zerolog.Logger{}
	root := t.TempDir()
	store := upload.NewSessionStore(nil, aspects.Aspects{}, root, false, options.TokenOptions{}, log)
	session := store.New(context.Background())

	root = filepath.Join(root, "uploads")
	assert.NoError(t, os.MkdirAll(root, 0755))

	tmpFile, err := os.Create(filepath.Join(root, session.ID()))
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, tmpFile.Close())
	}()

	_, err = tmpFile.WriteString("Hello, World!")
	assert.NoError(t, err)

	req, err := http.NewRequest("GET", "/", nil)
	assert.NoError(t, err)

	t.Run("contains the whole file without a range header", func(t *testing.T) {
		rr := httptest.NewRecorder()

		assert.NoError(t, session.ServeContent(context.Background(), rr, req))
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Empty(t, rr.Header().Get("Content-Range"))

		body, err := io.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t, "Hello, World!", string(body))
	})

	t.Run("contains the whole file with a range header even if the range is invalid", func(t *testing.T) {
		req.Header.Set("Range", "bytes=0-100")
		rr := httptest.NewRecorder()

		assert.NoError(t, session.ServeContent(context.Background(), rr, req))
		assert.Equal(t, http.StatusPartialContent, rr.Code)
		assert.Equal(t, "bytes 0-12/13", rr.Header().Get("Content-Range"))

		body, err := io.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t, "Hello, World!", string(body))
	})

	t.Run("contains bytes 0-4", func(t *testing.T) {
		req.Header.Set("Range", "bytes=0-4")
		rr := httptest.NewRecorder()

		assert.NoError(t, session.ServeContent(context.Background(), rr, req))
		assert.Equal(t, http.StatusPartialContent, rr.Code)
		assert.Equal(t, "bytes 0-4/13", rr.Header().Get("Content-Range"))

		body, err := io.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t, "Hello", string(body))
	})

	t.Run("contains bytes 4-4", func(t *testing.T) {
		req.Header.Set("Range", "bytes=4-4")
		rr := httptest.NewRecorder()

		assert.NoError(t, session.ServeContent(context.Background(), rr, req))
		assert.Equal(t, http.StatusPartialContent, rr.Code)
		assert.Equal(t, "bytes 4-4/13", rr.Header().Get("Content-Range"))

		body, err := io.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t, "o", string(body))
	})
}
