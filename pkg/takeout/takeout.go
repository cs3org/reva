package takeout

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/dustin/go-humanize"
	"google.golang.org/grpc/metadata"

	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/bundler"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
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
	Insecure             bool   `mapstructure:"insecure"`
	TakeoutAdminUsername string `mapstructure:"takeout_admin_username" validate:"required"`
	TakeoutPath          string `mapstructure:"takeout_path" validate:"required"`
	PublicURL            string `mapstructure:"public_url" validate:"required"`
	ArchiveSizeCeiling   uint64 `mapstructure:"archive_size_ceiling" validate:"required"`
	// Name of the manifest listing the entries left out of the archives, the archives
	// are listed by name so a name sorting before "takeout" shows it to the user first
	SkippedFileName string `mapstructure:"skipped_file_name" validate:"required"`
	// The delay in hours before the archives expire, it sets the public link
	// expiration and the date announced to the user, so it should not outlive the
	// cleanup_delay of the takeout http service
	ExpirationDelay int64 `mapstructure:"expiration_delay" validate:"required"`
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
	MaxArchiveSize uint64         `mapstructure:"max_archive_size"`
	ArchiveFormat  bundler.Format `mapstructure:"archive_format"` // One of ["zip", "tgz"]
	Username       string         `mapstructure:"username" validate:"required"`
	// Path to root directory to take out
	RootPath string `mapstructure:"root_path" validate:"required"`
}

// A created archive and its direct download link
type archive struct {
	Name string `json:"name"`
	Size string `json:"size"`
	URL  string `json:"url"`
}

// Run archives the user's userspace and shares it through a public link
func (j *job) Run(ctx context.Context, p rjobs.Params) (res rjobs.Params, err error) {
	// Decode run parameters
	var pp params
	if err := mapstructure.Decode(map[string]any(p), &pp); err != nil {
		return nil, errors.Wrap(err, "takeout: decoding params failed")
	}
	// Apply the default parameters
	if pp.ArchiveFormat == "" {
		pp.ArchiveFormat = bundler.FormatZip
	}
	if pp.MaxArchiveSize == 0 {
		pp.MaxArchiveSize = j.conf.ArchiveSizeCeiling / 2
	}

	// Validate the parameters
	if err := j.validateParams(pp); err != nil {
		j.log.Err(err).Msg("takeout: invalid parameters")
		return nil, err
	}
	j.log.Info().Msgf("takeout: using parameters %+v", pp)

	// Setup authentication: user context to walk and download, admin context to upload
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

	// Drop the destination directory when the run does not complete, on a failure as on a cancellation
	defer func() {
		if err != nil {
			j.cleanupTakeoutContainer(adminCtx, archPath)
		}
	}()

	// Create the archives, dropping the entries that cannot be read rather than failing the whole takeout
	var skipped []string
	opts := j.bundleOpts(pp)
	opts.OnEntryError = func(path string, err error) error {
		j.log.Warn().Err(err).Msgf("takeout: skipping unreadable entry %s", path)
		skipped = append(skipped, path)
		return nil
	}
	err = j.bundler.Create(userCtx, opts, j.newPartFunc(adminCtx, archPath, string(pp.ArchiveFormat)))
	if err != nil {
		return nil, errors.Wrapf(err, "takeout: %s archive could not be created", pp.ArchiveFormat)
	}

	// Tell the user what is missing from their takeout
	if len(skipped) > 0 {
		j.uploadSkipped(adminCtx, archPath, skipped)
	}

	// Share the folder containing the archives through a public link
	expiration := time.Now().Add(time.Duration(j.conf.ExpirationDelay) * time.Hour)
	token, err := j.createPublicShare(adminCtx, archPath, expiration)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: public share could not be created")
	}

	// List the created archives with their download links
	archives, err := j.listArchives(adminCtx, archPath, token)
	if err != nil {
		return nil, errors.Wrap(err, "takeout: archives could not be listed")
	}

	// Notify the user that their takeout is ready
	j.notifyCompletion(userCtx, archives, expiration)

	// Return the public link to the archives and their location
	res = rjobs.Params{"archives_tok": token, "archives_path": archPath}
	if len(skipped) > 0 {
		res["skipped_count"] = len(skipped)
	}
	return res, nil
}

