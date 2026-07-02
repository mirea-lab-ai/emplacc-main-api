package request

type CreateForumMessageRequest struct {
	ProblemID   string    `json:"problem_id" validate:"required,uuid"`
	Description *[]string `json:"description" validate:"required,max=255"`
	CreatorID   string    `json:"creator_id" validate:"required,uuid"`
	ReplyToID   *string   `json:"reply_to_id" validate:"omitempty,uuid"`
}

type UpdateForumMessageRequest struct {
	ProblemID   *string   `json:"problem_id" validate:"omitempty,uuid"`
	Description *[]string `json:"description" validate:"omitempty,max=255"`
	CreatorID   *string   `json:"creator_id" validate:"omitempty,uuid"`
}

type ForumMessageListRequest struct {
	Page     int `json:"page" validate:"required,min=1"`
	PageSize int `json:"page_size" validate:"required,min=1,max=100"`
}

type ForumMessageListByProblemIdRequest struct {
	Page     int `json:"page" validate:"required,min=1"`
	PageSize int `json:"page_size" validate:"required,min=1,max=100"`
}
