package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		RunE:  runList,
	}
	cmd.Flags().String("status", "", "Filter by status")
	cmd.Flags().String("type", "", "Filter by workflow type")
	cmd.Flags().String("queue", "", "Filter by task queue")
	cmd.Flags().Int32("limit", 20, "Page size")
	cmd.Flags().String("token", "", "Next page token")
	rootCmd.AddCommand(cmd)
}

func runList(cmd *cobra.Command, _ []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	statusFilter, _ := cmd.Flags().GetString("status")
	typeFilter, _ := cmd.Flags().GetString("type")
	queueFilter, _ := cmd.Flags().GetString("queue")
	limit, _ := cmd.Flags().GetInt32("limit")
	token, _ := cmd.Flags().GetString("token")

	resp, err := client.ListWorkflows(context.Background(), &apiv1.ListWorkflowsRequest{
		PageSize:      limit,
		NextPageToken: token,
		StatusFilter:  statusFilter,
		TypeFilter:    typeFilter,
		QueueFilter:   queueFilter,
		Namespace:     namespace,
	})
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tTYPE\tQUEUE\tCREATED")
	for _, wf := range resp.GetWorkflows() {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			wf.GetId(),
			wf.GetStatus(),
			wf.GetWorkflowType(),
			wf.GetTaskQueue(),
			wf.GetCreatedAt().AsTime().Format("2006-01-02 15:04:05"),
		)
	}
	w.Flush()

	if t := resp.GetNextPageToken(); t != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\nNext page token: %s\n", t)
	}
	return nil
}
