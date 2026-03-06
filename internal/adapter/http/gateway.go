package http

import (
	"context"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	apiv1 "github.com/asakaida/dandori/api/v1"
)

// NewGatewayMux creates a gRPC-Gateway ServeMux that proxies HTTP/JSON requests to the gRPC server.
func NewGatewayMux(ctx context.Context, grpcAddr string) (*runtime.ServeMux, error) {
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
	)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := apiv1.RegisterDandoriServiceHandlerFromEndpoint(ctx, mux, grpcAddr, opts); err != nil {
		return nil, err
	}

	return mux, nil
}

// NewHTTPHandler creates an http.Handler that routes to the gateway mux and swagger endpoint.
func NewHTTPHandler(gatewayMux *runtime.ServeMux) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/swagger.json", swaggerHandler())
	mux.Handle("/", gatewayMux)
	return mux
}
