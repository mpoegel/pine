package api

type TreeStatusResponse struct {
	TreeName   string `json:"name"`
	State      string `json:"status"`
	LastChange uint64 `json:"lastChange"`
	Uptime     uint64 `json:"uptime"`
}

type ListTreesResponse struct {
	Trees []TreeStatusResponse `json:"trees"`
}
