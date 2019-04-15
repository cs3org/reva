package ocdavsvc

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/utils"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func isChunked(fn string) (bool, error) {
	return regexp.MatchString(`-chunking-\w+-[0-9]+-[0-9]+$`, fn)
}

func sufferMacOSFinder(r *http.Request) bool {
	return r.Header.Get("X-Expected-Entity-Length") != ""
}

func handleMacOSFinder(w http.ResponseWriter, r *http.Request) error {
	/*
	   Many webservers will not cooperate well with Finder PUT requests,
	   because it uses 'Chunked' transfer encoding for the request body.
	   The symptom of this problem is that Finder sends files to the
	   server, but they arrive as 0-length files.
	   If we don't do anything, the user might think they are uploading
	   files successfully, but they end up empty on the server. Instead,
	   we throw back an error if we detect this.
	   The reason Finder uses Chunked, is because it thinks the files
	   might change as it's being uploaded, and therefore the
	   Content-Length can vary.
	   Instead it sends the X-Expected-Entity-Length header with the size
	   of the file at the very start of the request. If this header is set,
	   but we don't get a request body we will fail the request to
	   protect the end-user.
	*/

	content := r.Header.Get("Content-Length")
	expected := r.Header.Get("X-Expected-Entity-Length")
	logger.Build().Str("content-lenght", content).Str("x-expected-entity-length", expected).Msg(r.Context(), "Mac OS Finder corner-case detected")

	// The best mitigation to this problem is to tell users to not use crappy Finder.
	// Another possible mitigation is to change the use the value of X-Expected-Entity-Length header in the Content-Length header.
	expectedInt, err := strconv.ParseInt(expected, 10, 64)
	if err != nil {
		logger.Error(r.Context(), err)
		w.WriteHeader(http.StatusBadRequest)
		return err
	}
	r.ContentLength = expectedInt
	return nil
}

func isContentRange(r *http.Request) bool {
	/*
		   Content-Range is dangerous for PUT requests:  PUT per definition
		   stores a full resource.  draft-ietf-httpbis-p2-semantics-15 says
		   in section 7.6:
			 An origin server SHOULD reject any PUT request that contains a
			 Content-Range header field, since it might be misinterpreted as
			 partial content (or might be partial content that is being mistakenly
			 PUT as a full representation).  Partial content updates are possible
			 by targeting a separately identified resource with state that
			 overlaps a portion of the larger resource, or by using a different
			 method that has been specifically defined for partial updates (for
			 example, the PATCH method defined in [RFC5789]).
		   This clarifies RFC2616 section 9.6:
			 The recipient of the entity MUST NOT ignore any Content-*
			 (e.g. Content-Range) headers that it does not understand or implement
			 and MUST return a 501 (Not Implemented) response in such cases.
		   OTOH is a PUT request with a Content-Range currently the only way to
		   continue an aborted upload request and is supported by curl, mod_dav,
		   Tomcat and others.  Since some clients do use this feature which results
		   in unexpected behaviour (cf PEAR::HTTP_WebDAV_Client 1.0.1), we reject
		   all PUT requests with a Content-Range for now.
	*/
	return r.Header.Get("Content-Range") != ""
}

func (s *svc) doPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

	if r.Body == nil {
		logger.Println(ctx, "body is nil")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ok, err := isChunked(fn)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if ok {
		s.doPutChunked(w, r)
		return
	}

	if isContentRange(r) {
		logger.Println(ctx, "Content-Range not supported for PUT")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	if sufferMacOSFinder(r) {
		err := handleMacOSFinder(w, r)
		if err != nil {
			logger.Error(ctx, err)
			return
		}
	}

	client, err := s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
	}
	req := &storageproviderv0alphapb.StatRequest{Ref: ref}
	res, err := client.Stat(ctx, req)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code != rpcpb.Code_CODE_NOT_FOUND {
			logger.Println(ctx, res.Status)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	}

	info := res.Info
	if info != nil && info.Type != storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE {
		logger.Println(ctx, "resource is not a file")
		w.WriteHeader(http.StatusConflict)
		return
	}

	if info != nil {
		clientETag := r.Header.Get("If-Match")
		serverETag := info.Etag
		if clientETag != "" {
			serverETag = fmt.Sprintf(`"%s"`, serverETag)
			if clientETag != serverETag {
				logger.Build().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg(ctx, "etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	req2 := &storageproviderv0alphapb.InitiateFileUploadRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		},
	}

	// where to upload the file?
	res2, err := client.InitiateFileUpload(ctx, req2)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	dataServerURL := res2.UploadEndpoint
	// TODO(labkode): do a protocol switch
	httpReq, err := http.NewRequest("PUT", dataServerURL, r.Body)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO(labkode): harden http client
	// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if httpRes.StatusCode != http.StatusOK {
		logger.Println(ctx, httpRes.StatusCode)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err = client.Stat(ctx, req)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	info2 := res.Info

	w.Header().Add("Content-Type", info2.MimeType)
	w.Header().Set("ETag", info2.Etag)
	w.Header().Set("OC-FileId", fmt.Sprintf("%s:%s", info2.Id.StorageId, info2.Id.OpaqueId))
	w.Header().Set("OC-ETag", info2.Etag)
	t := utils.TSToTime(info2.Mtime)
	lastModifiedString := t.Format(time.RFC1123)
	w.Header().Set("Last-Modified", lastModifiedString)
	w.Header().Set("X-OC-MTime", "accepted")

	// file was new
	if info == nil {
		w.WriteHeader(http.StatusCreated)
		return
	}

	// overwrite
	w.WriteHeader(http.StatusNoContent)
	return
}
