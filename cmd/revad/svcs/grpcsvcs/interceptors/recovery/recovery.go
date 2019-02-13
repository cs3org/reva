package recovery

import (
	"context"
	"fmt"

	"github.com/cernbox/reva/pkg/log"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var logger = log.New("grpc-recovery")

func init() {
	grpcserver.RegisterUnaryInterceptor("recovery", NewUnary)
	grpcserver.RegisterStreamInterceptor("recovery", NewStream)
}

type config struct {
	Priority int `mapstructure:"priority"`
}

// NewUnary returns a server interceptor that adds telemetry to
// grpc calls.
func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}
	interceptor := grpc_recovery.UnaryServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc))
	return interceptor, conf.Priority, nil
}

// NewStream returns a streaming server inteceptor that adds telemetry to
// streaming grpc calls.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}
	interceptor := grpc_recovery.StreamServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc))
	return interceptor, conf.Priority, nil
}
func recoveryFunc(ctx context.Context, p interface{}) (err error) {
	logger.Panic(ctx, fmt.Sprintf("%+v", p))
	return grpc.Errorf(codes.Internal, "%s", p)
}
