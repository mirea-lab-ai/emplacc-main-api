package request

type ProblemCreateRequest struct {
	Description *[]string `json:"description"`
	CreatorID   string    `json:"creator_id"`
	Name        *string   `json:"name"`
}

type ProblemListByUserId struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type ProblemUpdateRequest struct {
	Name        *string   `json:"name"`
	Description *[]string `json:"description"`
}

type ProblemsListRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}
