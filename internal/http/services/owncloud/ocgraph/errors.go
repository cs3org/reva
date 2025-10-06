package ocgraph

import (
	"context"
	"net/http"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/trace"
	"google.golang.org/grpc/codes"
	rpcstatus "google.golang.org/grpc/status"
)

func handleError(ctx context.Context, err error, w http.ResponseWriter) {
	log := appctx.GetLogger(ctx)
	w.Header().Set("x-request-id", trace.Get(ctx))
	code := rpcstatus.Code(err)
	if code == codes.Internal {
		log.Error().Err(err).Msg("ocgraph error")
	} else {
		log.Info().Err(err).Msg("ocgraph error")
	}
	w.WriteHeader(grpcCodeToHTTPStatus(code))
	w.Write([]byte("Error: " + err.Error()))
}

func handleCustomError(ctx context.Context, err error, status int, w http.ResponseWriter) {
	log := appctx.GetLogger(ctx)
	w.Header().Set("x-request-id", trace.Get(ctx))
	if status == http.StatusInternalServerError {
		log.Error().Err(err).Int("status", status).Msg("ocgraph error")
	} else {
		log.Info().Err(err).Int("status", status).Msg("ocgraph error")
	}
	w.WriteHeader(status)
	w.Write([]byte("Error: " + err.Error()))
}

func handleBadRequest(ctx context.Context, err error, w http.ResponseWriter) {
	log := appctx.GetLogger(ctx)
	w.Header().Set("x-request-id", trace.Get(ctx))
	w.WriteHeader(http.StatusBadRequest)
	log.Info().Err(err).Msg("ocgraph error")
	w.Write([]byte("Error: " + err.Error()))
}

func handleRpcStatus(ctx context.Context, status *rpcv1beta1.Status, msg string, w http.ResponseWriter) {
	log := appctx.GetLogger(ctx)
	if status == nil {
		log.Error().Str("Status", "nil").Msg(msg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Error().Str("Status", status.String()).Msg(msg)

	w.Header().Set("x-request-id", trace.Get(ctx))

	code := int32(status.Code)
	w.WriteHeader(grpcCodeToHTTPStatus(codes.Code(code)))
}

func grpcCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // Client Closed Request (non-standard)
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusBadRequest
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
