package interceptors

import (
	"context"

	"github.com/cernbox/reva/pkg/log"
	"github.com/gofrs/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var logger = log.New("grpc-interceptor")

func LogUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		logger.Println(ctx, info.FullMethod, req)
		return handler(ctx, req)
	}
}

func TraceUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var trace string
		md, ok := metadata.FromIncomingContext(ctx)
		if ok && md != nil {
			if val, ok := md["x-trace"]; ok {
				if len(val) > 0 {
					trace = val[0]
				}
			}
		} else {
			trace = uuid.Must(uuid.NewV4()).String()
		}
		ctx = context.WithValue(ctx, "trace", trace)
		return handler(ctx, req)
	}
}

func TraceStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var trace string
		md, ok := metadata.FromIncomingContext(ss.Context())
		if ok && md != nil {
			if val, ok := md["x-trace"]; ok {
				if len(val) > 0 {
					trace = val[0]
				}
			}
		} else {
			trace = uuid.Must(uuid.NewV4()).String()
		}
		ctx := context.WithValue(ss.Context(), "trace", trace)
		wrapped := newWrappedServerStream(ss, ctx)
		return handler(srv, wrapped)
	}
}

func newWrappedServerStream(ss grpc.ServerStream, ctx context.Context) *wrappedServerStream {
	return &wrappedServerStream{ServerStream: ss, newCtx: ctx}
}

type wrappedServerStream struct {
	grpc.ServerStream
	newCtx context.Context
}

func (ss *wrappedServerStream) Context() context.Context {
	return ss.newCtx
}
