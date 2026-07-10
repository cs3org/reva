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
		return errors.Wrap(errors.New(authRes.Status.String()), "takeout: auth res status code not OK")
	}

	// Update authenticated context
	ctx = appctx.ContextSetToken(ctx, authRes.Token)
	ctx = appctx.ContextSetUser(ctx, authRes.User)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)

	// Get archive list
	listRes, err := gtw.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			Path: cj.conf.TakeoutPath,
		},
	})
	if err != nil {
		return err
	}

	appctx.GetLogger(ctx).Debug().Msgf("cleanup: found %d takeouts", len(listRes.Infos))

	// Compute time threshold
	threshold := time.Duration(cj.conf.CleanupDelay) * time.Hour
	for _, info := range listRes.Infos {
		// Compute time since takeout
		timeSinceTakeout := time.Since(time.Unix(int64(info.Mtime.Seconds), 0))

		if timeSinceTakeout > threshold {
			appctx.GetLogger(ctx).Debug().Msgf("cleanup: removing %s [%.1f]", info.Path, timeSinceTakeout.Hours())

		} else {
			appctx.GetLogger(ctx).Debug().Msgf("cleanup: keeping %s [%.1f]", info.Path, timeSinceTakeout.Hours())
		}
	}

	return nil
}
