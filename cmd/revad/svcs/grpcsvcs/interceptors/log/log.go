package log

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

var logger = log.New("grpc-interceptor-log")

func init() {
	grpcserver.RegisterUnaryInterceptor("log", NewUnary)
	grpcserver.RegisterStreamInterceptor("log", NewStream)
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
		start := time.Now()
		res, err := handler(ctx, req)
		code := grpc.Code(err)
		end := time.Now()
		diff := end.Sub(start).Nanoseconds()

		var b *log.Builder
		if code != codes.OK {
			b = logger.BuildError()
		} else {
			b = logger.Build()
		}

		b = b.Str("uri", info.FullMethod)
		b = b.Str("start", start.Format("02/Jan/2006:15:04:05 -0700"))
		b = b.Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff))
		b = b.Str("code", code.String())
		b.Msg(ctx, "GRPC unary call")

		return res, err
	}
	return interceptor, conf.Priority, nil
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		end := time.Now()
		code := grpc.Code(err)
		diff := end.Sub(start).Nanoseconds()

		var b *log.Builder
		if code != codes.OK {
			b = logger.BuildError()
		} else {
			b = logger.Build()
		}

		b = b.Str("uri", info.FullMethod)
		b = b.Str("start", start.Format("02/Jan/2006:15:04:05 -0700"))
		b = b.Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff))
		b = b.Str("code", code.String())
		b.Msg(ss.Context(), "GRPC stream call")

		return err
	}
	return interceptor, conf.Priority, nil
}
