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

package ocdavsvc

import (
/*
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
*/
)

/* TODO(jfd): refactor with out-of-band
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

	// check src exists
	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: src},
	}
	srcStatReq := &storageproviderv0alphapb.StatRequest{Ref: ref}
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
	ref = &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
	}
	dstStatReq := &storageproviderv0alphapb.StatRequest{Ref: ref}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var successCode int
	if dstStatRes.Status.Code == rpcpb.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			logger.Println(ctx, "destination already exists: ", dst)
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}

	} else {
		successCode = http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		ref = &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: intermediateDir},
		}
		intStatReq := &storageproviderv0alphapb.StatRequest{Ref : ref}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			logger.Println(ctx, "intermediateDir:", intermediateDir)
			w.WriteHeader(http.StatusConflict) // 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}
		// TODO what if intermediate is a file?
	}

	err = descend(ctx, client, srcStatRes.Info, dst)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}

func descend(ctx context.Context, client storageproviderv0alphapb.StorageProviderServiceClient, src *storageproviderv0alphapb.ResourceInfo, dst string) error {
	logger.Println(ctx, "descend src:", src, " dst:", dst)
	if src.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: dst},
		}
		createReq := &storageproviderv0alphapb.CreateContainerRequest{Ref: ref}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil || createRes.Status.Code != rpcpb.Code_CODE_OK {
			return err
		}

		// descend for children
		ref2 := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: src.Path},
		}
		listReq := &storageproviderv0alphapb.ListContainerRequest{ Ref: ref }
		stream, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}

		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}

			if err != nil || res.Status.Code != rpcpb.Code_CODE_OK {
				return err
			}

			childDst := path.Join(dst, path.Base(res.Metadata.Filename))
			descend(ctx, client, res.Metadata, childDst)
		}

	} else {
		// copy file

		readReq := &storageproviderv0alphapb.ReadRequest{Filename: src.Filename}
		readStream, err := client.Read(ctx, readReq)
		if err != nil {
			return err
		}

		startReq := &storageproviderv0alphapb.StartWriteSessionRequest{}
		writeSess, err := client.StartWriteSession(ctx, startReq)
		if err != nil || writeSess.Status.Code != rpcpb.Code_CODE_OK {
			return err
		}

		sessID := writeSess.SessionId
		logger.Build().Str("sessID", sessID).Msg(ctx, "got write session id")

		writeStream, err := client.Write(ctx)
		if err != nil {
			return err
		}

		for {
			res, err := readStream.Recv()
			if err == io.EOF {
				break
			}

			if err != nil || res.Status.Code != rpcpb.Code_CODE_OK {
				return err
			}

			dc := res.DataChunk
			if dc.Length > 0 {
				req := &storageproviderv0alphapb.WriteRequest{Data: dc.Data, Length: dc.Length, SessionId: sessID, Offset: dc.Offset}
				err = writeStream.Send(req)
				if err != nil {
					return err
				}
			}
		}

		closeRes, err := writeStream.CloseAndRecv()
		if err != nil || closeRes.Status.Code != rpcpb.Code_CODE_OK {
			return err
		}

		finishReq := &storageproviderv0alphapb.FinishWriteSessionRequest{Filename: dst, SessionId: sessID}
		finishRes, err := client.FinishWriteSession(ctx, finishReq)
		if err != nil || finishRes.Status.Code != rpcpb.Code_CODE_OK {
			return err
		}

	}
	return nil
}
*/
