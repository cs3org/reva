package takeout

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/sethvargo/go-password/password"
	"google.golang.org/grpc/metadata"

	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/bundler"
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
	GatewaySvc           string `mapstructure:"gatewaysvc" validate:"required"`
	Insecure             bool   `mapstructure:"insecure" validate:"required"`
	TakeoutAdminUsername string `mapstructure:"takeout_admin_username" validate:"required"`
	TakeoutPath          string `mapstructure:"takeout_path" validate:"required"`
	PublicURL            string `mapstructure:"public_url" validate:"required"`
	ArchiveSizeCeiling   uint64 `mapstructure:"archive_size_ceiling" validate:"required"`
	PasswordStrength     int    `mapstructure:"password_strength" validate:"required"`
}

/* Job setup */

// The takeout job structure
type job struct {
	conf    *config
	log     *zerolog.Logger
	gtw     gateway.GatewayAPIClient
	hc      *httpclient.Client
	bundler *bundler.Bundler
}

// New sets the potential custom job config and builds the job's deps
func New(ctx context.Context, m map[string]any) (rjobs.Job, error) {
	// Decode config
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// Declare logger
	l := appctx.GetLogger(ctx)

	// Setup gateway
	gtw, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "takeout: gateway client setup failed")
	}

	// Setup the http client for downloads and uploads
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure}}
	hc := httpclient.New(httpclient.RoundTripper(tr))

	// Setup the bundler with a walker and a downloader
	b := bundler.New(walker.NewWalker(gtw), downloader.NewDownloader(gtw, hc))

	return &job{conf: &c, log: l, gtw: gtw, hc: hc, bundler: b}, nil
}

// The per-run takeout parameters
type params struct {
	MaxArchiveSize uint64 `mapstructure:"max_archive_size"`
	ArchiveFormat  string `mapstructure:"archive_format"` // One of ["zip", "tgz"]
	Username       string `mapstructure:"username" validate:"required"`
	// Path to root directory to take out
	RootPath string `mapstructure:"root_path" validate:"required"`
}

// Run archives the user's userspace and shares it through a public link
func (j *job) Run(ctx context.Context, p rjobs.Params) (rjobs.Params, error) {
	// Decode run parameters
	var pp params
	if err := mapstructure.Decode(map[string]any(p), &pp); err != nil {
		return nil, errors.Wrap(err, "takeout: decoding params failed")
	}
	if err := j.validateParams(pp); err != nil {
		j.log.Err(err).Msg("takeout: invalid parameters")
		return nil, err
	}
	j.log.Info().Msgf("takeout: using parameters %+v", pp)

	// Setup authentification: user context to walk and download, admin context to upload
	userCtx, err := j.authenticate(ctx, pp.Username)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: user authentication failed")
	}
	adminCtx, err := j.authenticate(ctx, j.conf.TakeoutAdminUsername)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: admin authentication failed")
	}

	// Set the destination directory and create it
	archPath := fmt.Sprintf("%s/%s_%s/", j.conf.TakeoutPath, pp.Username, time.Now().Format("2006-01-02"))
	if err := j.createTakeoutContainer(adminCtx, archPath); err != nil {
		return nil, errors.Wrap(err, "takeout: destination directory could not be created")
	}

	// Create the archives
	opts, err := j.bundleOpts(pp)
	if err != nil {
		return nil, err
	}
	err = j.bundler.Create(userCtx, opts, j.newPartFunc(adminCtx, archPath, pp.ArchiveFormat))
	if err != nil {
		return nil, errors.Wrapf(err, "takeout: %s archive could not be created", pp.ArchiveFormat)
	}

	// Share the folder containing the archives through a public link
	token, pwd, err := j.createPublicShare(adminCtx, archPath)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: public share could not be created")
	}

	// Return the public link to the archives and their location
	url := fmt.Sprintf("%s/s/%s", j.conf.PublicURL, token)
	return rjobs.Params{"archives_url": url, "archives_pwd": pwd, "archives_path": archPath}, nil
}

// validateParams ensures the run parameters are valid
func (j *job) validateParams(pp params) error {
	if pp.MaxArchiveSize == 0 {
		return errors.Errorf("MaxArchiveSize cannot be null")
	}
	if pp.MaxArchiveSize > j.conf.ArchiveSizeCeiling {
		return errors.Errorf("MaxArchiveSize cannot be larger than %d", j.conf.ArchiveSizeCeiling)
	}
	if pp.ArchiveFormat == "" {
		return errors.Errorf("ArchiveFormat must be specified")
	}
	if pp.Username == "" {
		return errors.Errorf("Username must be specified")
	}
	if pp.RootPath == "" {
		return errors.Errorf("RootPath must be specified")
	}
	return nil
}

// bundleOpts maps the run parameters to the bundler options
func (j *job) bundleOpts(pp params) (bundler.Options, error) {
	opts := bundler.Options{
		Roots:        []string{pp.RootPath},
		MaxNumFiles:  bundler.Unlimited,
		MaxTotalSize: bundler.Unlimited,
		MaxPartSize:  pp.MaxArchiveSize,
		Errors:       bundler.ErrorFailFast,
	}
	switch pp.ArchiveFormat {
	case "zip":
		opts.Format = bundler.FormatZip
		opts.ZipMethod = zip.Deflate
	case "tgz":
		opts.Format = bundler.FormatTgz
	default:
		return bundler.Options{}, errors.Errorf("takeout: %s is not a supported archive format", pp.ArchiveFormat)
	}
	return opts, nil
}