// validateParams ensures the run parameters are valid
func (j *job) validateParams(pp params) error {
	if pp.MaxArchiveSize == 0 {
		return errors.Errorf("MaxArchiveSize cannot be null")
	}
	if pp.MaxArchiveSize > j.conf.ArchiveSizeCeiling {
		return errors.Errorf("MaxArchiveSize cannot be larger than %d", j.conf.ArchiveSizeCeiling)
	}
	switch pp.ArchiveFormat {
	case bundler.FormatZip, bundler.FormatTgz:
	default:
		return bundler.ErrUnsupportedFormat{Format: pp.ArchiveFormat}
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
func (j *job) bundleOpts(pp params) bundler.Options {
	opts := bundler.Options{
		Roots:        []string{pp.RootPath},
		Format:       pp.ArchiveFormat,
		MaxNumFiles:  bundler.Unlimited,
		MaxTotalSize: bundler.Unlimited,
		MaxPartSize:  pp.MaxArchiveSize,
	}
	// Compress the zip entries, the tgz stream is compressed as a whole
	if pp.ArchiveFormat == bundler.FormatZip {
		opts.ZipMethod = zip.Deflate
	}
	return opts
}

// listArchives lists the created archives, a public link download of a single file serves it directly
func (j *job) listArchives(adminCtx context.Context, archPath, token string) ([]archive, error) {
	lsRes, err := j.gtw.ListContainer(adminCtx, &provider.ListContainerRequest{
		Ref: &provider.Reference{Path: archPath},
	})
	switch {
	case err != nil:
		return nil, err
	case lsRes.Status.GetCode() != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(lsRes.Status.Message)
	}

	// Order the entries by name, the manifest of the skipped entries comes first and the parts follow
	slices.SortFunc(lsRes.Infos, func(a, b *provider.ResourceInfo) int {
		return strings.Compare(a.Name, b.Name)
	})

	archives := make([]archive, len(lsRes.Infos))
	for i, info := range lsRes.Infos {
		archives[i] = archive{
			Name: info.Name,
			Size: humanize.Bytes(info.Size),
			URL:  fmt.Sprintf("%s/remote.php/dav/public-files/%s/%s", j.conf.PublicURL, token, url.PathEscape(info.Name)),
		}
	}
	return archives, nil
}

// notifyCompletion emails the user the archives of their takeout
func (j *job) notifyCompletion(userCtx context.Context, archives []archive, expiration time.Time) {
	// Get the email from the authenticated user
	user, ok := appctx.ContextGetUser(userCtx)
	if !ok || user == nil || user.Mail == "" {
		j.log.Error().Msg("takeout: no user email available for the completion notification")
		return
	}

	// PublishEvent is restricted to reva daemons
	publishCtx, err := scope.ContextWithMachineScope(userCtx)
	if err != nil {
		j.log.Err(err).Msg("takeout: failed to elevate context for the completion notification")
		return
	}

	// Pass the archives and the expiration to the email template
	templateData := map[string]any{
		"user_display_name": user.DisplayName,
		"username":          user.Username,
		"archives":          archives,
		"expiration_date":   expiration.Format("Monday, 2 January 2006 at 15:04 MST"),
	}

	// Publish the completion event
	event := notifications.EncodeEvent(model.EventTakeout, []string{user.Mail}, templateData)
	res, err := j.gtw.PublishEvent(publishCtx, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		j.log.Err(err).Msg("takeout: failed to send the completion notification event")
		return
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		j.log.Error().Msg("takeout: gateway rejected the completion notification event")
	}
	j.log.Debug().Msgf("takeout: sent completion notification to %s", user.Mail)
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
	if authRes.Status.GetCode() != rpc.Code_CODE_OK {
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
	case delRes.Status.GetCode() != rpc.Code_CODE_OK && delRes.Status.GetCode() != rpc.Code_CODE_NOT_FOUND:
		return errtypes.InternalError(delRes.Status.Message)
	}

	// Creates the empty destination directory
	mkRes, err := j.gtw.CreateContainer(adminCtx, &provider.CreateContainerRequest{
		Ref: &provider.Reference{Path: archPath},
	})
	switch {
	case err != nil:
		return err
	case mkRes.Status.GetCode() != rpc.Code_CODE_OK:
		return errtypes.InternalError(mkRes.Status.Message)
	}
	return nil
}

// cleanupTakeoutContainer removes a partial takeout container
func (j *job) cleanupTakeoutContainer(adminCtx context.Context, archPath string) {
	delRes, err := j.gtw.Delete(context.WithoutCancel(adminCtx), &provider.DeleteRequest{
		Ref: &provider.Reference{Path: archPath},
	})
	switch {
	case err != nil:
		j.log.Err(err).Msgf("takeout: partial takeout %s could not be removed", archPath)
	case delRes.Status.GetCode() != rpc.Code_CODE_OK && delRes.Status.GetCode() != rpc.Code_CODE_NOT_FOUND:
		j.log.Error().Msgf("takeout: partial takeout %s could not be removed: %s", archPath, delRes.Status.Message)
	default:
		j.log.Debug().Msgf("takeout: removed partial takeout %s", archPath)
	}
}

// uploadArchive streams one archive part to its final destination
func (j *job) uploadArchive(adminCtx context.Context, archPath string, archIndex int, ext string, arch io.Reader) error {
	archName := fmt.Sprintf("takeout-%03d.%s", archIndex, ext)
	if err := j.upload(adminCtx, archPath+archName, arch); err != nil {
		return err
	}

	j.log.Debug().Msgf("takeout: uploaded archive %s to %s", archName, archPath)
	return nil
}

// uploadSkipped uploads the manifest of the entries left out of the archives, a takeout
// missing a few files is still worth delivering so a failure here is only logged
func (j *job) uploadSkipped(adminCtx context.Context, archPath string, skipped []string) {
	var manifest strings.Builder
	fmt.Fprintf(&manifest, "The following %d entries could not be read and are not included in the archives:\n\n", len(skipped))
	for _, path := range skipped {
		fmt.Fprintln(&manifest, path)
	}

	if err := j.upload(adminCtx, archPath+j.conf.SkippedFileName, strings.NewReader(manifest.String())); err != nil {
		j.log.Err(err).Msgf("takeout: %s could not be uploaded to %s", j.conf.SkippedFileName, archPath)
		return
	}

	j.log.Debug().Msgf("takeout: uploaded %s listing %d entries to %s", j.conf.SkippedFileName, len(skipped), archPath)
}

// upload streams the given content to the given destination path
func (j *job) upload(adminCtx context.Context, dst string, content io.Reader) error {
	// Initiate the file upload request
	upRes, err := j.gtw.InitiateFileUpload(adminCtx, &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{Path: dst},
	})
	switch {
	case err != nil:
		return err
	case upRes.Status.GetCode() != rpc.Code_CODE_OK:
		return errtypes.InternalError(upRes.Status.Message)
	}

	// Get upload protocol
	p, err := getUploadProtocol(upRes.Protocols, "simple")
	if err != nil {
		return err
	}

	// Create the upload request
	httpReq, err := http.NewRequestWithContext(adminCtx, http.MethodPut, p.UploadEndpoint, content)
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
			return errtypes.NotFound(dst)
		default:
			return errtypes.InternalError(httpRes.Status)
		}
	}
	return nil
}

// createPublicShare creates a read-only public link to the given path
func (j *job) createPublicShare(adminCtx context.Context, path string, expiration time.Time) (string, error) {
	// Get the resource info of the folder to share
	statRes, err := j.gtw.Stat(adminCtx, &provider.StatRequest{
		Ref: &provider.Reference{Path: path},
	})
	switch {
	case err != nil:
		return "", err
	case statRes.Status.GetCode() != rpc.Code_CODE_OK:
		return "", errors.New(statRes.Status.Message)
	}

	// Create the read-only public link, without password as the download route only serves such links
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
			Expiration: &types.Timestamp{Seconds: uint64(expiration.Unix())},
		},
	})
	switch {
	case err != nil:
		return "", err
	case shareRes.Status.GetCode() != rpc.Code_CODE_OK:
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
	return nil, errtypes.InternalError(fmt.Sprintf("takeout: protocol %s not supported for uploading", prot))
}
