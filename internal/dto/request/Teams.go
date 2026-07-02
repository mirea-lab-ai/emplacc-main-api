package request

// DTO для создания команды
// POST /team

type TeamCreateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	UsersIDs    []string `json:"user_ids"`
}

type GetAllProjectsRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// DTO для обновления команды
// PATCH /team/:id

type TeamUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type TeamAddUsersRequest struct {
	TeamID  string   `json:"team_id"`
	UserIDs []string `json:"user_ids"`
}

type TeamDeleteUserRequest struct {
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
}

type TeamAddProjectRequest struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
}

type TeamDeleteProjectRequest struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
}

type TeamUpdateMemberRoleRequest struct {
	TeamID         string `json:"team_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"`
	Specialization string `json:"specialization" validate:"required"`
}
