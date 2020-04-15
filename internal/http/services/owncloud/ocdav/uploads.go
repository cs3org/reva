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

package ocdav

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/router"
	ctxuser "github.com/cs3org/reva/pkg/user"
)

// UploadsHandler handles chunked upload requests
type UploadsHandler struct {
	uploads map[string]string
}

func (h *UploadsHandler) init(c *Config) error {
	h.uploads = make(map[string]string)
	return nil
}

// Handler handles requests
func (h *UploadsHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		// TODO implement options https://tus.io/protocols/resumable-upload.html#options
		/*
			if r.Method == http.MethodOptions {
				s.doOptions(w, r)
				return
			}
		*/

		// MKCOL /remote.php/dav/uploads/demo/web-file-upload-c8639c42235c9ec26749a804aba61396-1569849691529
		// PUT   /remote.php/dav/uploads/demo/web-file-upload-c8639c42235c9ec26749a804aba61396-1569849691529/<offset>
		// MOVE  /remote.php/dav/uploads/demo/web-file-upload-c8639c42235c9ec26749a804aba61396-1569849691529/.file

		var username string
		username, r.URL.Path = router.ShiftPath(r.URL.Path)

		if username == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		u, ok := ctxuser.ContextGetUser(ctx)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if u.Username != username {
			// listing other users uploads is forbidden, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// uploadFolder is given by the client, eg: web-file-upload-c8639c42235c9ec26749a804aba61396-1569849691529
		//
		var uploadFolder string
		uploadFolder, r.URL.Path = router.ShiftPath(r.URL.Path)
		// TODO implement options https://tus.io/protocols/resumable-upload.html#options
		/*
			if r.Method == http.MethodOptions {
				s.doOptions(w, r)
				return
			}
		*/

		// we always need an upload folder
		if uploadFolder == "" {
			w.WriteHeader(http.StatusBadRequest)
			return

		}

		uploadPath := path.Join("/", u.Username, "._reva_atomic_upload_"+uploadFolder)

		if r.Method == "MKCOL" && r.URL.Path == "/" {
			h.createUpload(w, r, s, u, uploadPath)
			return
		}

		if r.Method == "PUT" && r.URL.Path != "/" {

			offset := r.Header.Get("OC-Chunk-Offset")
			if offset == "" {
				// try using the path name as offset
				offset, r.URL.Path = router.ShiftPath(r.URL.Path)
			}
			h.uploadChunk(w, r, s, u, uploadPath, offset)
			return
		}

		if r.Method == "MOVE" && r.URL.Path == "/.file" {
			dstHeader := r.Header.Get("Destination")

			log.Info().Str("uploadPath", uploadPath).Str("dst", dstHeader).Msg("assemble")

			if dstHeader == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// strip baseURL from destination
			dstURL, err := url.ParseRequestURI(dstHeader)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			urlPath := dstURL.Path

			// baseURI is encoded as part of the response payload in href field
			baseURI := path.Join("/", s.Prefix(), "remote.php/dav/files")
			ctx = context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)
			r = r.WithContext(ctx)

			log.Info().Str("url_path", urlPath).Str("base_uri", baseURI).Msg("move urls")
			i := strings.Index(urlPath, baseURI)
			if i == -1 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			dst := path.Clean(urlPath[len(baseURI):])

			h.assembleUpload(w, r, s, u, uploadPath, dst)
			return
		}

		http.Error(w, "404 Not found", http.StatusNotFound)
	})
}

func (h *UploadsHandler) createUpload(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, uploadPath string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	iuReq := &provider.InitiateFileUploadRequest{
		// TODO make clients send the final destination on the initial MKCOL, for now we invent one
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: uploadPath},
		},
	}
	totalLength := r.Header.Get("OC-Total-Length")
	if totalLength != "" {
		iuReq.Opaque = &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"Upload-Length": &typespb.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(totalLength),
				},
			},
		}
	}

	// TODO send oc-total-length if possible
	iuRes, err := client.InitiateFileUpload(ctx, iuReq)
	if err != nil {
		log.Error().Err(err).Msg("error initiating file upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if iuRes.Status.Code != rpc.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO remember upload id in a distributed store
	h.uploads[uploadPath] = iuRes.UploadEndpoint

	w.WriteHeader(http.StatusCreated)

}

func (h *UploadsHandler) uploadChunk(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, uploadPath string, offset string) {

	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	dataServerURL := h.uploads[uploadPath]

	// see http://tus.io for the protocol
	httpReq, err := rhttp.NewRequest(ctx, "PATCH", dataServerURL, r.Body)
	// tus headers:
	// TODO parallel uploads using tus Concatenation extension
	httpReq.Header.Set("Tus-Resumable", "1.0.0")
	httpReq.Header.Set("Content-Type", "application/offset+octet-stream")
	httpReq.Header.Set("Upload-Offset", offset)
	if err != nil {
		log.Error().Err(err).Msg("error creating http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	httpClient := rhttp.GetHTTPClient(ctx)
	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("error doing http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusNoContent {
		log.Error().Err(err).Int("status", httpRes.StatusCode).Msg("expected 204 No Content")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *UploadsHandler) assembleUpload(w http.ResponseWriter, r *http.Request, s *svc, u *userpb.User, uploadPath string, dst string) {

	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	overwrite := r.Header.Get("Overwrite")
	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// check tmp file exists

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: uploadPath},
		},
	}
	sRes, err := client.Stat(ctx, sReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		if sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	info := sRes.Info
	if info != nil && info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		log.Warn().Msg("resource is not a file")
		w.WriteHeader(http.StatusConflict)
		return
	}

	// move temp file to dst

	// TODO check if path is on same storage, return 502 on problems, see https://tools.ietf.org/html/rfc4918#section-9.9.4

	// check dst exists
	dstStatRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: dst},
	}
	dstStatReq := &provider.StatRequest{Ref: dstStatRef}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var successCode int
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		if overwrite == "F" {
			log.Warn().Str("dst", dst).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return
		}

		// only Delete dirs ... we want to keep the versons for files ...
		info := sRes.Info
		if info != nil && info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {

			// delete existing tree
			delReq := &provider.DeleteRequest{Ref: dstStatRef}
			delRes, err := client.Delete(ctx, delReq)
			if err != nil {
				log.Error().Err(err).Msg("error sending grpc delete request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// TODO return a forbidden status if read only?
			if delRes.Status.Code != rpc.Code_CODE_OK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		}

	} else {
		successCode = http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		ref2 := &provider.Reference{
			Spec: &provider.Reference_Path{Path: intermediateDir},
		}
		intStatReq := &provider.StatRequest{Ref: ref2}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusConflict) // 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return
		}
		// TODO what if intermediate is a file?
	}

	sourceRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: uploadPath},
	}
	dstRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: dst},
	}
	mReq := &provider.MoveRequest{Source: sourceRef, Destination: dstRef}
	mRes, err := client.Move(ctx, mReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending move grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if mRes.Status.Code != rpc.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	dstStatRes, err = client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dstStatRes.Status.Code != rpc.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	delete(h.uploads, uploadPath)

	info = dstStatRes.Info
	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info.Id))
	w.Header().Set("OC-ETag", info.Etag)
	w.WriteHeader(successCode)

	w.WriteHeader(http.StatusCreated)
}
