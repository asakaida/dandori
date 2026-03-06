package cmd

import (
	"context"
	"fmt"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new workflow",
		RunE:  runStart,
	}
	cmd.Flags().String("id", "", "Workflow ID (optional, auto-generated if empty)")
	cmd.Flags().String("type", "", "Workflow type (required)")
	cmd.Flags().String("queue", "default", "Task queue")
	cmd.Flags().String("input", "{}", "Input JSON")
	_ = cmd.MarkFlagRequired("type")
	rootCmd.AddCommand(cmd)
}

func runStart(cmd *cobra.Command, _ []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	id, _ := cmd.Flags().GetString("id")
	wfType, _ := cmd.Flags().GetString("type")
	queue, _ := cmd.Flags().GetString("queue")
	input, _ := cmd.Flags().GetString("input")

	resp, err := client.StartWorkflow(context.Background(), &apiv1.StartWorkflowRequest{
		WorkflowId:   id,
		WorkflowType: wfType,
		TaskQueue:    queue,
		Input:        []byte(input),
	})
	if err != nil {
		return err
	}
	fmt.Printf("Started workflow: %s\n", resp.GetWorkflowId())
	return nil
}
