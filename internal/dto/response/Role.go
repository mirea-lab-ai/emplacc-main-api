package response

import "time"

type GetAllRolesResponse struct {
	Roles      []GetRoleResponse `json:"roles"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
}

type GetRoleResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type RoleUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type GetRoleByUserId struct {
	UserId string           `json:"user_id"`
	Role   *GetRoleResponse `json:"role"`
}
