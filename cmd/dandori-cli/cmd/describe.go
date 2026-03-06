package cmd

import (
	"context"
	"fmt"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "describe <workflow_id>",
		Short: "Describe a workflow",
		Args:  cobra.ExactArgs(1),
		RunE:  runDescribe,
	})
}

func runDescribe(_ *cobra.Command, args []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := client.DescribeWorkflow(context.Background(), &apiv1.DescribeWorkflowRequest{
		WorkflowId: args[0],
	})
	if err != nil {
		return err
	}

	wf := resp.GetWorkflowExecution()
	fmt.Printf("ID:       %s\n", wf.GetId())
	fmt.Printf("Type:     %s\n", wf.GetWorkflowType())
	fmt.Printf("Queue:    %s\n", wf.GetTaskQueue())
	fmt.Printf("Status:   %s\n", wf.GetStatus())
	fmt.Printf("Created:  %s\n", wf.GetCreatedAt().AsTime().Format("2006-01-02 15:04:05"))
	if wf.GetClosedAt() != nil {
		fmt.Printf("Closed:   %s\n", wf.GetClosedAt().AsTime().Format("2006-01-02 15:04:05"))
	}
	if len(wf.GetResult()) > 0 {
		fmt.Printf("Result:   %s\n", string(wf.GetResult()))
	}
	if wf.GetErrorMessage() != "" {
		fmt.Printf("Error:    %s\n", wf.GetErrorMessage())
	}
	return nil
}
