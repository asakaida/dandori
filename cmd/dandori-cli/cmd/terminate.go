package cmd

import (
	"context"
	"fmt"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "terminate <workflow_id>",
		Short: "Terminate a workflow",
		Args:  cobra.ExactArgs(1),
		RunE:  runTerminate,
	}
	cmd.Flags().String("reason", "", "Termination reason")
	rootCmd.AddCommand(cmd)
}

func runTerminate(cmd *cobra.Command, args []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	reason, _ := cmd.Flags().GetString("reason")
	_, err = client.TerminateWorkflow(context.Background(), &apiv1.TerminateWorkflowRequest{
		WorkflowId: args[0],
		Reason:     reason,
		Namespace:  namespace,
	})
	if err != nil {
		return err
	}
	fmt.Println("Workflow terminated")
	return nil
}
