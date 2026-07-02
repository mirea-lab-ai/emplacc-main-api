package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	models "emplacc-api/internal/domain"
	pgrepo "emplacc-api/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConveyorPostgresConcurrentIdempotencyReplay(t *testing.T) {
	dsn := os.Getenv("CONVEYOR_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("CONVEYOR_POSTGRES_DSN is required for PostgreSQL integration coverage")
	}
	if os.Getenv("CONVEYOR_POSTGRES_ALLOW_SCHEMA_RESET") != "1" {
		t.Skip("CONVEYOR_POSTGRES_ALLOW_SCHEMA_RESET=1 is required because this test resets the public schema")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)

	require.NoError(t, db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error)
	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.Board{},
		&models.Status{},
		&models.Task{},
		&models.AcceptanceCriterion{},
		&models.Evidence{},
		&models.ConveyorEvent{},
		&models.TaskLink{},
		&models.AgentRun{},
		&models.IdempotencyRecord{},
	))

	actorID := uuid.New()
	projectID := uuid.New()
	boardID := uuid.New()
	statusID := uuid.New()
	taskID := uuid.New()
	now := time.Now().UTC()
	deleted := false
	open := true
	name := "Integration task"
	projectName := "Integration project"
	boardName := "Integration board"
	statusName := "Open"
	priority := int16(1)

	require.NoError(t, db.Create(&models.User{ID: actorID, Email: "actor@example.test", IsActive: true, CreatedAt: now}).Error)
	require.NoError(t, db.Create(&models.Project{ID: projectID, Name: &projectName, CreatedBy: &actorID, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)
	require.NoError(t, db.Create(&models.Board{ID: boardID, ProjectID: projectID, Name: &boardName, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)
	require.NoError(t, db.Create(&models.Status{ID: statusID, BoardID: boardID, Name: &statusName, IsOpen: &open, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)
	require.NoError(t, db.Create(&models.Task{ID: taskID, Name: &name, Priority: &priority, StatusID: statusID, CreatedBy: &actorID, AssignedTo: &actorID, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)

	barrier := newTwoPartyBarrier(2, 5*time.Second)
	repo := &barrierConveyorRepository{ConveyorRepository: pgrepo.NewConveyorRepository(db), barrier: barrier}
	svc := NewConveyorService(repo)
	actor := ConveyorActor{ActorType: "user", ActorID: actorID, Source: "test"}
	req := CreateAcceptanceCriterionRequest{TaskID: taskID, Title: "Concurrent criterion", Required: true, IdempotencyKey: "same-key"}

	start := make(chan struct{})
	type callResult struct {
		result *ConveyorMutationResult
		err    error
	}
	results := make(chan callResult, 2)
	for range 2 {
		go func() {
			<-start
			result, err := svc.CreateAcceptanceCriterion(context.Background(), actor, req)
			results <- callResult{result: result, err: err}
		}()
	}
	close(start)

	first := <-results
	second := <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.result)
	require.NotNil(t, second.result)
	require.Equal(t, first.result.EntityID, second.result.EntityID)
	require.Equal(t, first.result.EventID, second.result.EventID)
	require.True(t, first.result.Replayed != second.result.Replayed, "exactly one concurrent caller should receive replay")

	var criteriaCount int64
	require.NoError(t, db.Model(&models.AcceptanceCriterion{}).Count(&criteriaCount).Error)
	require.Equal(t, int64(1), criteriaCount)
	var eventsCount int64
	require.NoError(t, db.Model(&models.ConveyorEvent{}).Count(&eventsCount).Error)
	require.Equal(t, int64(1), eventsCount)
	var idempotencyCount int64
	require.NoError(t, db.Model(&models.IdempotencyRecord{}).Count(&idempotencyCount).Error)
	require.Equal(t, int64(1), idempotencyCount)
}

func TestPMImportPostgresRollsBackLateTaskLinkConflict(t *testing.T) {
	dsn := os.Getenv("CONVEYOR_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("CONVEYOR_POSTGRES_DSN is required for PostgreSQL integration coverage")
	}
	if os.Getenv("CONVEYOR_POSTGRES_ALLOW_SCHEMA_RESET") != "1" {
		t.Skip("CONVEYOR_POSTGRES_ALLOW_SCHEMA_RESET=1 is required because this test resets the public schema")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)

	require.NoError(t, db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error)
	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.Board{},
		&models.Status{},
		&models.Task{},
		&models.AcceptanceCriterion{},
		&models.Evidence{},
		&models.ConveyorEvent{},
		&models.TaskLink{},
		&models.AgentRun{},
		&models.IdempotencyRecord{},
	))

	actorID := uuid.New()
	projectID := uuid.New()
	boardID := uuid.New()
	statusID := uuid.New()
	now := time.Now().UTC()
	deleted := false
	open := true
	projectName := "PM import integration project"
	boardName := "PM import integration board"
	statusName := "TODO"
	require.NoError(t, db.Create(&models.User{ID: actorID, Email: "pm-import-actor@example.test", IsActive: true, CreatedAt: now}).Error)
	require.NoError(t, db.Create(&models.Project{ID: projectID, Name: &projectName, CreatedBy: &actorID, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)
	require.NoError(t, db.Create(&models.Board{ID: boardID, ProjectID: projectID, Name: &boardName, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)
	require.NoError(t, db.Create(&models.Status{ID: statusID, BoardID: boardID, Name: &statusName, IsOpen: &open, Deleted: &deleted, CreatedAt: &now, UpdatedAt: &now}).Error)

	root := writePMImportFixture(t, map[string]string{
		"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tTODO\tImport PM ledger\tdev\tdefault\tdocs/spec/runbooks/conveyor-pm-import.md\t2026-06-16T00:00:00Z\t2026-06-16T01:00:00Z\tPM-2\tpm,import\nPM-2\tTODO\tSource task\tdev\tdefault\t\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
		"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\nPM-1\tAC-1\ttrue\tDry-run reports counts\tSPEC-CV-PMIMPORT-001\n",
		"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\nPM-1\t2026-06-16\tlink\thttps://example.test/evidence\tImport evidence\n",
		"pulse.log":    "2026-06-16T00:00:00Z\tCREATED\tPM-1 Created from PM canon\n",
		"status.md":    "# Status\n\nPM import integration fixture status.\n",
	})

	conflictLinkID := PMImportStableID("link", "PM-2:blocks:PM-1")
	preExistingConflict := models.TaskLink{ID: conflictLinkID, SourceTaskID: uuid.New(), TargetTaskID: uuid.New(), LinkType: models.TaskLinkTypeRelatesTo, CreatedBy: actorID, CreatedAt: now}
	require.NoError(t, db.Create(&preExistingConflict).Error)

	repo := pgrepo.NewConveyorRepository(db)
	svc := NewPMImportService(repo)
	actor := ConveyorActor{ActorType: "user", ActorID: actorID, Source: "pm-import-test"}
	req := PMImportRequest{RootPath: root, StatusByPMState: map[string]uuid.UUID{"TODO": statusID}}

	_, err = svc.ImportPMCanon(context.Background(), actor, req)
	require.Error(t, err)

	require.Equal(t, int64(0), postgresCountByID(t, db, &models.Task{}, PMImportStableID("ticket", "PM-1")))
	require.Equal(t, int64(0), postgresCountByID(t, db, &models.Task{}, PMImportStableID("ticket", "PM-2")))
	require.Equal(t, int64(0), postgresCountByID(t, db, &models.AcceptanceCriterion{}, PMImportStableID("criterion", "PM-1:AC-1")))
	require.Equal(t, int64(0), postgresCountByID(t, db, &models.Evidence{}, PMImportStableID("evidence", "PM-1:1")))
	require.Equal(t, int64(0), postgresCountByID(t, db, &models.ConveyorEvent{}, PMImportStableID("event", "pulse:1:PM-1:CREATED")))
	require.Equal(t, int64(1), postgresCountByID(t, db, &models.TaskLink{}, conflictLinkID))

	require.NoError(t, db.Delete(&models.TaskLink{}, "id = ?", conflictLinkID).Error)
	summary, err := svc.ImportPMCanon(context.Background(), actor, req)
	require.NoError(t, err)
	require.Equal(t, 2, summary.Tickets.Created)
	require.Equal(t, 1, summary.Criteria.Created)
	require.Equal(t, 1, summary.Evidence.Created)
	require.Equal(t, 1, summary.Events.Created)
	require.Equal(t, 1, summary.Links.Created)
	require.Equal(t, int64(1), postgresCountByID(t, db, &models.TaskLink{}, conflictLinkID))
}

func postgresCountByID(t *testing.T, db *gorm.DB, model any, id uuid.UUID) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(model).Where("id = ?", id).Count(&count).Error)
	return count
}

type twoPartyBarrier struct {
	parties int
	timeout time.Duration
	mu      sync.Mutex
	count   int
	once    sync.Once
	release chan struct{}
}

func newTwoPartyBarrier(parties int, timeout time.Duration) *twoPartyBarrier {
	return &twoPartyBarrier{parties: parties, timeout: timeout, release: make(chan struct{})}
}

func (b *twoPartyBarrier) wait() error {
	b.mu.Lock()
	b.count++
	if b.count == b.parties {
		b.once.Do(func() { close(b.release) })
	}
	b.mu.Unlock()

	select {
	case <-b.release:
		return nil
	case <-time.After(b.timeout):
		return fmt.Errorf("timed out waiting for concurrent idempotency barrier")
	}
}

type barrierConveyorRepository struct {
	ConveyorRepository
	barrier *twoPartyBarrier
}

func (r *barrierConveyorRepository) WithTransaction(ctx context.Context, fn func(ConveyorRepository) error) error {
	return r.ConveyorRepository.WithTransaction(ctx, func(txRepo ConveyorRepository) error {
		return fn(&barrierConveyorRepository{ConveyorRepository: txRepo, barrier: r.barrier})
	})
}

func (r *barrierConveyorRepository) GetIdempotencyRecord(ctx context.Context, actorID uuid.UUID, operation string, key string) (*models.IdempotencyRecord, error) {
	record, err := r.ConveyorRepository.GetIdempotencyRecord(ctx, actorID, operation, key)
	if operation == "criterion.create" && key == "same-key" && errors.Is(err, ErrNotFound) {
		if waitErr := r.barrier.wait(); waitErr != nil {
			return nil, waitErr
		}
	}
	return record, err
}