// uploadPart streams one archive part into an upload running concurrently
type uploadPart struct {
	pw   *io.PipeWriter
	done chan error
}

func (u *uploadPart) Write(p []byte) (int, error) {
	return u.pw.Write(p)
}

func (u *uploadPart) Close() error {
	_ = u.pw.Close()
	return <-u.done
}

func (u *uploadPart) Abort(err error) error {
	_ = u.pw.CloseWithError(err)
	return <-u.done
}

// newPartFunc returns the part factory used by the bundler
func (j *job) newPartFunc(adminCtx context.Context, archPath, ext string) bundler.PartFunc {
	return func(index int) (bundler.Part, error) {
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() {
			err := j.uploadArchive(adminCtx, archPath, index, ext, pr)
			// Unblock the producer if the upload fails mid-stream
			pr.CloseWithError(err)
			done <- err
		}()
		return &uploadPart{pw: pw, done: done}, nil
	}
}

// authenticate performs a machine authentication as the given user and returns an appropriate context
func (j *job) authenticate(ctx context.Context, clientID string) (context.Context, error) {
	authRes, err := j.gtw.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     clientID,
		ClientSecret: j.conf.MachineSecret,
	})
	if err != nil {
		return nil, errors.Wrap(err, "takeout: authentication failed")
	}
	if authRes.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(errors.New(authRes.Status.String()), "takeout: authentication failed")
	}

	// Update authenticated context
	ctx = appctx.ContextSetToken(ctx, authRes.Token)
	ctx = appctx.ContextSetUser(ctx, authRes.User)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)
	return ctx, nil
}

func (j *job) createTakeoutContainer(adminCtx context.Context, archPath string) error {
	// Delete any pre-existing takeout for this user on that day to avoid conflicts
	delRes, err := j.gtw.Delete(adminCtx, &provider.DeleteRequest{
		Ref: &provider.Reference{Path: archPath},
	})
	switch {
	case err != nil:
		return err
	case delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND:
		return errtypes.InternalError(delRes.Status.Message)
	}

	// Creates the empty destination directory
	mkRes, err := j.gtw.CreateContainer(adminCtx, &provider.CreateContainerRequest{
		Ref: &provider.Reference{Path: archPath},
	})
	switch {
	case err != nil:
		return err
	case mkRes.Status.Code != rpc.Code_CODE_OK:
		return errtypes.InternalError(mkRes.Status.Message)
	}
	return nil
}

func (j *job) uploadArchive(adminCtx context.Context, archPath string, archIndex int, ext string, arch io.Reader) error {
	// Setup archive name
	var (
		archName = fmt.Sprintf("takeout-%03d.%s", archIndex, ext)
		archFile = archPath + archName
	)

	// Initiate the file upload request
	upRes, err := j.gtw.InitiateFileUpload(adminCtx, &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: archFile},
	})
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
	httpReq, err := http.NewRequestWithContext(adminCtx, http.MethodPut, p.UploadEndpoint, arch)
	if err != nil {
		return err
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, p.Token)
	httpReq.Header.Set("Upload-Length", "-1")

	httpRes, err := j.hc.Do(httpReq)
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
func (j *job) createPublicShare(adminCtx context.Context, path string) (string, string, error) {
	// Get the resource info of the folder to share
	statRes, err := j.gtw.Stat(adminCtx, &provider.StatRequest{
		Ref: &provider.Reference{Path: path},
	})
	switch {
	case err != nil:
		return "", "", err
	case statRes.Status.Code != rpc.Code_CODE_OK:
		return "", "", errors.New(statRes.Status.Message)
	}

	// Generate password
	pwd, err := password.Generate(j.conf.PasswordStrength, j.conf.PasswordStrength/2, 0, false, false)
	if err != nil {
		return "", "", errors.Wrap(err, "takeout: could not generate password")
	}
	j.log.Debug().Msgf("takeout: password %s", pwd)

	// Create the read-only public link
	shareRes, err := j.gtw.CreatePublicShare(adminCtx, &link.CreatePublicShareRequest{
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
			Password:   pwd,
			Expiration: &types.Timestamp{Seconds: uint64(time.Now().Add(168 * time.Hour).Unix())},
		},
	})
	switch {
	case err != nil:
		return "", "", err
	case shareRes.Status.Code != rpc.Code_CODE_OK:
		return "", "", errtypes.InternalError(shareRes.Status.Message)
	}

	j.log.Debug().Msgf("takeout: created public share %s to %s", shareRes.Share.Token, path)
	return shareRes.Share.Token, pwd, nil
}

func getUploadProtocol(protocols []*gateway.FileUploadProtocol, prot string) (*gateway.FileUploadProtocol, error) {
	for _, p := range protocols {
		if p.Protocol == prot {
			return p, nil
		}
	}
	return nil, errtypes.InternalError(fmt.Sprintf("takeout: protocol %s not supported for uploading", prot))
}
