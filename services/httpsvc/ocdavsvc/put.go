package ocdavsvc

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

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

	req := &storageproviderv0alphapb.StatRequest{Filename: fn}
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

	md := res.Metadata
	if md != nil && md.IsDir {
		logger.Println(ctx, "resource is a folder")
		w.WriteHeader(http.StatusConflict)
		return
	}

	if md != nil {
		clientETag := r.Header.Get("If-Match")
		serverETag := md.Etag
		if clientETag != "" {
			serverETag = fmt.Sprintf(`"%s"`, serverETag)
			if clientETag != serverETag {
				logger.Build().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg(ctx, "etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	req2 := &storageproviderv0alphapb.StartWriteSessionRequest{}
	res2, err := client.StartWriteSession(ctx, req2)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res2.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res2.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sessID := res2.SessionId
	logger.Build().Str("sessID", sessID).Msg(ctx, "got write session id")

	stream, err := client.Write(ctx)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	buffer := make([]byte, 1024*1024*3)
	var offset uint64
	var numChunks uint64

	for {
		n, err := r.Body.Read(buffer)
		if n > 0 {
			req := &storageproviderv0alphapb.WriteRequest{Data: buffer, Length: uint64(n), SessionId: sessID, Offset: offset}
			err = stream.Send(req)
			if err != nil {
				logger.Error(ctx, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			numChunks++
			offset += uint64(n)
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	res3, err := stream.CloseAndRecv()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res3.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req4 := &storageproviderv0alphapb.FinishWriteSessionRequest{Filename: fn, SessionId: sessID}
	res4, err := client.FinishWriteSession(ctx, req4)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res4.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res4.Status)
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

	md2 := res.Metadata

	w.Header().Add("Content-Type", md2.Mime)
	w.Header().Set("ETag", md2.Etag)
	w.Header().Set("OC-FileId", md2.Id)
	w.Header().Set("OC-ETag", md2.Etag)
	t := time.Unix(int64(md2.Mtime), 0)
	lastModifiedString := t.Format(time.RFC1123)
	w.Header().Set("Last-Modified", lastModifiedString)
	w.Header().Set("X-OC-MTime", "accepted")

	if md == nil {
		w.WriteHeader(http.StatusCreated)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	return
}
