package ocdav

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
)

// index.php/s/jIKrtrkXCIXwg1y/download?path=%2FHugo&files=Intrinsico
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

func (s *svc) handleHttpError(w http.ResponseWriter, err error, log *zerolog.Logger) {
	log.Err(err).Msg("ocdav: got error")
	switch err.(type) {
	case errtypes.NotFound:
		http.Error(w, "Resource not found", http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *svc) downloadFiles(ctx context.Context, w http.ResponseWriter, token string, files []string) {
	log := appctx.GetLogger(ctx)
	ctx, err := s.authenticate(ctx, token)
	if err != nil {
		s.handleHttpError(w, err, log)
		return
	}
	isSingleFileShare, res, err := s.isSingleFileShare(ctx, token, files)
	if err != nil {
		s.handleHttpError(w, err, log)
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

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound(token)
		}
		return nil, errtypes.InternalError(res.Status.Message)
	}
	return statRes.Info, nil
}

func (s *svc) downloadFile(ctx context.Context, w http.ResponseWriter, res *provider.ResourceInfo) {
	log := appctx.GetLogger(ctx)
	c, err := s.getClient()
	if err != nil {
		s.handleHttpError(w, err, log)
		return
	}
	d := downloader.NewDownloader(c)
	r, err := d.Download(ctx, res.Path)
	if err != nil {
		s.handleHttpError(w, err, log)
		return
	}
	defer r.Close()

	w.WriteHeader(http.StatusOK)

	_, err = io.Copy(w, r)
	if err != nil {
		s.handleHttpError(w, err, log)
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

func prepareArchiverURL(endpoint string, files []string) string {
	q := url.Values{}
	for _, f := range files {
		q.Add("file", f)
	}
	u, _ := url.Parse(endpoint)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *svc) downloadArchive(ctx context.Context, w http.ResponseWriter, token string, files []string) {
	log := appctx.GetLogger(ctx)
	resources := getPublicLinkResources(s.c.PublicLinkDownload.PublicFolder, token, files)
	url := prepareArchiverURL(s.c.PublicLinkDownload.ArchiverEndpoint, resources)

	req, err := rhttp.NewRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		s.handleHttpError(w, err, log)
		return
	}

	res, err := s.client.Do(req)
	if err != nil {
		s.handleHttpError(w, err, log)
		return
	}
	defer res.Body.Close()

	w.WriteHeader(http.StatusOK)

	_, err = io.Copy(w, res.Body)
	if err != nil {
		s.handleHttpError(w, err, log)
		return
	}
}
