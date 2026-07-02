package httpapi

import (
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/dto/response"
	"emplacc-api/internal/service"
	utils "emplacc-api/internal/utils"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type AuthController struct {
	authService    service.AuthService
	sessionService service.SessionService
	userService    service.UserService
}

func NewAuthController(authService service.AuthService, sessionService service.SessionService, userService service.UserService) *AuthController {
	return &AuthController{
		authService:    authService,
		sessionService: sessionService,
		userService:    userService,
	}
}

func RegisterAuthRoutes(e Router, authService service.AuthService, sessionService service.SessionService, userService service.UserService) {
	controller := NewAuthController(authService, sessionService, userService)
	authGroup := e.Group("/auth")

	// Старые эндпоинты (оставляем для совместимости)
	authGroup.POST("/login", controller.Login)
	authGroup.POST("/logout", controller.Logout)
	authGroup.GET("/me", controller.Me)
	authGroup.GET("/validate", controller.ValidateToken)
	authGroup.POST("/refresh", controller.RefreshToken)

	// Новые сессионные эндпоинты
	authGroup.POST("/session", controller.CreateSession)        // Keycloak JWT → наш токен
	authGroup.POST("/session/rotate", controller.RotateSession) // продлить сессию
	authGroup.DELETE("/session", controller.ExpireSession)      // выход
}

// Login godoc
// @Summary Аутентификация пользователя
// @Description Аутентифицирует пользователя по email и паролю через Keycloak
// @Tags Auth
// @Accept json
// @Produce json
// @Param login body request.LoginRequest true "Данные для входа"
// @Success 200 {object} response.AuthResponse "Успешная аутентификация"
// @Failure 400 {object} map[string]string "Ошибка в запросе"
// @Failure 401 {object} map[string]string "Неверные учетные данные"
// @Router /auth/login [post]
func (ac *AuthController) Login(c echo.Context) error {
	var req request.LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	authResp, err := ac.authService.Login(req.Email, req.Password)
	if err != nil {
		log.Printf("Authorization error: %v", err)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}

	// Создаём сессионный токен sess_* для консистентности с OAuth flow
	sessionResult, err := ac.sessionService.Create(utils.GetString(authResp.UserID))
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
	}

	// Возвращаем сессионный токен вместо сырых Keycloak токенов
	// Для обратной совместимости добавляем также оригинальные токены
	resp := response.AuthResponse{
		UserID:       utils.GetString(authResp.UserID),
		Email:        utils.GetString(authResp.Email),
		AccessToken:  sessionResult.PlainToken, // sess_* токен вместо access_token
		RefreshToken: authResp.RefreshToken,    // оригинальный refresh token для безопасности
		ExpiresIn:    authResp.ExpiresIn,
		RefreshExp:   authResp.RefreshExp,
		TokenType:    "Bearer",
		ExpiresAt:    sessionResult.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	return c.JSON(http.StatusOK, resp)
}

// Logout godoc
// @Summary Выход из системы
// @Description Выполняет выход пользователя из системы, завершая сессию в Keycloak
// @Tags Auth
// @Accept json
// @Produce json
// @Param Authorization header string true "Refresh token"
// @Security BearerAuth
// @Success 200 {object} map[string]string "Успешный выход из системы"
// @Failure 400 {object} map[string]string "Отсутствует токен"
// @Failure 500 {object} map[string]string "Ошибка сервера при выходе"
// @Router /auth/logout [post]
func (ac *AuthController) Logout(c echo.Context) error {
	token := extractBearerToken(c)
	if token == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing token"})
	}

	if strings.HasPrefix(token, "sess_") {
		_ = ac.sessionService.Expire(token, "user_exit")
		return c.JSON(http.StatusOK, map[string]string{"message": "logout successful"})
	}

	err := ac.authService.Logout(token)
	if err != nil {
		log.Printf("Logout error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "logout failed"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "logout successful"})
}

// Me godoc
// @Summary Получение информации о текущем пользователе
// @Description Возвращает информацию о пользователе на основе переданного токена
// @Tags Auth
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer access token, например: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
// @Security BearerAuth
// @Success 200 {object} response.UserInfo "Информация о пользователе"
// @Failure 401 {object} map[string]string "Отсутствует или неверный токен"
// @Router /auth/me [get]
func (ac *AuthController) Me(c echo.Context) error {
	auth := c.Request().Header.Get("Authorization")
	if auth == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
	}

	token := auth
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("Bearer "):])
	}

	if strings.HasPrefix(token, "sess_") {
		validation, err := ac.sessionService.Validate(token)
		if err != nil || validation.Expired {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		}
		userID, err := uuid.Parse(validation.UserID)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid user_id in session"})
		}
		user, err := ac.userService.GetUserById(userID)
		if err != nil {
			log.Printf("Me (sess_*): user not found: %v", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
		}
		fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		return c.JSON(http.StatusOK, response.UserInfo{
			Sub:               user.ID.String(),
			Name:              fullName,
			PreferredUsername: user.Email,
			GivenName:         user.FirstName,
			FamilyName:        user.LastName,
			Email:             user.Email,
			EmailVerified:     user.EmailVerified,
		})
	}

	userInfo, err := ac.authService.GetUserInfo(token)
	if err != nil {
		log.Printf("Me error: %v", err)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	}

	return c.JSON(http.StatusOK, response.UserInfo{
		Sub:               utils.GetString(userInfo.Sub),
		Name:              utils.GetString(userInfo.Name),
		PreferredUsername: utils.GetString(userInfo.PreferredUsername),
		GivenName:         utils.GetString(userInfo.GivenName),
		FamilyName:        utils.GetString(userInfo.FamilyName),
		Email:             utils.GetString(userInfo.Email),
		EmailVerified:     userInfo.EmailVerified != nil && *userInfo.EmailVerified,
	})
}

