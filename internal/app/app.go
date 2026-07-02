// Package app is the COMPOSITION ROOT of the service — the single place that knows
// about every concrete dependency: it constructs adapters, hides them behind ports,
// injects them into services, and wires services into HTTP handlers. Nothing else in
// the codebase imports across layers for construction; that responsibility lives here.
//
// main.go stays thin: load timezone, call Bootstrap, start the server, Close on exit.
package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	docs "emplacc-api/docs"
	"emplacc-api/internal/db"
	"emplacc-api/internal/grpc/client"
	infragit "emplacc-api/internal/infra/git"
	"emplacc-api/internal/infra/keycloak"
	"emplacc-api/internal/infra/mail"
	"emplacc-api/internal/infra/storage"
	"emplacc-api/internal/repository/postgres"
	redisrepo "emplacc-api/internal/repository/redis"
	"emplacc-api/internal/service"
	httpapi "emplacc-api/internal/transport/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// App is the bootstrapped, ready-to-serve application: a configured Echo instance
// plus the cleanup hooks that release infrastructure handles on shutdown.
type App struct {
	Echo    *echo.Echo
	cleanup []func()
}

// Close runs cleanup hooks in reverse construction order.
func (a *App) Close() {
	for i := len(a.cleanup) - 1; i >= 0; i-- {
		a.cleanup[i]()
	}
}

