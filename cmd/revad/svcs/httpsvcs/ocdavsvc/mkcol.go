package ocdavsvc

import (
	"errors"
	"io"
	"net/http"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func (s *svc) doMkcol(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

	buf := make([]byte, 1)
	_, err := r.Body.Read(buf)
	if err != io.EOF {
		logger.Error(ctx, errors.New("unexpected body"))
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	client, err := s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageproviderv0alphapb.CreateDirectoryRequest{Filename: fn}
	res, err := client.CreateDirectory(ctx, req)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		logger.Println(ctx, res.Status)
		w.WriteHeader(http.StatusConflict)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
