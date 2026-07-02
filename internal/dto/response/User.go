package response

import "time"

type GetAllUsersResponse struct {
	Users      []GetUserResponse `json:"users"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
}

type GetUserResponse struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	TgId          string    `json:"tg_id"`
	TgUserId      int64     `json:"tg_user_id"`
	Profession    string    `json:"profession"`
	EmailVerified bool      `json:"email_verified"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	LastLogin     time.Time `json:"last_login"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
}

type UserUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type AddRoleUserResponse struct {
	RoleId  string `json:"role_id"`
	UserId  string `json:"user_id"`
	Message string `json:"message"`
}

type RemoveRoleUserResponse struct {
	RoleId  string `json:"role_id"`
	UserId  string `json:"user_id"`
	Message string `json:"message"`
}
