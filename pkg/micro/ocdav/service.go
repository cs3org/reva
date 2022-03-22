package ocdav

import (
	"strings"

	httpServer "github.com/asim/go-micro/plugins/server/http/v4"
	"github.com/cs3org/reva/v2/internal/http/interceptors/appctx"
	"github.com/cs3org/reva/v2/internal/http/interceptors/auth"
	revaLogMiddleware "github.com/cs3org/reva/v2/internal/http/interceptors/log"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/rhttp/global"
	"github.com/cs3org/reva/v2/pkg/storage/favorite/memory"
	"github.com/go-chi/chi/v5"
	"go-micro.dev/v4"
	"go-micro.dev/v4/registry"
	"go-micro.dev/v4/server"

	// initialize reva registries by importing the relevant loader packages
	_ "github.com/cs3org/reva/v2/internal/http/interceptors/auth/credential/loader"
	_ "github.com/cs3org/reva/v2/internal/http/interceptors/auth/token/loader"
	_ "github.com/cs3org/reva/v2/internal/http/interceptors/auth/tokenwriter/loader"
	_ "github.com/cs3org/reva/v2/pkg/token/manager/loader"
)

const (
	// ServerName to use when announcing the service to the registry
	ServerName = "ocdav"
)

// Service initializes the ocdav service and underlying http server.
func Service(opts ...Option) (micro.Service, error) {

	sopts := newOptions(opts...)

	// set defaults
	if err := setDefaults(&sopts); err != nil {
		return nil, err
	}

	sopts.Logger = sopts.Logger.With().Str("name", sopts.Name).Logger()

	srv := httpServer.NewServer(
		server.TLSConfig(sopts.TLSConfig),
		server.Name(sopts.Name),
		server.Address(sopts.Address), // Address defaults to ":0" and will pick any free port
	)

	revaService, err := ocdav.NewWith(&sopts.config, sopts.FavoriteManager, sopts.lockSystem, &sopts.Logger)
	if err != nil {
		return nil, err
	}

	// register additional webdav verbs
	chi.RegisterMethod(ocdav.MethodPropfind)
	chi.RegisterMethod(ocdav.MethodProppatch)
	chi.RegisterMethod(ocdav.MethodLock)
	chi.RegisterMethod(ocdav.MethodUnlock)
	chi.RegisterMethod(ocdav.MethodCopy)
	chi.RegisterMethod(ocdav.MethodMove)
	chi.RegisterMethod(ocdav.MethodMkcol)
	chi.RegisterMethod(ocdav.MethodReport)
	r := chi.NewRouter()

	if err := useMiddlewares(r, &sopts, revaService); err != nil {
		return nil, err
	}

	r.Handle("/*", revaService.Handler())

	hd := srv.NewHandler(r)
	if err := srv.Handle(hd); err != nil {
		return nil, err
	}

	service := micro.NewService(
		micro.Server(srv),
		micro.Registry(registry.NewRegistry()),
	)

	// Init the service? make that optional?
	service.Init()

	// finally, return the service so it can be Run() by the caller himself
	return service, nil
}

func setDefaults(sopts *Options) error {

	// set defaults
	if sopts.Name == "" {
		sopts.Name = ServerName
	}
	if sopts.lockSystem == nil {
		client, err := pool.GetGatewayServiceClient(sopts.config.GatewaySvc)
		if err != nil {
			return err
		}
		sopts.lockSystem = ocdav.NewCS3LS(client)
	}
	if sopts.FavoriteManager == nil {
		sopts.FavoriteManager, _ = memory.New(map[string]interface{}{})
	}
	if !strings.HasPrefix(sopts.config.Prefix, "/") {
		sopts.config.Prefix = "/" + sopts.config.Prefix
	}
	return nil
}

func useMiddlewares(r *chi.Mux, sopts *Options, svc global.Service) error {

	// auth
	for _, v := range svc.Unprotected() {
		sopts.Logger.Info().Msgf("unprotected URL: %s", v)
	}
	authMiddle, err := auth.New(map[string]interface{}{
		"gatewaysvc": sopts.config.GatewaySvc,
		"token_managers": map[string]interface{}{
			"jwt": map[string]interface{}{
				"secret": "Pive-Fumkiu4",
			},
		},
	}, svc.Unprotected())
	if err != nil {
		return err
	}

	// log
	lm := revaLogMiddleware.New()
	// TODO wrap middlewares with traceHandler

	// ctx
	cm := appctx.New(sopts.Logger)

	// actually register
	r.Use(authMiddle, lm, cm)

	return nil
}

/*
// TODO use traceHandler() to wrap other middlewares as in rhttp
func traceHandler(name string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := rtrace.Propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		t := rtrace.Provider.Tracer("reva")
		ctx, span := t.Start(ctx, name)
		defer span.End()

		rtrace.Propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}
*/
