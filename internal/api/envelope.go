package api

type V1ItemResponse struct {
	Data map[string]interface{} `json:"data"`
}

type V1ListResponse struct {
	Data       []map[string]interface{} `json:"data"`
	Pagination V1Pagination             `json:"pagination"`
}

type V1Pagination struct {
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
}

type V1DeletedResponse struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}

func AsMapSlice(value interface{}) []map[string]interface{} {
	switch items := value.(type) {
	case []map[string]interface{}:
		return items
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}
