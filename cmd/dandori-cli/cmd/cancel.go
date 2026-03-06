package cmd

import (
	"context"
	"fmt"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "cancel <workflow_id>",
		Short: "Cancel a workflow",
		Args:  cobra.ExactArgs(1),
		RunE:  runCancel,
	})
}

func runCancel(_ *cobra.Command, args []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = client.CancelWorkflow(context.Background(), &apiv1.CancelWorkflowRequest{
		WorkflowId: args[0],
		Namespace:  namespace,
	})
	if err != nil {
		return err
	}
	fmt.Println("Cancel requested")
	return nil
}
