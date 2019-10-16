// Copyright 2018-2019 CERN
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

package thumbnails

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/nfnt/resize"
	"github.com/rs/zerolog"
)

type thumnailContext struct {
	parent  *svc
	log     *zerolog.Logger
	writer  http.ResponseWriter
	request *http.Request
	url     string

	fileType string
	size     uint

	tempFileName string
}

func newThumbnail(s *svc, w http.ResponseWriter, r *http.Request) *thumnailContext {
	log := appctx.GetLogger(r.Context())
	var u *url.URL
	if s.conf.WebDavHost == "" {
		u = &url.URL{}
		u.Scheme = "http" // from where should that come
		u.Host = r.Host
	} else {
		var err error
		u, err = url.Parse(s.conf.WebDavHost)
		if err != nil {
			log.Err(err).Msg("Failed to partse host: " + s.conf.WebDavHost)
			w.WriteHeader(http.StatusInternalServerError)
			return nil
		}
	}
	u.Path += "/remote.php/dav/files" + r.URL.Path

	query := r.URL.Query()
	fileType := query.Get("type")
	if fileType == "" {
		fileType = "png"
	}
	switch fileType {
	case "png":
	case "jpg":
		fallthrough
	case "jpeg":
		fileType = "jpg"
	case "":
		fileType = "png"
	default:
		log.Error().Msg("Unsupported file type: " + fileType)
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}

	size, err := strconv.Atoi(query.Get("size"))
	if err != nil {
		log.Err(err).Msg("error parsing size, size" + query.Get("size"))
		w.WriteHeader(http.StatusInternalServerError)
		return nil
	}
	return &thumnailContext{parent: s, log: log, writer: w, request: r, url: u.String(), size: uint(size), fileType: fileType}

}

func (t *thumnailContext) CheckCache() bool {
	ctx := t.request.Context()
	httpClient := rhttp.GetHTTPClient(ctx)

	httpReq, err := rhttp.NewRequest(ctx, "HEAD", t.url, nil)
	if err != nil {
		t.log.Error().Err(err).Msg("error creating http request")
		t.writer.WriteHeader(http.StatusInternalServerError)
		return true
	}
	httpReq.Header = t.request.Header.Clone()
	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		t.log.Error().Err(err).Msg("error performing http request")
		t.writer.WriteHeader(http.StatusInternalServerError)
		return true
	}
	httpRes.Body.Close()
	etag := httpRes.Header.Get("Etag")
	if etag == t.request.Header.Get("If-None-Match") {
		t.writer.WriteHeader(http.StatusNotModified)
		t.log.Info().Msg("Etag match, nothing to do")
		return true
	}
	t.tempFileName, err = t.getTmpName(etag)
	if err != nil {
		t.writer.WriteHeader(http.StatusInternalServerError)
		t.log.Err(err).Msg("error getting cache name")
		return true
	}
	if t.parent.Cache[t.tempFileName] {
		data, err := ioutil.ReadFile(t.tempFileName)
		if err != nil {
			t.log.Err(err).Msg("error restoring cached image")
		} else if len(data) != 0 {
			t.log.Info().Msg("Restore thumbnail from cache")
			_, err = t.writer.Write(data)
			if err != nil {
				t.writer.WriteHeader(http.StatusInternalServerError)
				t.log.Err(err).Msg("error writing response")
			}
			return true
		}
	}
	return false
}

func (t *thumnailContext) GetFile() *http.Response {
	ctx := t.request.Context()
	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "GET", t.url, nil)
	if err != nil {
		t.log.Error().Err(err).Msg("error creating http request")
		t.writer.WriteHeader(http.StatusInternalServerError)
		return nil
	}
	httpReq.Header = t.request.Header.Clone()

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		t.log.Error().Err(err).Msg("error performing http request")
		t.writer.WriteHeader(http.StatusInternalServerError)
		return nil
	}
	return httpRes
}

func (t *thumnailContext) GenerateThumbnail(httpRes *http.Response) {
	image, _, err := image.Decode(httpRes.Body)

	if err != nil {
		t.log.Err(err).Msg("Failed to decode img")
		t.writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	thumnail := resize.Thumbnail(t.size, t.size, image, resize.Lanczos3)

	buf := new(bytes.Buffer)

	switch t.fileType {
	case "png":
		err = png.Encode(buf, thumnail)
	case "jpg":
		err = jpeg.Encode(buf, thumnail, nil)
	}
	if err != nil {
		t.log.Err(err).Msg("error encoding image")
		t.writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	data := buf.Bytes()
	err = ioutil.WriteFile(t.tempFileName, data, 0755)
	if err != nil {
		t.writer.WriteHeader(http.StatusInternalServerError)
		t.log.Err(err).Msg("error creating cache file")
	}
	t.parent.Cache[t.tempFileName] = true

	_, err = t.writer.Write(data)
	if err != nil {
		t.writer.WriteHeader(http.StatusInternalServerError)
		t.log.Err(err).Msg("error writing response")
	}
}

func cacheFolder() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return path.Join(cacheDir, "revad", "thumbnails"), nil
}

func (t *thumnailContext) getTmpName(etag string) (string, error) {
	if etag == "" {
		return "", errors.New("No etag provided")
	}
	hash := md5.Sum([]byte(etag + t.fileType + strconv.FormatUint(uint64(t.size), 10)))
	cache, err := cacheFolder()
	if err != nil {
		return "", err
	}
	out := path.Join(cache, hex.EncodeToString(hash[:]))
	err = os.MkdirAll(path.Dir(out), 0755)
	if err != nil {
		return "", err
	}
	return out, nil
}
