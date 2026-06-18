package observability

import (
	"google.golang.org/grpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

// GRPCServerOptions returns grpc.Server options that install OpenTelemetry
// stats/tracing handling. Pass the result (spread) to grpc.NewServer. It uses
// the global providers, so it is a no-op until Setup installs exporters.
//
// Example: grpc.NewServer(append(observability.GRPCServerOptions(), opts...)...)
func GRPCServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}
}
