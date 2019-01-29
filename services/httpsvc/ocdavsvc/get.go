package ocdavsvc

import (
	"bytes"
	"io"
	"net/http"
	"time"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func (s *svc) doGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

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
		logger.Println(ctx, res.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	md := res.Metadata
	if md.IsDir {
		logger.Println(ctx, "resource is a folder, cannot be downloaded")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	req2 := &storageproviderv0alphapb.ReadRequest{Filename: fn}
	stream, err := client.Read(ctx, req2)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", md.Mime)
	w.Header().Set("ETag", md.Etag)
	w.Header().Set("OC-FileId", md.Id)
	w.Header().Set("OC-ETag", md.Etag)
	t := time.Unix(int64(md.Mtime), 0)
	lastModifiedString := t.Format(time.RFC1123)
	w.Header().Set("Last-Modified", lastModifiedString)
	if md.Checksum != "" {
		w.Header().Set("OC-Checksum", md.Checksum)
	}

	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			logger.Println(ctx, res)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var reader io.Reader
		dc := res.DataChunk
		if dc.Length > 0 {
			reader = bytes.NewReader(dc.Data)
			_, err = io.CopyN(w, reader, int64(dc.Length))
			if err != nil {
				logger.Error(ctx, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
}
