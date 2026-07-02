package response

import "time"

type ForumMessageAuthor struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
}

type ForumMessageReplyPreview struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	AuthorName string `json:"author_name"`
}

type ForumMessageResponse struct {
	ID          string                    `json:"id"`
	ProblemID   string                    `json:"problem_id"`
	Description []string                  `json:"description"`
	CreatorID   string                    `json:"creator_id"`
	ReplyToID   *string                   `json:"reply_to_id,omitempty"`
	Author      *ForumMessageAuthor       `json:"author,omitempty"`
	ReplyTo     *ForumMessageReplyPreview `json:"reply_to,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

type ForumMessageListResponse struct {
	Messages   []ForumMessageResponse `json:"messages"`
	TotalCount int64                  `json:"total_count"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
}

type ForumMessageUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type ForumMessageListByProblemIdResponse struct {
	ProblemId  string                 `json:"problem_id"`
	Messages   []ForumMessageResponse `json:"messages"`
	TotalCount int64                  `json:"total_count"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
}
