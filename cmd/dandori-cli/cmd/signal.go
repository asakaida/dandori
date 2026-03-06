package cmd

import (
	"context"
	"fmt"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "signal <workflow_id>",
		Short: "Send a signal to a workflow",
		Args:  cobra.ExactArgs(1),
		RunE:  runSignal,
	}
	cmd.Flags().String("name", "", "Signal name (required)")
	cmd.Flags().String("input", "{}", "Signal input JSON")
	_ = cmd.MarkFlagRequired("name")
	rootCmd.AddCommand(cmd)
}

func runSignal(cmd *cobra.Command, args []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	name, _ := cmd.Flags().GetString("name")
	input, _ := cmd.Flags().GetString("input")

	_, err = client.SignalWorkflow(context.Background(), &apiv1.SignalWorkflowRequest{
		WorkflowId: args[0],
		SignalName: name,
		Input:      []byte(input),
		Namespace:  namespace,
	})
	if err != nil {
		return err
	}
	fmt.Println("Signal sent")
	return nil
}
