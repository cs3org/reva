package ocdavsvc

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func (s *svc) doMove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	src := r.URL.Path
	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")

	logger.Build().Str("source", src).Str("destination", dstHeader).Str("overwrite", overwrite).Msg(ctx, "move")

	if dstHeader == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// strip baseURL from destination
	dstURL, err := url.ParseRequestURI(dstHeader)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	urlPath := dstURL.Path
	baseURI := r.Context().Value("baseuri").(string)
	logger.Println(r.Context(), "Move urlPath=", urlPath, " baseURI=", baseURI)
	i := strings.Index(urlPath, baseURI)
	if i == -1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// check src exists
	srcStatReq := &storageproviderv0alphapb.StatRequest{Filename: src}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if srcStatRes.Status.Code != rpcpb.Code_CODE_OK {
		if srcStatRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		logger.Println(ctx, srcStatRes.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO check if path is on same storage, return 502 on problems, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	dst := path.Clean(urlPath[len(baseURI):])

	// check dst exists
	dstStatReq := &storageproviderv0alphapb.StatRequest{Filename: dst}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var successCode int
	if dstStatRes.Status.Code == rpcpb.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		if overwrite == "F" {
			logger.Println(ctx, "destination already exists: ", dst)
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return
		}

		// delete existing tree
		delReq := &storageproviderv0alphapb.DeleteRequest{Filename: dst}
		delRes, err := client.Delete(ctx, delReq)
		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// TODO return a forbidden status if read only?
		if delRes.Status.Code != rpcpb.Code_CODE_OK {
			logger.Println(ctx, delRes.Status)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		successCode = http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		intStatReq := &storageproviderv0alphapb.StatRequest{Filename: intermediateDir}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			logger.Println(ctx, "intermediateDir:", intermediateDir)
			w.WriteHeader(http.StatusConflict) // 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return
		}
		// TODO what if intermediate is a file?
	}

	req := &storageproviderv0alphapb.MoveRequest{SourceFilename: src, TargetFilename: dst}
	res, err := client.Move(ctx, req)
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

	req2 := &storageproviderv0alphapb.StatRequest{Filename: dst}
	res2, err := client.Stat(ctx, req2)
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

	md := res2.Metadata
	w.Header().Set("Content-Type", md.Mime)
	w.Header().Set("ETag", md.Etag)
	w.Header().Set("OC-FileId", md.Id)
	w.Header().Set("OC-ETag", md.Etag)
	w.WriteHeader(successCode)
}
