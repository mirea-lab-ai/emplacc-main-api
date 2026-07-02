package db

import (
	models "emplacc-api/internal/domain"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB_conn *gorm.DB = GetDBConnection()

func GetDBConnection() *gorm.DB {
	time.Sleep(5 * time.Second)

	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASS")
	name := os.Getenv("DB_NAME")
	sslMode := os.Getenv("DB_SSLMODE")
	timezone := os.Getenv("DB_TIMEZONE")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		host, user, pass, name, port, sslMode, timezone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatal("Failed to connect to database", err)
	}

	// Get the raw database handle for connection pool settings.
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Failed to get *sql.DB from GORM:", err)
	}

	// Connection pool settings.
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(
		// Base entities with no foreign keys or optional foreign keys.
		&models.User{},
		&models.Role{},
		&models.Team{},
		&models.Problem{},

		// Join tables and entities that reference base entities.
		&models.UserRole{},
		&models.TeamMember{},
		&models.ForumMessage{}, // references Problem and User.
		&models.Project{},      // references User through CreatedBy.

		// Second-level entities.
		&models.Board{}, // references Project.

		// Statuses reference Board.
		&models.Status{},

		// Tasks reference Status and User.
		&models.Task{},

		// Reports and related task or user entities.
		&models.Attendance{},
		&models.DailyReport{},
		&models.ReportProblem{},
		&models.HelpRequest{},
		&models.CompletedWork{},
		&models.TomorrowPlans{},
		&models.ProjectTeam{}, // references Project and Team.
		&models.Subscription{},
		&models.APIToken{},
		&models.LLMSettings{},
		&models.AcceptanceCriterion{},
		&models.Evidence{},
		&models.ConveyorEvent{},
		&models.TaskLink{},
		&models.WorkOrder{},
		&models.AgentRun{},
		&models.AgentInboxItem{},
		&models.IdempotencyRecord{},
		&models.GeneratedReport{},
		&models.ForumDigest{},
		&models.ForumActionCandidate{},
		&models.Waiver{},
		&models.ApprovalRequest{},
		&models.CodeRepository{}, // git commit-tracker
		&models.Commit{},
		&models.Notification{}, // in-app notifications
		&models.UserAlias{},    // приватные пер-юзер псевдонимы
		// Session is stored in Redis, not PostgreSQL.
	); err != nil {
		log.Fatal("AutoMigrate failed:", err)
	}

	if err := SetupFullTextSearch(db); err != nil {
		log.Printf("Warning: Full-Text Search setup had issues: %v", err)
	}

	return db
}
