package cmdutil

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

const maxAllRows = 10000

func listOpsFromCmd(cmd *cobra.Command) output.ListOps {
	var ops output.ListOps
	if cmd == nil {
		return ops
	}
	if v, err := cmd.Flags().GetString("columns"); err == nil {
		ops.Columns = v
	}
	if v, err := cmd.Flags().GetStringArray("filter"); err == nil {
		ops.Filters = v
	}
	if v, err := cmd.Flags().GetString("sort"); err == nil {
		ops.Sort = v
	}
	return ops
}

func (f *Factory) RenderList(cmd *cobra.Command, rows []map[string]interface{}, columns []output.Column, emptyMessage string) error {
	rows, columns, err := output.ApplyListOps(rows, columns, listOpsFromCmd(cmd))
	if err != nil {
		return err
	}
	return f.ListFormatter(emptyMessage).WriteList(rows, columns)
}

func (f *Factory) FetchList(ctx context.Context, cmd *cobra.Command, client *api.Client, path string, extra url.Values) (rows []map[string]interface{}, hasMore bool, err error) {
	all := false
	limit := 50
	cursor := ""
	if cmd != nil {
		if v, e := cmd.Flags().GetBool("all"); e == nil {
			all = v
		}
		if v, e := cmd.Flags().GetInt("limit"); e == nil && v > 0 {
			limit = v
		}
		if v, e := cmd.Flags().GetString("cursor"); e == nil {
			cursor = v
		}
	}

	if all {
		it := api.NewListIterator(client, path, api.PageParams{Limit: limit}, extra)
		for it.Next(ctx) {
			rows = append(rows, it.Item())
			if len(rows) >= maxAllRows {
				fmt.Fprintf(f.ErrOut, "Note: --all stopped at the %d-row safety cap; narrow with --filter or page with --cursor.\n", maxAllRows)
				break
			}
		}
		if err := it.Err(); err != nil {
			return nil, false, err
		}
		return rows, false, nil
	}

	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	for k, vs := range extra {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	var resp api.V1ListResponse
	if err := client.Get(ctx, path, q, &resp); err != nil {
		return nil, false, err
	}
	return resp.Data, resp.Pagination.HasMore, nil
}

func (f *Factory) MaybeMoreHint(hasMore bool) {
	if hasMore && f.Format == output.FormatTable {
		fmt.Fprintln(f.ErrOut, "More results available — use --all to fetch every page, or --cursor to page.")
	}
}
