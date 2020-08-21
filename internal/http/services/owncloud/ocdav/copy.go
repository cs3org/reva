// Copyright 2018-2020 CERN
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

package ocdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/eventials/go-tus"
	"github.com/eventials/go-tus/memorystore"
)

func (s *svc) handleCopy(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	ns = applyLayout(ctx, ns)

	src := path.Join(ns, r.URL.Path)
	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "infinity"
	}

	dst, err := extractDestination(dstHeader, r.Context().Value(ctxKeyBaseURI).(string))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dst = path.Join(ns, dst)

	log.Info().Str("source", src).Str("destination", dst).
		Str("overwrite", overwrite).Str("depth", depth).Msg("copy")

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if depth != "infinity" && depth != "0" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check src exists
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: src},
	}
	srcStatReq := &provider.StatRequest{Ref: ref}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		if srcStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check dst exists
	ref = &provider.Reference{
		Spec: &provider.Reference_Path{Path: dst},
	}
	dstStatReq := &provider.StatRequest{Ref: ref}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var successCode int
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			log.Warn().Str("dst", dst).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}

	} else {
		successCode = http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		ref = &provider.Reference{
			Spec: &provider.Reference_Path{Path: intermediateDir},
		}
		intStatReq := &provider.StatRequest{Ref: ref}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusConflict) // 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}
		// TODO what if intermediate is a file?
	}

	err = s.descend(ctx, client, srcStatRes.Info, dst, depth == "infinity")
	if err != nil {
		log.Error().Err(err).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}

func (s *svc) descend(ctx context.Context, client gateway.GatewayAPIClient, src *provider.ResourceInfo, dst string, recurse bool) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", src.Path).Str("dst", dst).Msg("descending")
	if src.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &provider.CreateContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: dst},
			},
		}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil || createRes.Status.Code != rpc.Code_CODE_OK {
			return err
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if !recurse {
			return nil
		}

		// descend for children
		listReq := &provider.ListContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: src.Path},
			},
		}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", res.Status.Code)
		}

		for i := range res.Infos {
			childDst := path.Join(dst, path.Base(res.Infos[i].Path))
			err := s.descend(ctx, client, res.Infos[i], childDst, recurse)
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		// 1. get download url
		dReq := &provider.InitiateFileDownloadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: src.Path},
			},
		}

		dRes, err := client.InitiateFileDownload(ctx, dReq)
		if err != nil {
			return err
		}

		if dRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", dRes.Status.Code)
		}

		// 2. get upload url

		uReq := &provider.InitiateFileUploadRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{Path: dst},
			},
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(fmt.Sprintf("%d", src.GetSize())),
					},
				},
			},
		}

		uRes, err := client.InitiateFileUpload(ctx, uReq)
		if err != nil {
			return err
		}

		if uRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", uRes.Status.Code)
		}

		// 3. do download

		httpDownloadReq, err := rhttp.NewRequest(ctx, "GET", dRes.DownloadEndpoint, nil)
		if err != nil {
			return err
		}
		httpDownloadReq.Header.Set(datagateway.TokenTransportHeader, dRes.Token)

		httpDownloadClient := rhttp.GetHTTPClient(
			rhttp.Context(ctx),
			rhttp.Timeout(time.Duration(s.c.Timeout*int64(time.Second))),
			rhttp.Insecure(s.c.Insecure),
		)

		httpDownloadRes, err := httpDownloadClient.Do(httpDownloadReq)
		if err != nil {
			return err
		}
		defer httpDownloadRes.Body.Close()

		if httpDownloadRes.StatusCode != http.StatusOK {
			return fmt.Errorf("status code %d", httpDownloadRes.StatusCode)
		}

		// do upload
		err = s.tusUpload(ctx, uRes.UploadEndpoint, uRes.Token, dst, httpDownloadRes.Body, src.GetSize())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *svc) tusUpload(ctx context.Context, dataServerURL string, transferToken string, fn string, body io.Reader, length uint64) error {
	var err error
	log := appctx.GetLogger(ctx)

	// create the tus client.
	c := tus.DefaultConfig()
	c.Resume = true
	c.HttpClient = rhttp.GetHTTPClient(
		rhttp.Context(ctx),
		rhttp.Timeout(time.Duration(s.c.Timeout*int64(time.Second))),
		rhttp.Insecure(s.c.Insecure),
	)
	c.Store, err = memorystore.NewMemoryStore()
	if err != nil {
		return err
	}

	log.Debug().
		Str("header", tokenpkg.TokenHeader).
		Str("token", tokenpkg.ContextMustGetToken(ctx)).
		Msg("adding token to header")
	c.Header.Set(tokenpkg.TokenHeader, tokenpkg.ContextMustGetToken(ctx))
	c.Header.Set(datagateway.TokenTransportHeader, transferToken)

	tusc, err := tus.NewClient(dataServerURL, c)
	if err != nil {
		return nil
	}

	// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2
	metadata := map[string]string{
		"filename": path.Base(fn),
		"dir":      path.Dir(fn),
		//"checksum": fmt.Sprintf("%s %s", storageprovider.GRPC2PKGXS(xsType).String(), xs),
	}
	log.Debug().
		Str("length", fmt.Sprintf("%d", length)).
		Str("filename", path.Base(fn)).
		Str("dir", path.Dir(fn)).
		Msg("tus.NewUpload")

	upload := tus.NewUpload(body, int64(length), metadata, "")

	// create the uploader.
	c.Store.Set(upload.Fingerprint, dataServerURL)
	uploader := tus.NewUploader(tusc, dataServerURL, upload, 0)

	// start the uploading process.
	err = uploader.Upload()
	if err != nil {
		return err
	}
	return nil
}
