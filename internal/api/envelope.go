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
