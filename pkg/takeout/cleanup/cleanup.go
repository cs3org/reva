package cleanup

import (
	"context"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
)

/* Job registration */

// Clteanup job name
const CleanupJobName = "takeout.cleanup"

// RegisterCleanup registers the periodic cleanup job
func RegisterCleanup(ctx context.Context, c *Config, l *zerolog.Logger) {
	l.Info().Msg("cleanup: registered periodic job")

	cj := &CleanupJob{
		conf: &Config{
			MachineSecret:        c.MachineSecret,
			TakeoutAdminUsername: c.TakeoutAdminUsername,
			TakeoutPath:          c.TakeoutPath,
			CleanupSchedule:      c.CleanupSchedule,
			CleanupDelay:         c.CleanupDelay,
		},
		log: l,
	}

	if err := rjobs.RegisterPeriodic(rjobs.Periodic{
		Name:       CleanupJobName,
		Schedule:   cj.conf.CleanupSchedule,
		Scope:      rjobs.ScopeLeader,
		RunOnStart: true,
		Run:        cj.Run,
	}); err != nil {
		panic(err)
	}

}

type CleanupJob struct {
	conf *Config
	log  *zerolog.Logger
}

/* Job's configuration setup */

type Config struct {
	MachineSecret        string
	TakeoutAdminUsername string
	TakeoutPath          string
	// The frequency at which the cleanup job is run
	CleanupSchedule string
	// The minimal delay in hours before a directory is removed by the cleanup job
	CleanupDelay int64
}

func (cj *CleanupJob) Run(ctx context.Context) error {
	appctx.GetLogger(ctx).Info().Msg("running cleanup job")

	// Setup gateway
	gtw, err := pool.GetGatewayServiceClient(pool.Endpoint("localhost:9142"))
	if err != nil {
		return err
	}

	// Authenticate
	authRes, err := gtw.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     cj.conf.TakeoutAdminUsername,
		ClientSecret: cj.conf.MachineSecret,
	})
	if err != nil {
		return errors.Wrap(err, "takeout: authentication failed")
	}
	if authRes.Status.Code != rpc.Code_CODE_OK {
		return errors.Wrap(errors.New(authRes.Status.Message), "takeout: authentication failed")
	}

	// Update authenticated context
	ctx = appctx.ContextSetToken(ctx, authRes.Token)
	ctx = appctx.ContextSetUser(ctx, authRes.User)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)

	// Get container list
	containerRes, err := gtw.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			Path: cj.conf.TakeoutPath,
		},
	})
	if err != nil {
		return errors.Wrap(err, "cleanup: could not get takeout containers")
	}
	if containerRes.Status.Code != rpc.Code_CODE_OK {
		return errors.Wrap(errors.New(containerRes.Status.Message), "cleanup: could not get takeout containers")
	}
	appctx.GetLogger(ctx).Debug().Msgf("cleanup: found %d takeout containers", len(containerRes.Infos))

	// Compute time threshold
	threshold := time.Duration(cj.conf.CleanupDelay) * time.Hour

	for _, info := range containerRes.Infos {
		// Compute time since takeout
		timeSinceTakeout := time.Since(time.Unix(int64(info.Mtime.Seconds), 0))

		if timeSinceTakeout > threshold {
			appctx.GetLogger(ctx).Debug().Msgf("cleanup: removing %s [%.1fh]", info.Path, timeSinceTakeout.Hours())

			// Delete container and content recursively
			delRes, err := gtw.Delete(ctx, &provider.DeleteRequest{
				Ref: &provider.Reference{
					ResourceId: info.Id,
				},
			})
			if err != nil {
				return errors.Wrap(err, "cleanup: could not delete container")
			}
			if delRes.Status.Code != rpc.Code_CODE_OK {
				return errors.Wrap(errors.New(delRes.Status.Message), "cleanup: could not delete container")
			}
		}
	}

	return nil
}
