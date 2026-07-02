package service

import (
	"context"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AuthService interface {
	Login(email, password string) (*AuthResponse, error)
	Logout(token string) error
	GetUserInfo(token string) (*ports.UserInfo, error)
	RefreshToken(refreshToken string) (*TokenResponse, error)
	ValidateToken(token string) error
	ValidateTokenForMiddleware(token string) error // Новый метод для middleware
}

type authService struct {
	idp      ports.IdentityProvider
	userRepo ports.UserRepository
}

type AuthResponse struct {
	UserID       *string `json:"user_id"`
	Email        *string `json:"email"`
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int     `json:"expires_in"`
	RefreshExp   int     `json:"refresh_expires_in"`
	TokenType    string  `json:"token_type"`
	ExpiresAt    string  `json:"expires_at"`
}

type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope,omitempty"`
}

func NewAuthService(userRepo ports.UserRepository, idp ports.IdentityProvider) AuthService {
	return &authService{idp: idp, userRepo: userRepo}
}

func (s *authService) Login(email, password string) (*AuthResponse, error) {
	ctx := context.Background()
	token, err := s.idp.Login(ctx, email, password)
	if err != nil {
		return nil, err
	}

	userInfo, err := s.idp.GetUserInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339)

	return &AuthResponse{
		UserID:       userInfo.Sub,
		Email:        userInfo.Email,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    token.ExpiresIn,
		RefreshExp:   token.RefreshExpiresIn,
		TokenType:    token.TokenType,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *authService) Logout(token string) error {
	return s.idp.Logout(context.Background(), token)
}

func (s *authService) GetUserInfo(token string) (*ports.UserInfo, error) {
	return s.idp.GetUserInfo(context.Background(), token)
}

func (s *authService) RefreshToken(refreshToken string) (*TokenResponse, error) {
	token, err := s.idp.RefreshToken(context.Background(), refreshToken)
	if err != nil {
		return nil, err
	}
	return &TokenResponse{
		AccessToken:      token.AccessToken,
		RefreshToken:     token.RefreshToken,
		ExpiresIn:        token.ExpiresIn,
		RefreshExpiresIn: token.RefreshExpiresIn,
		TokenType:        token.TokenType,
	}, nil
}

// parseJWTClaims разбирает payload JWT для извлечения полей.
// ВАЖНО: не проверяет подпись — вызывающий код обязан сначала вызвать VerifyToken
// (как это делает ensureUserFromJWT).
func parseJWTClaims(token string) (sub, email, firstName, lastName string, exp int64, emailVerified bool, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		err = fmt.Errorf("malformed JWT")
		return
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		err = e
		return
	}
	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Exp           int64  `json:"exp"`
		EmailVerified bool   `json:"email_verified"`
	}
	if e := json.Unmarshal(payload, &claims); e != nil {
		err = e
		return
	}
	if claims.Sub == "" {
		err = fmt.Errorf("missing sub claim")
		return
	}
	if time.Now().Unix() > claims.Exp {
		err = fmt.Errorf("token expired")
		return
	}
	sub, email, firstName, lastName, exp, emailVerified = claims.Sub, claims.Email, claims.GivenName, claims.FamilyName, claims.Exp, claims.EmailVerified
	return
}

func (s *authService) ValidateToken(token string) error {
	return s.ensureUserFromJWT(token)
}

func (s *authService) ValidateTokenForMiddleware(token string) error {
	return s.ensureUserFromJWT(token)
}

// ensureUserFromJWT проверяет подпись JWT по Keycloak realm, затем разбирает payload
// и создаёт пользователя в БД если его нет.
// Если JWT валидный — всегда возвращает nil по DB-ошибкам (не блокируем запрос из-за БД),
// но невалидная подпись/claims всегда отклоняются.
func (s *authService) ensureUserFromJWT(token string) error {
	if err := s.idp.VerifyToken(context.Background(), token); err != nil {
		log.Printf("JWT signature verification failed: %v", err)
		return fmt.Errorf("token invalid")
	}

	sub, email, firstName, lastName, _, emailVerified, err := parseJWTClaims(token)
	if err != nil {
		log.Printf("JWT parse failed: %v", err)
		return fmt.Errorf("token invalid")
	}

	userId, err := uuid.Parse(sub)
	if err != nil {
		return fmt.Errorf("invalid sub in token")
	}

	// Пробуем найти или создать пользователя, но не блокируем запрос при DB-ошибке
	_, dbErr := s.userRepo.GetUserById(userId)
	if dbErr != nil {
		if dbErr.Error() == "user not found" {
			isActive := true
			ev := emailVerified
			userCreateReq := request.UserCreateRequest{
				Email:         &email,
				FirstName:     &firstName,
				LastName:      &lastName,
				IsActive:      &isActive,
				EmailVerified: &ev,
			}
			if createErr := s.userRepo.CreateUserWithID(userCreateReq, userId); createErr != nil {
				log.Printf("Failed to create user (non-fatal): %v", createErr)
				// Не возвращаем ошибку — JWT валидный, запрос разрешаем
			}
		} else {
			log.Printf("DB error in ensureUserFromJWT (non-fatal): %v", dbErr)
			// Не возвращаем ошибку — JWT валидный, запрос разрешаем
		}
	}
	return nil
}
