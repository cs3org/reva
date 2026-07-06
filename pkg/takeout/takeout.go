package takeout

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/v3/pkg/storage/utils/walker"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
)

/* Job registration */

// Takeout job name
const JobName = "takeout"

// Init registers the on-demand takeout job
func init() {
	if err := rjobs.RegisterOnDemand(JobName, New); err != nil {
		panic(err)
	}
}

/* Job's configuration setup */

// The takeout job config
type config struct {
	MachineSecret        string `mapstructure:"machine_secret" validate:"required"`
	TakeoutAdminUsername string `mapstructure:"takeout_admin_username" validate:"required"`
	PublicURL            string `mapstructure:"public_url" validate:"required"`
}

// New sets the potential custom job config
func New(ctx context.Context, m map[string]any) (rjobs.Job, error) {
	// Decode config
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// Declare logger
	l := appctx.GetLogger(ctx)

	return &job{conf: &c, log: l}, nil
}

/* Job setup */

// The takeout job structure
type job struct {
	conf *config
	log  *zerolog.Logger
}

// The per-run takeout parameters
type params struct {
	MaxArchiveSize int64  `mapstructure:"maxArchiveSize"` // < 10GB
	ArchiveFormat  string `mapstructure:"archiveFormat"`  // One of ["zip", "tar"]
	Username       string `mapstructure:"username" validate:"required"`
}

// Run walks the user's home space, downloads its content as the user,
// archives it, uploads the archives to the takeout space as the takeout
// admin, and returns a public link to the folder containing the archives
func (j *job) Run(ctx context.Context, p rjobs.Params) (rjobs.Params, error) {
	// Decode run parameters
	pp := params{
		// Default values in case they're not provided
		MaxArchiveSize: 2 << 30, // 2 GiB
		ArchiveFormat:  "zip",
	}
	if err := mapstructure.Decode(map[string]any(p), &pp); err != nil {
		return nil, errors.Wrap(err, "takeout: decoding params failed")
	}
	if pp.MaxArchiveSize > 10<<30 {
		return nil, errors.Errorf("takeout: MaxArchiveSize cannot be larger than 10GB")
	}
	j.log.Info().Msgf("takeout: using parameters %+v", pp)

	// Setup gateway
	gtw, err := pool.GetGatewayServiceClient(pool.Endpoint("localhost:9142"))
	if err != nil {
		return nil, err
	}

	// Setup authentification: user context to walk and download, admin context to upload
	userCtx, err := j.authenticate(ctx, gtw, pp.Username)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: user authentication failed")
	}
	adminCtx, err := j.authenticate(ctx, gtw, j.conf.TakeoutAdminUsername)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: admin authentication failed")
	}

	// Setup downloader
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	hc := httpclient.New(httpclient.RoundTripper(tr), httpclient.Timeout(time.Duration(10*time.Minute)))
	dl := downloader.NewDownloader(gtw, hc)

	// Setup upload client: no timeout, as an archive upload stays open for
	// the whole duration of its part's creation
	upHC := httpclient.New(httpclient.RoundTripper(tr), httpclient.Timeout(0))

	// Setup walker
	wk := walker.NewWalker(gtw)

	// Set the source & destination directories
	root := "/eos/user/" + pp.Username[0:1] + "/" + pp.Username
	archPath := fmt.Sprintf("/eos/project/t/takeout/%s_%s/", pp.Username, time.Now().Format("2006-01-02"))

	// Create archives depending on requested archive format
	switch pp.ArchiveFormat {
	case "zip":
		err = j.createZipArchives(userCtx, adminCtx, root, archPath, wk, dl, gtw, upHC, pp.MaxArchiveSize)
		if err != nil {
			return nil, errors.Wrap(err, "takeout: zip archive could not be created")
		}
	case "tar":
		err = j.createTarArchives(userCtx, adminCtx, root, archPath, wk, dl, pp.MaxArchiveSize)
		if err != nil {
			return nil, errors.Wrap(err, "takeout: tar archive could not be created")
		}
	default:
		return nil, errors.Errorf("takeout: %s is not a supported archive format", pp.ArchiveFormat)
	}

	// Share the folder containing the archives through a public link
	token, err := j.createPublicShare(adminCtx, gtw, archPath)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: public share could not be created")
	}

	// Return the public link to the archives and their location
	url := fmt.Sprintf("%s/s/%s", j.conf.PublicURL, token)
	return rjobs.Params{"archives_url": url, "archives_path": archPath}, nil
}

