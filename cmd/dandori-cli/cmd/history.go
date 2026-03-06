package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	apiv1 "github.com/asakaida/dandori/api/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "history <workflow_id>",
		Short: "Show workflow history",
		Args:  cobra.ExactArgs(1),
		RunE:  runHistory,
	})
}

func runHistory(cmd *cobra.Command, args []string) error {
	client, conn, err := newClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := client.GetWorkflowHistory(context.Background(), &apiv1.GetWorkflowHistoryRequest{
		WorkflowId: args[0],
	})
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SEQ\tEVENT_TYPE\tTIMESTAMP\tDATA")
	for _, e := range resp.GetEvents() {
		data := string(e.GetEventData())
		if len(data) > 80 {
			data = data[:80] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			e.GetSequenceNum(),
			e.GetEventType(),
			e.GetTimestamp().AsTime().Format("2006-01-02 15:04:05"),
			data,
		)
	}
	w.Flush()
	return nil
}
