package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/token"
	"github.com/cernbox/reva/pkg/user"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	tokenmgr "github.com/cernbox/reva/pkg/token/manager/registry"

	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

var logger = log.New("grpc-interceptor-auth")
var errors = err.New("grpc-interceptor-auth")

func init() {
	grpcserver.RegisterUnaryInterceptor("auth", NewUnary)
	grpcserver.RegisterStreamInterceptor("auth", NewStream)
}

type config struct {
	Priority        int                               `mapstructure:"priority"`
	Methods         map[string]bool                   `mapstructure:"methods"`
	Header          string                            `mapstructure:"header"`
	TokenStrategy   string                            `mapstructure:"token_strategy"`
	TokenStrategies map[string]map[string]interface{} `mapstructure:"token_strategies"`
	TokenManager    string                            `mapstructure:"token_manager"`
	TokenManagers   map[string]map[string]interface{} `mapstructure:"token_managers"`
}

// NewUnary returns a new unary interceptor that adds
// trace information for the request.
func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, errors.Wrap(err, "error decoding conf")
	}

	if conf.Header == "" {
		return nil, 0, errors.New("header is empty")
	}

	for k := range conf.Methods {
		logger.Println(context.Background(), "grpc auth protected method: ", k)
	}

	h, ok := tokenmgr.NewFuncs[conf.TokenManager]
	if !ok {
		return nil, 0, errors.New("token manager not found: " + conf.TokenStrategy)
	}

	tokenManager, err := h(conf.TokenManagers[conf.TokenManager])
	if err != nil {
		return nil, 0, errors.Wrap(err, "error creating token manager")
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		method := strings.ToLower(info.FullMethod)
		if _, ok := conf.Methods[method]; !ok {
			return handler(ctx, req)
		}

		var tkn string
		md, ok := metadata.FromIncomingContext(ctx)
		if ok && md != nil {
			if val, ok := md[conf.Header]; ok {
				if len(val) > 0 && val[0] != "" {
					tkn = val[0]
				}
			}
		}

		if tkn == "" {
			return nil, grpc.Errorf(codes.Unauthenticated, "core access token not found")
		}

		// validate the token
		claims, err := tokenManager.DismantleToken(ctx, tkn)
		if err != nil {
			return nil, grpc.Errorf(codes.Unauthenticated, "core access token is invalid")
		}

		u := &user.User{}
		if err := mapstructure.Decode(claims, u); err != nil {
			return nil, grpc.Errorf(codes.Unauthenticated, "claims are invalid")
		}

		// store user and core access token in context.
		ctx = user.ContextSetUser(ctx, u)
		ctx = token.ContextSetToken(ctx, tkn)
		return handler(ctx, req)
	}
	return interceptor, conf.Priority, nil
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, errors.Wrap(err, "error decoding conf")
	}

	if conf.Header == "" {
		return nil, 0, errors.New("header is empty")
	}

	h, ok := tokenmgr.NewFuncs[conf.TokenManager]
	if !ok {
		return nil, 0, fmt.Errorf("token manager not found: %s", conf.TokenStrategy)
	}

	tokenManager, err := h(conf.TokenManagers[conf.TokenManager])
	if err != nil {
		return nil, 0, errors.New("token manager not found: " + conf.TokenStrategy)
	}

	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		method := strings.ToLower(info.FullMethod)
		if _, ok := conf.Methods[method]; !ok {
			return handler(srv, ss)
		}

		ctx := ss.Context()
		var tkn string
		md, ok := metadata.FromIncomingContext(ss.Context())
		if ok && md != nil {
			if val, ok := md[conf.Header]; ok {
				if len(val) > 0 && val[0] != "" {
					tkn = val[0]
				}
			}
		}

		if tkn == "" {
			return grpc.Errorf(codes.Unauthenticated, "core access token is invalid")
		}

		// validate the token
		claims, err := tokenManager.DismantleToken(ctx, tkn)
		if err != nil {
			return grpc.Errorf(codes.Unauthenticated, "core access token is invalid")
		}

		u := &user.User{}
		if err := mapstructure.Decode(claims, u); err != nil {
			return grpc.Errorf(codes.Unauthenticated, "claims are invalid")
		}

		// store user and core access token in context.
		ctx = user.ContextSetUser(ctx, u)
		ctx = token.ContextSetToken(ctx, tkn)

		wrapped := newWrappedServerStream(ctx, ss)
		return handler(srv, wrapped)
	}
	return interceptor, conf.Priority, nil
}

func newWrappedServerStream(ctx context.Context, ss grpc.ServerStream) *wrappedServerStream {
	return &wrappedServerStream{ServerStream: ss, newCtx: ctx}
}

type wrappedServerStream struct {
	grpc.ServerStream
	newCtx context.Context
}

func (ss *wrappedServerStream) Context() context.Context {
	return ss.newCtx
}