// authenticate performs a machine authentication as the given user and returns an appropriate context
func (j *job) authenticate(ctx context.Context, gtw gateway.GatewayAPIClient, clientID string) (context.Context, error) {
	authRes, err := gtw.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     clientID,
		ClientSecret: j.conf.MachineSecret,
	})
	if err != nil {
		return nil, errors.Wrap(err, "takeout: authentication failed")
	}
	if authRes.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(errors.New(authRes.Status.String()), "takeout: auth res status code not OK")
	}

	// Update authenticated context
	ctx = appctx.ContextSetToken(ctx, authRes.Token)
	ctx = appctx.ContextSetUser(ctx, authRes.User)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)
	return ctx, nil
}

func (j *job) createTarArchives(userCtx, adminCtx context.Context, root_path, arch_path string, wk walker.Walker, dl downloader.Downloader, maxArchiveSize int64) error {
	panic("unimplemented")
}

func (j *job) createZipArchives(userCtx, adminCtx context.Context, root_path, arch_path string, wk walker.Walker, dl downloader.Downloader, gtw gateway.GatewayAPIClient, hc *httpclient.Client, maxArchiveSize int64) error {
	// Ensure the destination directory exists before any upload starts
	mkRes, err := gtw.CreateContainer(adminCtx, &provider.CreateContainerRequest{
		Ref: &provider.Reference{Path: arch_path},
	})
	switch {
	case err != nil:
		return err
	case mkRes.Status.Code != rpc.Code_CODE_OK && mkRes.Status.Code != rpc.Code_CODE_ALREADY_EXISTS:
		return errtypes.InternalError(mkRes.Status.Message)
	}

	// Setup zip archive streaming state
	var (
		pw        *io.PipeWriter
		done      chan error
		cw        *countingWriter
		w         *zip.Writer
		archIndex = 0
	)

	// Start a fresh archive by initiating its upload and streaming the zip into it
	startPart := func() {
		pr, npw := io.Pipe()
		pw = npw
		done = make(chan error, 1)
		go func(idx int) {
			err := j.uploadArchive(adminCtx, gtw, hc, arch_path, idx, pr)
			// Unblock the producer if the upload fails mid-stream
			pr.CloseWithError(err)
			done <- err
		}(archIndex)
		cw = &countingWriter{w: pw}
		w = zip.NewWriter(cw)
	}

	// Finalize the current archive and wait for its upload to complete
	flush := func() error {
		zipErr := w.Close()
		if zipErr != nil {
			pw.CloseWithError(zipErr)
		} else {
			zipErr = pw.Close()
		}
		upErr := <-done
		pw = nil
		if zipErr != nil {
			return zipErr
		}
		return upErr
	}

	// Start the first archive
	startPart()

	// Create the archives by walking the specified directory
	err = wk.Walk(userCtx, root_path, func(current_path string, info *provider.ResourceInfo, err error) error {
		if err != nil {
			return err
		}

		isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

		// Get relative path of the current file
		fileName, err := filepath.Rel(filepath.Dir(root_path), current_path)
		if err != nil {
			return err
		}

		// Cut the archive if adding the current file could exceed maxArchiveSize
		if !isDir && cw.n > 0 && cw.n+int64(info.Size) > maxArchiveSize {
			if err := flush(); err != nil {
				return err
			}
			archIndex++
			startPart()
		}

		// Create zip header of the current file
		header := zip.FileHeader{
			Name:     fileName,
			Modified: time.Unix(int64(info.Mtime.Seconds), 0),
			Method:   zip.Deflate,
		}
		if isDir {
			header.Name += "/"
		}
		zip_file, err := w.CreateHeader(&header)
		if err != nil {
			return err
		}

		if isDir {
			return nil
		}

		// Download file content
		dl_file, err := dl.Download(userCtx, current_path, "")
		if err != nil {
			return err
		}
		defer dl_file.Close()

		// Copies downloaded file to zip archive
		if _, err := io.Copy(zip_file, dl_file); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		// Abort the in-flight upload, if any
		if pw != nil {
			pw.CloseWithError(err)
			<-done
		}
		return err
	}

	// Finalize last archive
	if err := flush(); err != nil {
		j.log.Err(err).Msg("takeout: archive upload failed")
		return err
	}

	return nil
}

func (j *job) uploadArchive(ctx context.Context, gtw gateway.GatewayAPIClient, hc *httpclient.Client, archPath string, archIndex int, arch io.Reader) error {
	// Setup archive name
	var (
		archName = fmt.Sprintf("takeout-%03d.zip", archIndex)
		archFile = archPath + archName
	)

	// Initiate the file upload request
	req := &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: archFile},
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"Upload-Length": {
					Decoder: "plain",
					Value:   []byte("-1"),
				},
			},
		},
	}
	upRes, err := gtw.InitiateFileUpload(ctx, req)
	switch {
	case err != nil:
		return err
	case upRes.Status.Code != rpc.Code_CODE_OK:
		return errtypes.InternalError(upRes.Status.Message)
	}

	// Get upload protocol
	p, err := getUploadProtocol(upRes.Protocols, "simple")
	if err != nil {
		return err
	}

	// Create the upload request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, p.UploadEndpoint, arch)
	if err != nil {
		return err
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)
	httpReq.Header.Set("Upload-Length", "-1")

	httpRes, err := hc.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		switch httpRes.StatusCode {
		case http.StatusNotFound:
			return errtypes.NotFound(archFile)
		default:
			return errtypes.InternalError(httpRes.Status)
		}
	}

	j.log.Debug().Msgf("takeout: uploaded archive %s to %s", archName, archPath)
	return nil
}

