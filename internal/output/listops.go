package output

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type ListOps struct {
	Columns string
	Filters []string
	Sort string
}

func (o ListOps) Empty() bool {
	return o.Columns == "" && len(o.Filters) == 0 && o.Sort == ""
}

func ApplyListOps(rows []map[string]interface{}, columns []Column, ops ListOps) ([]map[string]interface{}, []Column, error) {
	if ops.Empty() {
		return rows, columns, nil
	}

	out := rows
	if len(ops.Filters) > 0 {
		filtered := make([]map[string]interface{}, 0, len(out))
		preds, err := parseFilters(ops.Filters)
		if err != nil {
			return nil, nil, err
		}
		for _, row := range out {
			if matchesAll(row, preds) {
				filtered = append(filtered, row)
			}
		}
		out = filtered
	}

	if ops.Sort != "" {
		field, desc := ops.Sort, false
		if strings.HasPrefix(field, "-") {
			field, desc = field[1:], true
		}
		sorted := make([]map[string]interface{}, len(out))
		copy(sorted, out)
		sort.SliceStable(sorted, func(i, j int) bool {
			c := compareValue(extractField(sorted[i], field), extractField(sorted[j], field))
			if desc {
				return c > 0
			}
			return c < 0
		})
		out = sorted
	}

	cols := columns
	if ops.Columns != "" {
		cols = columnsFromFields(ops.Columns)
	}
	return out, cols, nil
}

type filterPred struct {
	field    string
	value    string
	contains bool // true = substring (~), false = exact (=)
}

func parseFilters(filters []string) ([]filterPred, error) {
	preds := make([]filterPred, 0, len(filters))
	for _, f := range filters {
		if i := strings.Index(f, "~"); i > 0 {
			preds = append(preds, filterPred{field: f[:i], value: strings.ToLower(f[i+1:]), contains: true})
			continue
		}
		if i := strings.Index(f, "="); i > 0 {
			preds = append(preds, filterPred{field: f[:i], value: strings.ToLower(f[i+1:]), contains: false})
			continue
		}
		return nil, fmt.Errorf("invalid --filter %q: expected field=value or field~value", f)
	}
	return preds, nil
}

func matchesAll(row map[string]interface{}, preds []filterPred) bool {
	for _, p := range preds {
		got := strings.ToLower(formatValue(extractField(row, p.field)))
		if p.contains {
			if !strings.Contains(got, p.value) {
				return false
			}
		} else if got != p.value {
			return false
		}
	}
	return true
}

func columnsFromFields(spec string) []Column {
	var cols []Column
	for _, raw := range strings.Split(spec, ",") {
		field := strings.TrimSpace(raw)
		if field == "" {
			continue
		}
		leaf := field
		if i := strings.LastIndex(field, "."); i >= 0 {
			leaf = field[i+1:]
		}
		cols = append(cols, Column{Header: strings.ToUpper(leaf), Field: field})
	}
	return cols
}

func compareValue(a, b interface{}) int {
	as, bs := formatValue(a), formatValue(b)
	af, aerr := strconv.ParseFloat(as, 64)
	bf, berr := strconv.ParseFloat(bs, 64)
	if aerr == nil && berr == nil {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(as, bs)
}
