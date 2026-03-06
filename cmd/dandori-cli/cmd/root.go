package cmd

import (
	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serverAddr string
	namespace  string
)

var rootCmd = &cobra.Command{
	Use:   "dandori-cli",
	Short: "CLI for dandori workflow engine",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:7233", "gRPC server address")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "default", "Namespace")
}

func Execute() error {
	return rootCmd.Execute()
}

func newClient() (apiv1.DandoriServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return apiv1.NewDandoriServiceClient(conn), conn, nil
}
