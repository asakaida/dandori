package main

import (
	"log"
	"net"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":7233")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	apiv1.RegisterDandoriServiceServer(srv, &apiv1.UnimplementedDandoriServiceServer{})

	log.Println("dandori server listening on :7233")
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
