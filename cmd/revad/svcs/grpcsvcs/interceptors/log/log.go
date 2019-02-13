package log

import (
	"context"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

var logger = log.New("log")

func init() {
	grpcserver.RegisterUnaryInterceptor("log", NewUnary)
}

type config struct {
	Priority int `mapstructure:"priority"`
}

// NewUnary returns a new unary interceptor
// that logs grpc calls.
func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		//logger.Println(ctx, info.FullMethod, req)
		logger.Println(ctx, info.FullMethod)
		return handler(ctx, req)
	}
	return interceptor, conf.Priority, nil
}
