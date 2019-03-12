package ocdavsvc

import (
	"net/http"
	"net/url"
	"strings"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func (s *svc) doCopy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	src := r.URL.Path
	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")

	logger.Build().Str("source", src).Str("destination", dstHeader).Str("overwrite", overwrite).Msg(ctx, "copy")

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
	logger.Println(r.Context(), "Copy urlPath=", urlPath, " baseURI=", baseURI)
	i := strings.Index(urlPath, baseURI)
	if i == -1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dst := urlPath[len(baseURI):]

	if overwrite == "F" {
		dstStatReq := &storageproviderv0alphapb.StatRequest{Filename: dst}
		dstStatRes, err := client.Stat(ctx, dstStatReq)
		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if dstStatRes.Status.Code == rpcpb.Code_CODE_OK {
			logger.Println(ctx, "destination already exists: ", dst)
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
	}

	w.WriteHeader(http.StatusNotImplemented)
	return
	/*
		req := &storageproviderv0alphapb.CopyRequest{SourceFilename: src, TargetFilename: dst}
		res, err := client.Copy(ctx, req)
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
		w.WriteHeader(http.StatusCreated)
	*/
}