// RefreshToken godoc
// @Summary Обновление access токена
// @Description Получение нового access_token и refresh_token на основе существующего refresh_token
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body request.RefreshRequest true "Refresh token"
// @Success 200 {object} response.RefreshResponse "Новые токены и время жизни"
// @Failure 400 {object} map[string]string "Неверный формат запроса"
// @Failure 401 {object} map[string]string "Неверный или истёкший refresh_token"
// @Router /auth/refresh [post]
func (ac *AuthController) RefreshToken(c echo.Context) error {
	var req request.RefreshRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	tokenResp, err := ac.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	}

	now := time.Now()

	newTokens := response.RefreshResponse{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		RefreshExp:   tokenResp.RefreshExpiresIn,
		TokenType:    "Bearer",
		ExpiresAt:    now.Add(time.Duration(300) * time.Second),
	}

	return c.JSON(http.StatusOK, newTokens)
}

// ValidateToken godoc
// @Summary Проверка access_token
// @Description Проверяет валидность токена через Keycloak
// @Tags Auth
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer access token, например: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
// @Success 200 {object} response.TokenValidationResponse "Токен валиден"
// @Failure 401 {object} response.TokenValidationResponse "Невалидный или отсутствующий токен"
// @Router /auth/validate [get]
func (ac *AuthController) ValidateToken(c echo.Context) error {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Invalid Authorization header"})
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	if strings.HasPrefix(token, "sess_") {
		validation, err := ac.sessionService.Validate(token)
		if err != nil || validation.Expired {
			return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Token invalid"})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Token is valid"})
	}

	err := ac.authService.ValidateToken(token)
	if err != nil {
		log.Printf("Token validation error: %v", err)
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Token invalid"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Token is valid"})
}

// CreateSession — обменивает Keycloak JWT на наш сессионный токен
// @Summary Создать сессию
// @Router /auth/session [post]
func (ac *AuthController) CreateSession(c echo.Context) error {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing keycloak token"})
	}
	keycloakToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Валидируем через Keycloak и получаем userID
	err := ac.authService.ValidateToken(keycloakToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "keycloak token invalid"})
	}

	userInfo, err := ac.authService.GetUserInfo(keycloakToken)
	if err != nil || userInfo.Sub == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "cannot get user info"})
	}

	result, err := ac.sessionService.Create(*userInfo.Sub)
	if err != nil {
		log.Printf("CreateSession error: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
	}

	return c.JSON(http.StatusCreated, result)
}

// RotateSession — продлевает сессию (без нового логина через Keycloak)
// @Summary Ротировать сессию
// @Router /auth/session/rotate [post]
func (ac *AuthController) RotateSession(c echo.Context) error {
	token := extractBearerToken(c)
	if token == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
	}

	result, err := ac.sessionService.Rotate(token)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error":  "rotation failed",
			"reason": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, result)
}

// ExpireSession — завершает сессию (выход пользователя)
// @Summary Завершить сессию
// @Router /auth/session [delete]
func (ac *AuthController) ExpireSession(c echo.Context) error {
	token := extractBearerToken(c)
	if token == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing token"})
	}

	_ = ac.sessionService.Expire(token, "user_exit")
	return c.JSON(http.StatusOK, map[string]string{"message": "session expired"})
}

func extractBearerToken(c echo.Context) string {
	h := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
