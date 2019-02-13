package prometheus

import (
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.RegisterUnaryInterceptor("prometheus", NewUnary)
	grpcserver.RegisterStreamInterceptor("prometheus", NewStream)
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
	return grpc_prometheus.UnaryServerInterceptor, conf.Priority, nil
}

// NewStream returns a streaming server inteceptor that adds telemetry to
// streaming grpc calls.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}
	return grpc_prometheus.StreamServerInterceptor, conf.Priority, nil
}
