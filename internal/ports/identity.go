package ports

import "context"

// IdentityToken — набор токенов от провайдера идентичности (Keycloak).
type IdentityToken struct {
	AccessToken      string
	RefreshToken     string
	ExpiresIn        int
	RefreshExpiresIn int
	TokenType        string
}

// UserInfo — профиль пользователя из OIDC userinfo. Поля-указатели повторяют форму
// gocloak.UserInfo, чтобы транспорт строил тот же JSON-ответ без изменений контракта.
type UserInfo struct {
	Sub               *string
	Name              *string
	PreferredUsername *string
	GivenName         *string
	FamilyName        *string
	Email             *string
	EmailVerified     *bool
}

// IdentityProvider — выходной порт провайдера идентичности (Keycloak/OIDC).
// Адаптер живёт в internal/infra/keycloak; gocloak и net/http не выходят за него.
type IdentityProvider interface {
	Login(ctx context.Context, email, password string) (*IdentityToken, error)
	Logout(ctx context.Context, refreshToken string) error
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
	RefreshToken(ctx context.Context, refreshToken string) (*IdentityToken, error)
	// VerifyToken проверяет подпись access-токена по JWKS realm.
	VerifyToken(ctx context.Context, accessToken string) error
}