// Bootstrap builds the entire dependency graph and returns a runnable App.
func Bootstrap() (*App, error) {
	app := &App{}

	e := echo.New()
	// ── Recovery middleware — поймать панику перед логированием ──
	e.Use(middleware.Recover())
	e.Use(middleware.RemoveTrailingSlash())
	e.Use(middleware.Logger())

	// Разрешённые origin'ы: дефолтные + из env CORS_ALLOW_ORIGINS (через запятую) —
	// для white-label деплоев (напр. mytask.trusted-ai.ru) без пересборки логики.
	allowOrigins := []string{
		"https://emplacc.g-309.ru",
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:3002",
	}
	for _, o := range strings.Split(os.Getenv("CORS_ALLOW_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowOrigins = append(allowOrigins, o)
		}
	}

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowOrigins,
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS, echo.PATCH},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
		},
		AllowCredentials: true,
	}))

	dbConn := db.DB_conn

	llmHost := os.Getenv("LLM_HOST")
	llmAddr := llmHost + ":50051"
	llmClient, err := client.NewLLMClient(llmAddr, os.Getenv("LLM_GRPC_AUTH_TOKEN"))
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}
	app.cleanup = append(app.cleanup, func() { llmClient.Close() })

	// ── Repositories + services ──
	userRepo := postgres.NewUserRepository(dbConn)
	authService := service.NewAuthService(userRepo, keycloak.New())
	notificationService := service.NewNotificationService(postgres.NewNotificationRepository(dbConn), userRepo, mail.New())
	boardRepo := postgres.NewBoardRepository(dbConn)
	boardService := service.NewBoardService(boardRepo)
	reportRepo := postgres.NewReportRepository(dbConn)
	reportService := service.NewReportService(reportRepo)
	forumMessageRepo := postgres.NewForumMessageRepository(dbConn)
	teamRepo := postgres.NewTeamRepository(dbConn)
	forumMessageService := service.NewForumMessageService(forumMessageRepo, notificationService, teamRepo)
	problemRepo := postgres.NewProblemRepository(dbConn)
	projectRepo := postgres.NewProjectRepository(dbConn)
	projectService := service.NewProjectService(projectRepo)
	attendanceRepo := postgres.NewAttendanceRepository(dbConn)
	attendanceService := service.NewAttendanceService(attendanceRepo)
	roleRepo := postgres.NewRoleRepository(dbConn)
	roleService := service.NewRoleService(roleRepo)
	statusRepo := postgres.NewStatusRepository(dbConn)
	statusService := service.NewStatusService(statusRepo)
	subscriptionRepo := postgres.NewSubscriptionRepository(dbConn)
	subscriptionService := service.NewSubscriptionService(subscriptionRepo)
	taskRepo := postgres.NewTaskRepository(dbConn)
	taskService := service.NewTaskService(taskRepo, notificationService)
	conveyorRepo := postgres.NewConveyorRepository(dbConn)
	conveyorService := service.NewConveyorServiceWithReportLLM(conveyorRepo, service.NewBackendGeneratedReportLLMClient(llmClient))
	pmImportService := service.NewPMImportService(conveyorRepo)
	teamService := service.NewTeamService(teamRepo)
	userService := service.NewUserService(userRepo)
	userAliasService := service.NewUserAliasService(postgres.NewUserAliasRepository(dbConn))
	apiTokenRepo := postgres.NewAPITokenRepository(dbConn)
	apiTokenService := service.NewAPITokenService(apiTokenRepo, userRepo)
	llmSettingsService := service.NewLLMSettingsService(postgres.NewLLMSettingsRepository(dbConn))

	// Git commit-tracker: host-agnostic (порт CommitProvider) + адаптеры GitHub/GitFlic.
	gitRepo := postgres.NewGitRepository(dbConn)
	gitService := service.NewGitService(gitRepo, userRepo,
		infragit.NewGitHubProvider(os.Getenv("GITHUB_TOKEN")),
		infragit.NewGitFlicProvider(os.Getenv("GITFLIC_TOKEN"), os.Getenv("GITFLIC_API_URL")),
	)

	// Redis — сессии без персистентности на диск
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	log.Printf("Redis connected: %s", redisAddr)
	app.cleanup = append(app.cleanup, func() { _ = rdb.Close() })

	sessionRepo := redisrepo.NewSessionRepository(rdb)
	sessionService := service.NewSessionService(sessionRepo)

	// SSE realtime: in-memory шина событий; conveyor.createEvent публикует в неё через GlobalEventHub.
	eventHub := service.NewEventHub()
	service.GlobalEventHub = eventHub

	systemUserId, err := userService.CreateSystemUser()
	if err != nil {
		return nil, fmt.Errorf("create system user: %w", err)
	}

	problemService := service.NewProblemService(problemRepo, forumMessageRepo, systemUserId)

	// S3/RustFS storage (опционально — не падаем если не настроен)
	storageService, storageErr := storage.New()
	if storageErr != nil {
		log.Printf("Warning: S3 storage not configured: %v", storageErr)
	}

	// Swagger: не хардкодим host, оставляем пустым, чтобы UI брал текущий адрес запроса
	docs.SwaggerInfo.Host = ""

	// ── Главный middleware ПЕРЕД маршрутами — критический порядок ──
	e.Use(httpapi.AppAuthMiddleware(authService, sessionService, apiTokenService))

	// Role-based middleware — три уровня доступа
	adminMw := httpapi.RequireRoles(roleRepo, "admin")
	managerMw := httpapi.RequireRoles(roleRepo, "admin", "manager")
	employeeMw := httpapi.RequireRoles(roleRepo, "admin", "manager", "employee")

	// freshAvatarURL — генерация свежих presigned URL; деградирует gracefully без storage
	freshAvatarURL := func(s string) string { return s }
	if storageService != nil {
		freshAvatarURL = storageService.FreshAvatarURL
	}

	// mountAPI регистрирует весь JSON-API на переданном роутере. Вызывается дважды:
	// на корне (v1, сырые ответы) и на группе /v2 (обёрнутые в {data,error,meta}).
	mountAPI := func(r httpapi.Router) {
		httpapi.RegisterVersionRoutes(r)
		httpapi.RegisterAuthRoutes(r, authService, sessionService, userService)
		httpapi.RegisterUserRoutes(r, userService, freshAvatarURL, adminMw)
		httpapi.RegisterUserAliasRoutes(r, userAliasService, freshAvatarURL, employeeMw)
		httpapi.RegisterRoleRoutes(r, roleService, adminMw)
		httpapi.RegisterTeamRoutes(r, teamService, freshAvatarURL, managerMw)
		httpapi.RegisterProjectRoutes(r, projectService, managerMw)
		httpapi.RegisterBoardRoutes(r, boardService, managerMw)
		httpapi.RegisterStatusRoutes(r, statusService, managerMw)
		httpapi.RegisterTaskRoutes(r, taskService, userService, projectService, llmClient, conveyorService, llmSettingsService, freshAvatarURL, employeeMw, managerMw)
		httpapi.RegisterConveyorRoutes(r, conveyorService, pmImportService, employeeMw, managerMw)
		httpapi.RegisterLLMSettingsRoutes(r, llmSettingsService, adminMw)
		httpapi.RegisterReportRoutes(r, reportService, freshAvatarURL, employeeMw, managerMw)
		httpapi.RegisterForumMessagesRoutes(r, forumMessageService, freshAvatarURL, employeeMw, managerMw)
		httpapi.RegisterProblemRoutes(r, problemService, employeeMw, managerMw)
		httpapi.RegisterAttendanceRoutes(r, attendanceService, employeeMw, managerMw)
		httpapi.RegisterSubscriptionRoutes(r, subscriptionService)
		httpapi.RegisterAPITokenRoutes(r, apiTokenService)
		httpapi.RegisterGitRoutes(r, gitService, employeeMw, managerMw)
		httpapi.RegisterNotificationRoutes(r, notificationService, employeeMw)
		if storageService != nil {
			httpapi.RegisterUploadRoutes(r, storageService, userService)
		}
	}

	// v1 — сырые ответы (обратная совместимость с текущим фронтом)
	mountAPI(e)
	// v2 — те же ручки под /v2, ответы нормализованы в {data,error,meta}
	mountAPI(e.Group("/v2", httpapi.EnvelopeMiddleware))

	// SSE realtime stream (/v2/stream) — auth по query-токену, см. Stream.go.
	// Регистрируется на корне (НЕ под enveloped-группой — это поток, не JSON).
	httpapi.RegisterStreamRoutes(e, eventHub, sessionService, apiTokenService)

	// Git commit-tracker (репозитории/коммиты/привязка к задачам)
	httpapi.RegisterGitRoutes(e, gitService, employeeMw, managerMw)

	// Swagger UI
	e.GET("/swagger", func(c echo.Context) error {
		return c.Redirect(http.StatusTemporaryRedirect, "/swagger/index.html")
	})
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// Preflight OPTIONS для вебхука
	e.OPTIONS("/event", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	app.Echo = e
	return app, nil
}
