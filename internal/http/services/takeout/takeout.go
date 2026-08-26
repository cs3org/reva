package takeout

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/takeout"
	"github.com/cs3org/reva/v3/pkg/takeout/cleanup"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
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
	if err := cleanup.RegisterCleanup(cleanupConfig, l); err != nil {
		return nil, errors.Wrap(err, "takeout: cleanup job registration failed")
	}

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

// The status JSON reply structure
type statusReply struct {
	RunID       string     `json:"run_id"`
	State       string     `json:"state"`
	EnqueuedAt  time.Time  `json:"enqueued_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	ArchivesTok string     `json:"archives_tok,omitempty"`
	Skipped     int        `json:"skipped,omitempty"`
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

// Handler propagates the request depending on the suffix
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
		s.log.Debug().Msgf("takeout: handling method %s", r.Method)
		switch r.Method {
		case http.MethodPost:
			s.handlePost(w, r)
		case http.MethodGet:
			s.handleGet(w, r)
		case http.MethodDelete:
			s.handleDelete(w, r)
		default:
			s.log.Warn().Msgf("takeout: %s is not a supported method", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (s *svc) handlePost(w http.ResponseWriter, r *http.Request) {
	// Parse parameters from the request body
	// Unset parameters are defaulted by the job
	var req struct {
		ArchiveFormat  string `json:"archive_format"`
		MaxArchiveSize int64  `json:"max_archive_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		s.log.Err(err).Msg("takeout: could not decode job parameters")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get job runner
	runner, ok := s.runner(w)
	if !ok {
		return
	}

	// Get current authenticated user
	user := appctx.ContextMustGetUser(r.Context())

	// Get root path
	gtw, err := service.Gateway(r.Context())
	if err != nil {
		s.log.Err(err).Msg("takeout: could not get gateway")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	homeRes, err := gtw.GetHome(r.Context(), &provider.GetHomeRequest{})
	switch {
	case err != nil:
		s.log.Err(err).Msg("takeout: could not find home directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	case homeRes.Status.GetCode() != rpc.Code_CODE_OK:
		s.log.Error().Msgf("takeout: could not find home directory: %s", homeRes.Status.Message)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	rootPath := homeRes.Path

	// Enqueue job
	runID, err := runner.Enqueue(r.Context(), takeout.JobName, rjobs.Params{
		"archive_format":   req.ArchiveFormat,
		"max_archive_size": req.MaxArchiveSize,
		"username":         user.Username,
		"root_path":        rootPath,
	}, rjobs.WithOwner(user.Username), rjobs.Unique("takeout:"+user.Username))
	if err != nil {
		s.log.Err(err).Msg("takeout: could not enqueue job")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	s.log.Info().Msgf("takeout: takeout job %s enqueued", runID)
	w.WriteHeader(http.StatusOK)
}

func (s *svc) handleGet(w http.ResponseWriter, r *http.Request) {
	// Get job runner
	runner, ok := s.runner(w)
	if !ok {
		return
	}

	// Handle latest job
	st, ok := s.latestJob(w, r, runner)
	if !ok {
		return
	}
	s.respondWithStatus(w, st)
}

func (s *svc) handleDelete(w http.ResponseWriter, r *http.Request) {
	// Get job runner
	runner, ok := s.runner(w)
	if !ok {
		return
	}

	// Get latest job
	st, ok := s.latestJob(w, r, runner)
	if !ok {
		return
	}

	// Cancel it, cancelling a finished job is a no-op
	cancelled, err := runner.Cancel(r.Context(), st.RunID)
	if err != nil {
		s.log.Err(err).Msg("takeout: could not cancel job")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Reply with the updated status
	s.log.Info().Msgf("takeout: takeout job %s cancelled", cancelled.RunID)
	s.respondWithStatus(w, cancelled)
}

// runner returns the process-wide job runner, it replies itself and reports false
// when there is none to hand back
func (s *svc) runner(w http.ResponseWriter) (*rjobs.Runner, bool) {
	runner := rjobs.Default()
	if runner == nil {
		s.log.Error().Msg("takeout: could not find runner")
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	return runner, true
}

// latestJob returns the most recent takeout job of the authenticated user, it replies
// itself and reports false when there is none to hand back
func (s *svc) latestJob(w http.ResponseWriter, r *http.Request, runner *rjobs.Runner) (rjobs.Status, bool) {
	// Get takeout job from username, if any
	user := appctx.ContextMustGetUser(r.Context())
	jobs, err := runner.ListByOwner(r.Context(), user.Username, rjobs.ListFilter{Job: takeout.JobName})
	if err != nil {
		s.log.Err(err).Msg("takeout: could not list user's jobs")
		w.WriteHeader(http.StatusInternalServerError)
		return rjobs.Status{}, false
	}
	if len(jobs) == 0 {
		s.log.Debug().Msgf("takeout: user %s has no takeout job attached", user.Username)
		w.WriteHeader(http.StatusNotFound)
		return rjobs.Status{}, false
	}
	return jobs[0], true
}

// respondWithStatus sends the JSON reply describing the given job
func (s *svc) respondWithStatus(w http.ResponseWriter, st rjobs.Status) {
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
		// Reply with the public link to the archives, their location stays internal
		tok, ok := st.Result["archives_tok"].(string)
		if !ok {
			// Unreachable
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		rep.ArchivesTok = tok
		// The result is stored as JSON, so the count comes back as a number
		if skipped, ok := st.Result["skipped_count"].(float64); ok {
			rep.Skipped = int(skipped)
		}
	case rjobs.StateQueued, rjobs.StateRunning, rjobs.StateCancelling, rjobs.StateCancelled:
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