// createPublicShare creates a read-only public link to the given path
func (j *job) createPublicShare(ctx context.Context, gtw gateway.GatewayAPIClient, path string) (string, error) {
	// Get the resource info of the folder to share
	statRes, err := gtw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{Path: path},
	})
	switch {
	case err != nil:
		return "", err
	case statRes.Status.Code != rpc.Code_CODE_OK:
		return "", errtypes.InternalError(statRes.Status.Message)
	}

	// Create the read-only public link
	shareRes, err := gtw.CreatePublicShare(ctx, &link.CreatePublicShareRequest{
		ResourceInfo: statRes.Info,
		Grant: &link.Grant{
			Permissions: &link.PublicSharePermissions{
				Permissions: &provider.ResourcePermissions{
					GetPath:              true,
					InitiateFileDownload: true,
					ListContainer:        true,
					Stat:                 true,
				},
			},
		},
	})
	switch {
	case err != nil:
		return "", err
	case shareRes.Status.Code != rpc.Code_CODE_OK:
		return "", errtypes.InternalError(shareRes.Status.Message)
	}

	j.log.Debug().Msgf("takeout: created public share %s to %s", shareRes.Share.Token, path)
	return shareRes.Share.Token, nil
}

func getUploadProtocol(protocols []*gateway.FileUploadProtocol, prot string) (*gateway.FileUploadProtocol, error) {
	for _, p := range protocols {
		if p.Protocol == prot {
			return p, nil
		}
	}
	return nil, errtypes.InternalError(fmt.Sprintf("protocol %s not supported for uploading", prot))
}

// countingWriter counts the bytes written through it to measure the actual archive size
type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}
