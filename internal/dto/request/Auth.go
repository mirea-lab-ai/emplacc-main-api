package request

// DTO для запроса на вход
// POST /auth/login

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// DTO для запроса на регистрацию
// POST /auth/register

type RegisterRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
	FirstName  string `json:"first_name" binding:"required"`
	LastName   string `json:"last_name" binding:"required"`
	Profession string `json:"profession"`
}

// DTO для запроса на TOTP
// POST /auth/totp

type TOTPRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

// DTO для запроса на OAuth
// POST /auth/oauth

type OAuthRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
