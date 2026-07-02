package test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/repository/postgres"
	"emplacc-api/internal/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestPMImportE2EFromProjectCanon exercises the real .pm -> PostgreSQL migration
// path end to end against the live test database: it stages the project's actual
// `.pm/scopes/default` canon (plus repo status.md) into a temp root and imports
// it through the real repository, then re-imports to prove idempotency. The
// project's `.pm` files are copied, never modified or deleted.
func TestPMImportE2EFromProjectCanon(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Locate the project's PM canon. The test working dir is emplacc-main-api/test,
	// so the monorepo root is two levels up.
	scopeDir := filepath.Join("..", "..", ".pm", "scopes", "default")
	statusMD := filepath.Join("..", "..", "status.md")
	ticketsTSV := filepath.Join(scopeDir, "tickets.tsv")
	ticketsData, err := os.ReadFile(ticketsTSV)
	if err != nil {
		t.Skipf("project .pm canon not found at %s: %v", scopeDir, err)
	}
	// Skip (don't fail) when the canon only has a header and no ticket rows in this
	// checkout — the importer is correct, there's just nothing to import here.
	dataRows := 0
	for _, line := range strings.Split(strings.TrimSpace(string(ticketsData)), "\n")[1:] {
		if strings.TrimSpace(line) != "" {
			dataRows++
		}
	}
	if dataRows == 0 {
		t.Skipf("project .pm canon at %s has no ticket rows", ticketsTSV)
	}

	// Stage a temp root the importer accepts (all five files in one directory),
	// copying the real project files so the originals are untouched.
	root := t.TempDir()
	for src, name := range map[string]string{
		filepath.Join(scopeDir, "tickets.tsv"):  "tickets.tsv",
		filepath.Join(scopeDir, "criteria.tsv"): "criteria.tsv",
		filepath.Join(scopeDir, "evidence.tsv"): "evidence.tsv",
		filepath.Join(scopeDir, "pulse.log"):    "pulse.log",
		statusMD:                                "status.md",
	} {
		data, err := os.ReadFile(src)
		require.NoErrorf(t, err, "read %s", src)
		require.NoError(t, os.WriteFile(filepath.Join(root, name), data, 0o644))
	}

	// Real project/board/statuses so ticket states resolve to actual Status rows.
	projectID := createTestProject(t, db, "Conveyor PM Import E2E")
	boardID := createTestBoard(t, db, projectID, "default")
	userID := createTestUser(t, db, "pm-import-e2e@example.com")

	statusByState := map[string]uuid.UUID{}
	for name, open := range map[string]bool{
		"DRAFT": true, "READY": true, "IN_PROGRESS": true, "BLOCKED": true,
		"REVIEW": true, "DONE": false, "CANCELED": false,
	} {
		statusName := name
		isOpen := open
		deleted := false
		status := models.Status{ID: uuid.New(), BoardID: boardID, Name: &statusName, IsOpen: &isOpen, Deleted: &deleted}
		require.NoError(t, db.Create(&status).Error)
		statusByState[name] = status.ID
	}

	repo := postgres.NewConveyorRepository(db)
	svc := service.NewPMImportService(repo)
	actor := service.ConveyorActor{ActorID: userID, ActorType: "user", Source: "importer"}
	req := service.PMImportRequest{RootPath: root, StatusByPMState: statusByState}

	// First import: every ticket/criterion/evidence row is created.
	summary, err := svc.ImportPMCanon(ctx, actor, req)
	require.NoError(t, err)
	require.Greater(t, summary.Tickets.Total, 0, "project canon should contain tickets")
	require.Equal(t, summary.Tickets.Total, summary.Tickets.Created, "first import creates all tickets")
	require.Equal(t, summary.Criteria.Total, summary.Criteria.Created, "first import creates all criteria")

	// Rows actually landed in PostgreSQL.
	firstTicket := summary.Tickets.Created
	stableID := service.PMImportStableID("ticket", "T-0001")
	var stored models.Task
	require.NoError(t, db.First(&stored, "id = ?", stableID).Error)
	require.NotNil(t, stored.Name)

	var criteriaCount int64
	require.NoError(t, db.Model(&models.AcceptanceCriterion{}).Count(&criteriaCount).Error)
	require.GreaterOrEqual(t, criteriaCount, int64(summary.Criteria.Created))

	// Second import: idempotent — stable IDs mean nothing new is created.
	summary2, err := svc.ImportPMCanon(ctx, actor, req)
	require.NoError(t, err)
	require.Equal(t, 0, summary2.Tickets.Created, "re-import must not duplicate tickets")
	require.Equal(t, 0, summary2.Criteria.Created, "re-import must not duplicate criteria")
	require.Equal(t, 0, summary2.Evidence.Created, "re-import must not duplicate evidence")
	require.Equal(t, firstTicket, summary2.Tickets.Total, "ticket total stays stable")
}
