package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	time.Sleep(5 * time.Second)

	// Создаём тестовую БД один раз перед всеми тестами
	createTestDatabase()

	// Запускаем все тесты
	code := m.Run()

	// (опционально) можно удалить БД после — но обычно не нужно
	// dropTestDatabase()

	os.Exit(code)
}

func createTestDatabase() {
	host := getEnv("TEST_DB_HOST", "localhost")
	port := getEnv("TEST_DB_PORT", "5432")
	user := getEnv("TEST_DB_USER", "postgres")
	password := getEnv("TEST_DB_PASSWORD", "admin")
	dbname := getEnv("TEST_DB_NAME", "postgres")

	adminDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		host, user, password, dbname, port)
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		panic("Failed to connect to admin DB: " + err.Error())
	}
	defer func() {
		if sqlDB, err := adminDB.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	// Check whether the test database exists.
	var exists bool
	err = adminDB.Raw("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'test')").Scan(&exists).Error
	if err != nil {
		panic("Failed to check if test DB exists: " + err.Error())
	}

	if !exists {
		err = adminDB.Exec("CREATE DATABASE test").Error
		if err != nil {
			panic("Failed to create test DB: " + err.Error())
		}
	}
}

func setupTestDB(t *testing.T) *gorm.DB {
	host := getEnv("TEST_DB_HOST", "localhost")
	port := getEnv("TEST_DB_PORT", "5432")
	user := getEnv("TEST_DB_USER", "postgres")
	password := getEnv("TEST_DB_PASSWORD", "admin")

	testDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=test port=%s sslmode=disable TimeZone=UTC",
		host, user, password, port)

	db, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: logger.Discard,
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Team{},
		&models.Problem{},
		&models.UserRole{},
		&models.TeamMember{},
		&models.ForumMessage{},
		&models.Project{},
		&models.Board{},
		&models.Status{},
		&models.Task{},
		&models.Attendance{},
		&models.DailyReport{},
		&models.ReportProblem{},
		&models.HelpRequest{},
		&models.CompletedWork{},
		&models.TomorrowPlans{},
		&models.ProjectTeam{},
		&models.Subscription{},
		&models.AcceptanceCriterion{},
		&models.Evidence{},
		&models.ConveyorEvent{},
		&models.TaskLink{},
		&models.WorkOrder{},
		&models.AgentRun{},
		&models.IdempotencyRecord{},
		&models.GeneratedReport{},
		&models.ForumDigest{},
		&models.ForumActionCandidate{},
		&models.Waiver{},
		&models.ApprovalRequest{},
	)
	require.NoError(t, err)

	return db
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func testFreshAvatarURL(raw string) string {
	return raw
}
