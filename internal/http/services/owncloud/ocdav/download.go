// Copyright 2018-2023 CERN
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
	"io"
	"net/http"
	"path"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/archiver/manager"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/pkg/storage/utils/walker"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
)

func (s *svc) handleLegacyPublicLinkDownload(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/")
	files := getFilesFromRequest(r)
	s.downloadFiles(r.Context(), w, token, files)
}

func getFilesFromRequest(r *http.Request) []string {
	q := r.URL.Query()
	dir := q.Get("path")
	files := []string{}

	if q.Get("files") != "" {
		files = append(files, path.Join(dir, q.Get("files")))
	} else {
		for _, f := range q["files[]"] {
			files = append(files, path.Join(dir, f))
		}
	}
	return files
}

func (s *svc) authenticate(ctx context.Context, token string) (context.Context, error) {
	// TODO (gdelmont): support password protected public links
	c, err := s.getClient()
	if err != nil {
		return nil, err
	}
	res, err := c.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "publicshares",
		ClientId:     token,
		ClientSecret: "password|",
	})
	if err != nil {
		return nil, err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound(token)
		}
		return nil, errors.New(res.Status.Message)
	}

	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, res.Token)
	ctx = ctxpkg.ContextSetToken(ctx, res.Token)

	return ctx, nil
}

func (s *svc) handleHTTPError(w http.ResponseWriter, err error, log *zerolog.Logger) {
	log.Error().Err(err).Msg("ocdav: got error")
	switch err.(type) {
	case errtypes.NotFound:
		http.Error(w, "Resource not found", http.StatusNotFound)
	case errtypes.PermissionDenied:
		http.Error(w, "Permission denied", http.StatusForbidden)
	case manager.ErrMaxSize, manager.ErrMaxFileCount:
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *svc) downloadFiles(ctx context.Context, w http.ResponseWriter, token string, files []string) {
	log := appctx.GetLogger(ctx)
	ctx, err := s.authenticate(ctx, token)
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}
	isSingleFileShare, res, err := s.isSingleFileShare(ctx, token, files)
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}
	if isSingleFileShare {
		s.downloadFile(ctx, w, res)
	} else {
		s.downloadArchive(ctx, w, token, files)
	}
}

func (s *svc) isSingleFileShare(ctx context.Context, token string, files []string) (bool, *provider.ResourceInfo, error) {
	switch len(files) {
	case 0:
		return s.resourceIsFileInPublicLink(ctx, token, "")
	case 1:
		return s.resourceIsFileInPublicLink(ctx, token, files[0])
	default:
		// FIXME (gdelmont): even if the list contains more than one file
		// these (or part of them), could not exist
		// in this case, filtering the existing ones, we could
		// end up having 0 or 1 files
		return false, nil, nil
	}
}

func (s *svc) resourceIsFileInPublicLink(ctx context.Context, token, file string) (bool, *provider.ResourceInfo, error) {
	res, err := s.getResourceFromPublicLinkToken(ctx, token, file)
	if err != nil {
		return false, nil, err
	}
	return res.Type == provider.ResourceType_RESOURCE_TYPE_FILE, res, nil
}

func (s *svc) getResourceFromPublicLinkToken(ctx context.Context, token, file string) (*provider.ResourceInfo, error) {
	c, err := s.getClient()
	if err != nil {
		return nil, err
	}
	res, err := c.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{
		Token: token,
	})
	if err != nil {
		return nil, err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound(token)
		}
		return nil, errtypes.InternalError(res.Status.Message)
	}

	statRes, err := c.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: res.Share.ResourceId, Path: file}})
	if err != nil {
		return nil, err
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound(token)
		} else if statRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
			return nil, errtypes.PermissionDenied(file)
		}
		return nil, errtypes.InternalError(statRes.Status.Message)
	}
	return statRes.Info, nil
}

func (s *svc) downloadFile(ctx context.Context, w http.ResponseWriter, res *provider.ResourceInfo) {
	log := appctx.GetLogger(ctx)
	c, err := s.getClient()
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}
	d := downloader.NewDownloader(c)
	r, err := d.Download(ctx, res.Path, "")
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}
	defer r.Close()

	w.WriteHeader(http.StatusOK)

	_, err = io.Copy(w, r)
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}
}

func getPublicLinkResources(rootFolder, token string, files []string) []string {
	r := make([]string, 0, len(files))
	for _, f := range files {
		r = append(r, path.Join(rootFolder, token, f))
	}
	if len(r) == 0 {
		r = []string{path.Join(rootFolder, token)}
	}
	return r
}

func (s *svc) downloadArchive(ctx context.Context, w http.ResponseWriter, token string, files []string) {
	log := appctx.GetLogger(ctx)
	resources := getPublicLinkResources(s.c.PublicLinkDownload.PublicFolder, token, files)

	gtw, err := s.getClient()
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}

	downloader := downloader.NewDownloader(gtw, rhttp.Context(ctx))
	walker := walker.NewWalker(gtw)

	archiver, err := manager.NewArchiver(resources, walker, downloader, manager.Config{
		MaxNumFiles: s.c.PublicLinkDownload.MaxNumFiles,
		MaxSize:     s.c.PublicLinkDownload.MaxSize,
	})
	if err != nil {
		s.handleHTTPError(w, err, log)
		return
	}

	if err := archiver.CreateTar(ctx, w); err != nil {
		s.handleHTTPError(w, err, log)
		return
	}
}
