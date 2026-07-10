package takeout

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/takeout"
	"github.com/cs3org/reva/v3/pkg/takeout/cleanup"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/rs/zerolog"
)

/* Service registration */

// Init registers the takeout http service
func init() {
	global.Register("takeout", New)
}

/* Service's configuration setup */

// The takeout service Config
type Config struct {
	Prefix               string `mapstructure:"prefix"`
	MachineSecret        string `mapstructure:"machine_secret" validate:"required"`
	TakeoutAdminUsername string `mapstructure:"takeout_admin_username" validate:"required"`
	TakeoutPath          string `mapstructure:"takeout_path" validate:"required"`
	CleanupSchedule      string `mapstructure:"cleanup_schedule"`
	CleanupDelay         int64  `mapstructure:"cleanup_delay"` // In hours
}

// New sets the potential custom service config
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	// Decode config
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// Declare logger
	l := appctx.GetLogger(ctx)

	// Register periodic cleanup job
	cleanupConfig := &cleanup.Config{
		MachineSecret:        c.MachineSecret,
		TakeoutAdminUsername: c.TakeoutAdminUsername,
		TakeoutPath:          c.TakeoutPath,
		CleanupSchedule:      c.CleanupSchedule,
		CleanupDelay:         c.CleanupDelay,
	}
	cleanup.RegisterCleanup(ctx, cleanupConfig, l)

	return &svc{conf: &c, log: l}, nil
}

// ApplyDefaults sets the default service config
func (c *Config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "takeout"
	}
	if c.CleanupSchedule == "" {
		c.CleanupSchedule = "@daily"
	}
	if c.CleanupDelay == 0 {
		c.CleanupDelay = 168 // One week
	}
}

// The GET JSON reply structure
type statusReply struct {
	RunID        string     `json:"run_id"`
	State        string     `json:"state"`
	EnqueuedAt   time.Time  `json:"enqueued_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Error        string     `json:"error,omitempty"`
	ArchivesURL  string     `json:"archives_url,omitempty"`
	ArchivesPath string     `json:"archives_path,omitempty"`
}

/* Service setup */

// The takeout service structure
type svc struct {
	conf *Config
	log  *zerolog.Logger
}

// Close performs a clean up
func (s *svc) Close() error {
	return nil
}

// Prefix sets the prefix
func (s *svc) Prefix() string {
	return s.conf.Prefix
}

// Unprotected sets the unprotected paths
func (s *svc) Unprotected() []string {
	return nil
}

// Handler propagates the request dependanding on the suffix
func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The only accepted suffix should be the conf one
		url := strings.TrimSuffix(r.URL.Path, "/")
		if url != "" {
			s.log.Warn().Msgf("takeout: %s is not a supported suffix", url)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Dispatch depending on request method
		s.log.Info().Msgf("takeout: handling method %s", r.Method)
		switch r.Method {
		case http.MethodPost:
			s.handlePost(w, r)
		case http.MethodGet:
			s.handleGet(w, r)

		default:
			s.log.Warn().Msgf("takeout: %s is not a supported method", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

func (s *svc) handlePost(w http.ResponseWriter, r *http.Request) {
	// Parse parameters from the request body
	req := struct {
		ArchiveFormat  string `json:"archiveFormat"`
		MaxArchiveSize int64  `json:"maxArchiveSize"`
	}{
		// Default values in case they're not provided
		ArchiveFormat:  "zip",
		MaxArchiveSize: 2 << 30, // 2 GiB
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		s.log.Err(err).Msg("takeout: could not decode job parameters")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get job runner
	runner := rjobs.Default()
	if runner == nil {
		s.log.Error().Msg("takeout: could not find runner")
		w.WriteHeader(http.StatusFailedDependency)
		return
	}

	// Get current authenticated user
	user := appctx.ContextMustGetUser(r.Context())

	// Enqueue job
	runId, err := runner.Enqueue(r.Context(), takeout.JobName, rjobs.Params{
		"archiveFormat":  req.ArchiveFormat,
		"maxArchiveSize": req.MaxArchiveSize,
		"username":       user.Username,
	}, rjobs.WithOwner(user.Username), rjobs.Unique("takeout:"+user.Username))
	if err != nil {
		s.log.Err(err).Msg("takeout: could not enqueue job")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Reply with job ID
	s.log.Info().Msgf("takeout: takeout job %s enqueued", runId)
	w.WriteHeader(http.StatusOK)
}

func (s *svc) handleGet(w http.ResponseWriter, r *http.Request) {
	// Get job runner
	runner := rjobs.Default()
	if runner == nil {
		s.log.Error().Msg("takeout: could not find runner")
		w.WriteHeader(http.StatusFailedDependency)
		return
	}

	// Get takeout job from username, if any
	user := appctx.ContextMustGetUser(r.Context())
	jobs, err := runner.ListByOwner(r.Context(), user.Username, rjobs.ListFilter{Job: "takeout"})
	if err != nil {
		s.log.Err(err).Msg("takeout: could not list user's jobs")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(jobs) == 0 {
		s.log.Debug().Msgf("takeout: user %s has no takeout job listed", user.Username)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Handle latest job
	st := jobs[0]
	rep := statusReply{
		RunID:      string(st.RunID),
		State:      string(st.State),
		EnqueuedAt: st.EnqueuedAt,
		StartedAt:  st.StartedAt,
		FinishedAt: st.FinishedAt,
	}
	switch st.State {
	case rjobs.StateFailed:
		rep.Error = st.LastError
	case rjobs.StateSucceeded:
		// Reply with the public link to the archives
		url, okT := st.Result["archives_url"].(string)
		path, okP := st.Result["archives_path"].(string)
		if !okT || !okP {
			// Unreachable
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		rep.ArchivesURL = url
		rep.ArchivesPath = path
	case rjobs.StateQueued, rjobs.StateRunning:
		// Nothing to add
	default:
		// Unreachable
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Encode and send the JSON reply
	body, err := json.Marshal(rep)
	if err != nil {
		// Unreachable
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
