package apperr

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// String returns a stable, lower-case identifier for the kind (e.g. "not_found").
func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not_found"
	case KindUnavailable:
		return "unavailable"
	case KindInvalid:
		return "invalid"
	case KindUnauthorized:
		return "unauthorized"
	case KindConflict:
		return "conflict"
	case KindInternal:
		return "internal"
	case KindCircuitOpen:
		return "circuit_open"
	case KindProviderExhausted:
		return "provider_exhausted"
	case KindDeadline:
		return "deadline"
	case KindCanceled:
		return "canceled"
	case KindNotImplemented:
		return "not_implemented"
	default:
		return "unknown"
	}
}

// CodeToHTTP maps a Kind to an HTTP status code.
func CodeToHTTP(k Kind) int {
	switch k {
	case KindNotFound:
		return http.StatusNotFound
	case KindUnavailable, KindCircuitOpen, KindProviderExhausted:
		return http.StatusServiceUnavailable
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindConflict:
		return http.StatusConflict
	case KindDeadline:
		return http.StatusGatewayTimeout
	case KindCanceled:
		return 499 // client closed request (nginx convention)
	case KindNotImplemented:
		return http.StatusNotImplemented
	default: // KindInternal, KindUnknown
		return http.StatusInternalServerError
	}
}

// CodeToGRPC maps a Kind to a gRPC status.
func CodeToGRPC(k Kind) *status.Status {
	switch k {
	case KindNotFound:
		return status.New(codes.NotFound, k.String())
	case KindUnavailable, KindCircuitOpen, KindProviderExhausted:
		return status.New(codes.Unavailable, k.String())
	case KindInvalid:
		return status.New(codes.InvalidArgument, k.String())
	case KindUnauthorized:
		return status.New(codes.Unauthenticated, k.String())
	case KindConflict:
		return status.New(codes.AlreadyExists, k.String())
	case KindDeadline:
		return status.New(codes.DeadlineExceeded, k.String())
	case KindCanceled:
		return status.New(codes.Canceled, k.String())
	case KindNotImplemented:
		return status.New(codes.Unimplemented, k.String())
	default: // KindInternal, KindUnknown
		return status.New(codes.Internal, k.String())
	}
}
