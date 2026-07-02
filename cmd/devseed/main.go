// devseed creates a demo user + admin role + emplacc_* API token and a sample
// project/board/task with Conveyor criteria, evidence and events, then prints a
// ready-to-use access blob (token + a browser console snippet that signs the web
// UI in past AuthGate without Keycloak). Idempotent: deterministic UUIDs mean
// re-running upserts rather than duplicating. Demo-only.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	models "emplacc-api/internal/domain"
	pgrepo "emplacc-api/internal/repository/postgres"
	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var ns = uuid.NewSHA1(uuid.NameSpaceURL, []byte("conveyor:demo"))

func did(s string) uuid.UUID { return uuid.NewSHA1(ns, []byte(s)) }

func bptr(b bool) *bool           { return &b }
func sptr(s string) *string       { return &s }
func tptr(t time.Time) *time.Time { return &t }

func main() {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		env("DB_HOST", "db"), env("DB_USER", "postgres"), env("DB_PASS", "admin"),
		env("DB_NAME", "emplacc"), env("DB_PORT", "5432"), env("DB_SSLMODE", "disable"), env("DB_TIMEZONE", "UTC"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("devseed: db connect: %v", err)
	}

	now := time.Now()

	roleID := did("role:admin")
	upsert(db, &models.Role{ID: roleID, Name: sptr("admin"), Deleted: bptr(false), CreatedAt: tptr(now), UpdatedAt: tptr(now)}, "id = ?", roleID)

	userID := did("user:dev")
	upsert(db, &models.User{ID: userID, Email: "dev@local", IsActive: true, EmailVerified: true, FirstName: "Demo", LastName: "User", LastLogin: now, Deleted: false, CreatedAt: now, UpdatedAt: now}, "id = ?", userID)

	var urCount int64
	db.Model(&models.UserRole{}).Where("user_id = ? AND role_id = ?", userID, roleID).Count(&urCount)
	if urCount == 0 {
		if err := db.Create(&models.UserRole{RoleID: roleID, UserID: userID, AssignedAt: tptr(now), Deleted: bptr(false), CreatedAt: tptr(now), UpdatedAt: tptr(now)}).Error; err != nil {
			log.Fatalf("devseed: user role: %v", err)
		}
	}

	projectID := did("project")
	upsert(db, &models.Project{ID: projectID, Name: sptr("Conveyor Demo"), Deleted: bptr(false), CreatedAt: tptr(now), UpdatedAt: tptr(now)}, "id = ?", projectID)
	boardID := did("board")
	upsert(db, &models.Board{ID: boardID, ProjectID: projectID, Name: sptr("default"), Deleted: bptr(false), CreatedAt: tptr(now), UpdatedAt: tptr(now)}, "id = ?", boardID)

	statuses := []struct {
		key  string
		open bool
	}{{"draft", true}, {"ready", true}, {"in_progress", true}, {"blocked", true}, {"review", true}, {"done", false}}
	statusID := map[string]uuid.UUID{}
	for _, s := range statuses {
		id := did("status:" + s.key)
		statusID[s.key] = id
		upsert(db, &models.Status{ID: id, BoardID: boardID, Name: sptr(s.key), IsOpen: bptr(s.open), Deleted: bptr(false)}, "id = ?", id)
	}

	taskID := did("task")
	upsert(db, &models.Task{ID: taskID, Name: sptr("Add healthcheck endpoint"), Description: sptr("Need /healthz for post-deploy checks."), CreatedBy: &userID, AssignedTo: &userID, StatusID: statusID["in_progress"], Deleted: bptr(false), CreatedAt: tptr(now), UpdatedAt: tptr(now)}, "id = ?", taskID)

	c1, c2 := did("crit:1"), did("crit:2")
	upsert(db, &models.AcceptanceCriterion{ID: c1, TaskID: taskID, ACID: "AC-1", Title: "GET /healthz returns 200", Required: true, State: models.AcceptanceCriterionStatePassed, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}, "id = ?", c1)
	upsert(db, &models.AcceptanceCriterion{ID: c2, TaskID: taskID, ACID: "AC-2", Title: "Response includes app version", Required: true, State: models.AcceptanceCriterionStateUnchecked, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}, "id = ?", c2)

	e1, e2 := did("ev:1"), did("ev:2")
	upsert(db, &models.Evidence{ID: e1, TaskID: taskID, Type: models.EvidenceTypePR, Verdict: models.EvidenceVerdictSupports, URI: "https://example.test/pr/42", Title: "PR #42 healthcheck", CreatedBy: userID, CreatedAt: now}, "id = ?", e1)
	upsert(db, &models.Evidence{ID: e2, TaskID: taskID, Type: models.EvidenceTypeLog, Verdict: models.EvidenceVerdictContradicts, URI: "https://example.test/log/7", Title: "Smoke test failed once", CreatedBy: userID, CreatedAt: now}, "id = ?", e2)

	ev1, ev2 := did("evt:1"), did("evt:2")
	upsert(db, &models.ConveyorEvent{ID: ev1, WorkItemID: taskID, Type: "criterion.passed", Timestamp: now, ActorType: "user", ActorID: userID, Payload: json.RawMessage(`{"criterion":"AC-1"}`), SchemaVersion: 1, Source: "devseed"}, "id = ?", ev1)
	upsert(db, &models.ConveyorEvent{ID: ev2, WorkItemID: taskID, Type: "evidence.attached", Timestamp: now, ActorType: "user", ActorID: userID, Payload: json.RawMessage(`{"evidence":"PR #42"}`), SchemaVersion: 1, Source: "devseed"}, "id = ?", ev2)

	tokenSvc := service.NewAPITokenService(pgrepo.NewAPITokenRepository(db), pgrepo.NewUserRepository(db))
	tok, err := tokenSvc.Create(userID, "demo", nil)
	if err != nil {
		log.Fatalf("devseed: api token: %v", err)
	}

	exp := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	fmt.Println("\n================ CONVEYOR DEMO READY ================")
	fmt.Printf("API token : %s\n", tok.PlainText)
	fmt.Printf("User ID   : %s\n", userID)
	fmt.Printf("Task ID   : %s\n", taskID)
	fmt.Printf("Project   : %s   Board: %s\n", projectID, boardID)
	fmt.Println("\nWeb:  http://localhost:3002   (click the green \"Demo-вход\" button — no Keycloak, no snippet)")
	fmt.Println("API:  http://localhost:8081/swagger/index.html")
	fmt.Println("\n--- Fallback: paste this in the browser console at http://localhost:3002 to sign in ---")
	fmt.Printf("localStorage.setItem('sess_token','%s');localStorage.setItem('sess_user_id','%s');localStorage.setItem('sess_expires_at','%s');localStorage.setItem('sess_absolute_expires_at','%s');location.href='/tasks/%s';\n", tok.PlainText, userID, exp, exp, taskID)
	fmt.Println("\n--- Or verify the API directly ---")
	fmt.Printf("curl -s http://localhost:8081/api/work-items/%s/criteria -H 'Authorization: Bearer %s'\n", taskID, tok.PlainText)
	fmt.Printf("curl -s http://localhost:8081/api/work-items/%s/evidence  -H 'Authorization: Bearer %s'\n", taskID, tok.PlainText)
	fmt.Println("====================================================")
}

func upsert(db *gorm.DB, model interface{}, where string, args ...interface{}) {
	var count int64
	if err := db.Model(model).Where(where, args...).Count(&count).Error; err != nil {
		log.Fatalf("devseed: count: %v", err)
	}
	if count > 0 {
		return
	}
	if err := db.Create(model).Error; err != nil {
		log.Fatalf("devseed: create: %v", err)
	}
}
