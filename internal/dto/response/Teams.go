package response

import "time"

// Для GET /team/all

type TeamsListResponse struct {
	Teams []TeamResponse `json:"teams"`
}

// Для GET /team/:id, POST /team, PATCH /team/:id

type TeamResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	UpdatedAt   time.Time            `json:"updated_at"`
	CreatedAt   time.Time            `json:"created_at"`
	Members     []TeamMemberResponse `json:"members"`
}

// Для участника команды

type TeamMemberResponse struct {
	UserID         string `json:"user_id"`
	Specialization string `json:"specialization"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Email          string `json:"email"`
	AvatarURL      string `json:"avatar_url,omitempty"`
}

// Для DELETE /team/:id (если нужен ответ)

type TeamUniversalResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type TeamUniversalUserResponse struct {
	TeamID  string `json:"team_id"`
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

type TeamUniversalProjectResponse struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
	Message   string `json:"message"`
}

type UsersAddResponse struct {
	TeamID  string   `json:"team_id"`
	UsersID []string `json:"users_id"`
	Message string   `json:"message"`
}
