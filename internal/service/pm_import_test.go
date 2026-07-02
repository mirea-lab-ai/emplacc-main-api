package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPMImportDryRunSummarizesWithoutWritesAndRequiresExplicitRoot(t *testing.T) {
	repo := newFakePMImportRepository()
	actor := testActor()
	root := writePMImportFixture(t, map[string]string{
		"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tIN_PROGRESS\tImport PM ledger\tdev\tdefault\tdocs/spec/runbooks/conveyor-pm-import.md\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
		"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\nPM-1\tAC-1\tfalse\tDry-run reports counts\tSPEC-CV-PMIMPORT-001\n",
		"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\nPM-1\t2026-06-16\tlink\thttps://example.test/evidence\tImport evidence\n",
		"pulse.log":    "2026-06-16T00:00:00Z\tCREATED\tPM-1 Created from PM canon\n",
		"status.md":    "# Status\n\nPM import fixture status.\n",
	})
	statusID := repo.addStatusForPMState("IN_PROGRESS")
	svc := NewPMImportService(repo)

	summary, err := svc.ImportPMCanon(context.Background(), actor, PMImportRequest{RootPath: root, DryRun: true, StatusByPMState: map[string]uuid.UUID{"IN_PROGRESS": statusID}})

	require.NoError(t, err)
	require.True(t, summary.DryRun)
	require.Equal(t, 1, summary.Tickets.Total)
	require.Equal(t, 1, summary.Criteria.Total)
	require.Equal(t, 1, summary.Evidence.Total)
	require.Equal(t, 1, summary.Events.Total)
	require.Empty(t, repo.tasks)
	require.Empty(t, repo.criteria)
	require.Empty(t, repo.evidence)
	require.Empty(t, repo.events)
	require.False(t, summary.SwitchOverAccepted)

	_, err = svc.ImportPMCanon(context.Background(), actor, PMImportRequest{RootPath: filepath.Join(root, "missing"), DryRun: true, StatusByPMState: map[string]uuid.UUID{"IN_PROGRESS": statusID}})
	require.ErrorIs(t, err, ErrValidation)
	require.Contains(t, err.Error(), "pm root")
}

func TestPMImportCreatesWorkItemsAndRelatedFactsWithStableSourceLocators(t *testing.T) {
	repo := newFakePMImportRepository()
	actor := testActor()
	statusID := repo.addStatusForPMState("IN_PROGRESS")
	root := writePMImportFixture(t, map[string]string{
		"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tIN_PROGRESS\tImport PM ledger\tdev\tdefault\tdocs/spec/runbooks/conveyor-pm-import.md\t2026-06-16T00:00:00Z\t2026-06-16T01:00:00Z\tPM-2\tpm,import\nPM-2\tIN_PROGRESS\tSource task\tdev\tdefault\t\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
		"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\nPM-1\tAC-1\ttrue\tDry-run reports counts\tSPEC-CV-PMIMPORT-001\n",
		"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\nPM-1\t2026-06-16\tlink\thttps://example.test/evidence\tImport evidence\n",
		"pulse.log":    "2026-06-16T00:00:00Z\tCREATED\tPM-1 Created from PM canon\n2026-06-16T00:01:00Z\tEVIDENCE\tPM-1 Evidence recorded\n",
		"status.md":    "# Status\n\nCurrent PM status snapshot.\n",
	})
	svc := NewPMImportService(repo)

	summary, err := svc.ImportPMCanon(context.Background(), actor, PMImportRequest{RootPath: root, StatusByPMState: map[string]uuid.UUID{"IN_PROGRESS": statusID}})

	require.NoError(t, err)
	require.False(t, summary.DryRun)
	require.Equal(t, 2, summary.Tickets.Created)
	require.Equal(t, 1, summary.Criteria.Created)
	require.Equal(t, 1, summary.Evidence.Created)
	require.Equal(t, 2, summary.Events.Created)
	require.Equal(t, 1, summary.Links.Created)

	workItemID := PMImportStableID("ticket", "PM-1")
	task := repo.tasks[workItemID]
	require.NotNil(t, task)
	require.Equal(t, statusID, task.StatusID)
	require.NotNil(t, task.Name)
	require.Equal(t, "Import PM ledger", *task.Name)
	require.NotNil(t, task.Description)
	require.Contains(t, *task.Description, "PM Source ID: PM-1")
	require.Contains(t, *task.Description, "tickets.tsv:2")
	require.Contains(t, *task.Description, "status.md")

	criterionID := PMImportStableID("criterion", "PM-1:AC-1")
	require.Equal(t, models.AcceptanceCriterionStatePassed, repo.criteria[criterionID].State)
	require.Contains(t, repo.criteria[criterionID].Title, "Dry-run reports counts")

	evidenceID := PMImportStableID("evidence", "PM-1:1")
	require.Equal(t, "https://example.test/evidence", repo.evidence[evidenceID].URI)
	require.JSONEq(t, `{"source":"pm-import","pm_ticket_id":"PM-1","source_locator":"evidence.tsv:2","kind":"link"}`, string(repo.evidence[evidenceID].Metadata))
	require.ElementsMatch(t, []string{"pm_import.CREATED", "pm_import.EVIDENCE"}, collectEventTypes(repo.events))
}

func TestPMImportRerunReusesExistingRecordsWithoutDuplicates(t *testing.T) {
	repo := newFakePMImportRepository()
	actor := testActor()
	statusID := repo.addStatusForPMState("TODO")
	root := writePMImportFixture(t, map[string]string{
		"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tTODO\tImport PM ledger\tdev\tdefault\t\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
		"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\nPM-1\tAC-1\tfalse\tDry-run reports counts\tSPEC-CV-PMIMPORT-001\n",
		"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\nPM-1\t2026-06-16\tlog\tfile:///tmp/import.log\tImport log\n",
		"pulse.log":    "2026-06-16T00:00:00Z\tSTATE\tPM-1 TODO\n",
		"status.md":    "# Status\n",
	})
	svc := NewPMImportService(repo)
	req := PMImportRequest{RootPath: root, StatusByPMState: map[string]uuid.UUID{"TODO": statusID}}

	first, err := svc.ImportPMCanon(context.Background(), actor, req)
	require.NoError(t, err)
	second, err := svc.ImportPMCanon(context.Background(), actor, req)

	require.NoError(t, err)
	require.Equal(t, 1, first.Tickets.Created)
	require.Equal(t, 1, second.Tickets.Reused)
	require.Equal(t, 1, second.Criteria.Reused)
	require.Equal(t, 1, second.Evidence.Reused)
	require.Equal(t, 1, second.Events.Reused)
	require.Len(t, repo.tasks, 1)
	require.Len(t, repo.criteria, 1)
	require.Len(t, repo.evidence, 1)
	require.Len(t, repo.events, 1)
}

func TestPMImportRejectsMalformedOrUnsafeInputWithoutPartialWrites(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		err   string
	}{
		{
			name: "malformed ticket row",
			files: map[string]string{
				"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tTODO\ttoo few\n",
				"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\n",
				"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\n",
				"pulse.log":    "",
				"status.md":    "# Status\n",
			},
			err: "tickets.tsv:2",
		},
		{
			name: "orphan criterion",
			files: map[string]string{
				"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tTODO\tImport PM ledger\tdev\tdefault\t\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
				"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\nPM-404\tAC-1\tfalse\tMissing ticket\tSPEC-CV-PMIMPORT-001\n",
				"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\n",
				"pulse.log":    "",
				"status.md":    "# Status\n",
			},
			err: "orphan criterion",
		},
		{
			name: "unknown state",
			files: map[string]string{
				"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tMYSTERY\tImport PM ledger\tdev\tdefault\t\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
				"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\n",
				"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\n",
				"pulse.log":    "",
				"status.md":    "# Status\n",
			},
			err: "unknown PM state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakePMImportRepository()
			statusID := repo.addStatusForPMState("TODO")
			root := writePMImportFixture(t, tt.files)
			svc := NewPMImportService(repo)

			_, err := svc.ImportPMCanon(context.Background(), testActor(), PMImportRequest{RootPath: root, StatusByPMState: map[string]uuid.UUID{"TODO": statusID}})

			require.ErrorIs(t, err, ErrValidation)
			require.Contains(t, err.Error(), tt.err)
			require.Empty(t, repo.tasks)
			require.Empty(t, repo.criteria)
			require.Empty(t, repo.evidence)
			require.Empty(t, repo.events)
		})
	}
}

func TestPMImportRejectsMissingRequiredSourceFilesWithoutPartialWrites(t *testing.T) {
	for _, missing := range []string{"tickets.tsv", "criteria.tsv", "evidence.tsv", "pulse.log", "status.md"} {
		t.Run(missing, func(t *testing.T) {
			repo := newFakePMImportRepository()
			statusID := repo.addStatusForPMState("TODO")
			files := validPMImportFixtureFiles()
			delete(files, missing)
			root := writePMImportFixture(t, files)
			svc := NewPMImportService(repo)

			_, err := svc.ImportPMCanon(context.Background(), testActor(), PMImportRequest{RootPath: root, StatusByPMState: map[string]uuid.UUID{"TODO": statusID}})

			require.ErrorIs(t, err, ErrValidation)
			require.Contains(t, err.Error(), missing)
			require.Empty(t, repo.tasks)
			require.Empty(t, repo.criteria)
			require.Empty(t, repo.evidence)
			require.Empty(t, repo.events)
			require.Empty(t, repo.links)
		})
	}
}

func TestPMImportDoesNotRequireMetaEnvOrReadOptionalSecretLikeFiles(t *testing.T) {
	repo := newFakePMImportRepository()
	statusID := repo.addStatusForPMState("TODO")
	root := writePMImportFixture(t, validPMImportFixtureFiles())
	svc := NewPMImportService(repo)

	summary, err := svc.ImportPMCanon(context.Background(), testActor(), PMImportRequest{RootPath: root, DryRun: true, StatusByPMState: map[string]uuid.UUID{"TODO": statusID}})

	require.NoError(t, err)
	require.Equal(t, 1, summary.Tickets.Total)
	require.Empty(t, repo.tasks)
}

func TestPMImportRollsBackLateWriteFailureWithoutPartialRows(t *testing.T) {
	repo := newFailingLatePMImportRepository()
	statusID := repo.addStatusForPMState("TODO")
	root := writePMImportFixture(t, validPMImportFixtureFiles())
	svc := NewPMImportService(repo)

	_, err := svc.ImportPMCanon(context.Background(), testActor(), PMImportRequest{RootPath: root, StatusByPMState: map[string]uuid.UUID{"TODO": statusID}})

	require.ErrorIs(t, err, errForcedPMImportWrite)
	require.Empty(t, repo.tasks)
	require.Empty(t, repo.criteria)
	require.Empty(t, repo.evidence)
	require.Empty(t, repo.events)
	require.Empty(t, repo.links)
}

type fakePMImportRepository struct {
	*fakeConveyorRepository
}

func newFakePMImportRepository() *fakePMImportRepository {
	return &fakePMImportRepository{fakeConveyorRepository: newFakeConveyorRepository()}
}

func (r *fakePMImportRepository) GetEvent(ctx context.Context, id uuid.UUID) (*models.ConveyorEvent, error) {
	for _, event := range r.events {
		if event.ID == id {
			cp := event
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakePMImportRepository) addStatusForPMState(state string) uuid.UUID {
	id := uuid.New()
	open := state != "DONE"
	name := state
	r.statuses[id] = &models.Status{ID: id, Name: &name, IsOpen: &open}
	return id
}

func writePMImportFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return root
}

func validPMImportFixtureFiles() map[string]string {
	return map[string]string{
		"tickets.tsv":  "id\tstate\ttitle\towner\tscope\tspec_path\tcreated_at\tupdated_at\tdeps\ttags\nPM-1\tTODO\tImport PM ledger\tdev\tdefault\tdocs/spec/runbooks/conveyor-pm-import.md\t2026-06-16T00:00:00Z\t2026-06-16T00:00:00Z\t\tpm\n",
		"criteria.tsv": "ticket_id\tac_id\tchecked\ttext\tspec_ids\nPM-1\tAC-1\tfalse\tDry-run reports counts\tSPEC-CV-PMIMPORT-001\n",
		"evidence.tsv": "ticket_id\tdate\tkind\tref\tnote\nPM-1\t2026-06-16\tlink\thttps://example.test/evidence\tImport evidence\n",
		"pulse.log":    "2026-06-16T00:00:00Z\tCREATED\tPM-1 Created from PM canon\n",
		"status.md":    "# Status\n\nPM import fixture status.\n",
	}
}

var errForcedPMImportWrite = errors.New("forced pm import write failure")

type failingLatePMImportRepository struct {
	*fakePMImportRepository
}

func newFailingLatePMImportRepository() *failingLatePMImportRepository {
	return &failingLatePMImportRepository{fakePMImportRepository: newFakePMImportRepository()}
}

func (r *failingLatePMImportRepository) WithTransaction(ctx context.Context, fn func(ConveyorRepository) error) error {
	snapshot := snapshotFakePMImportRepository(r.fakeConveyorRepository)
	err := fn(r)
	if err != nil {
		restoreFakePMImportRepository(r.fakeConveyorRepository, snapshot)
	}
	return err
}

func (r *failingLatePMImportRepository) CreateEvent(ctx context.Context, event *models.ConveyorEvent) error {
	return errForcedPMImportWrite
}

type fakePMImportSnapshot struct {
	tasks       map[uuid.UUID]*models.Task
	criteria    map[uuid.UUID]*models.AcceptanceCriterion
	evidence    map[uuid.UUID]*models.Evidence
	events      []models.ConveyorEvent
	links       map[string]models.TaskLink
	idempotency map[string]models.IdempotencyRecord
}

func snapshotFakePMImportRepository(repo *fakeConveyorRepository) fakePMImportSnapshot {
	return fakePMImportSnapshot{
		tasks:       cloneTaskMap(repo.tasks),
		criteria:    cloneCriterionMap(repo.criteria),
		evidence:    cloneEvidenceMap(repo.evidence),
		events:      append([]models.ConveyorEvent(nil), repo.events...),
		links:       cloneLinkMap(repo.links),
		idempotency: cloneIdempotencyMap(repo.idempotency),
	}
}

func restoreFakePMImportRepository(repo *fakeConveyorRepository, snapshot fakePMImportSnapshot) {
	repo.tasks = snapshot.tasks
	repo.criteria = snapshot.criteria
	repo.evidence = snapshot.evidence
	repo.events = snapshot.events
	repo.links = snapshot.links
	repo.idempotency = snapshot.idempotency
}

func cloneTaskMap(input map[uuid.UUID]*models.Task) map[uuid.UUID]*models.Task {
	out := make(map[uuid.UUID]*models.Task, len(input))
	for k, v := range input {
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneCriterionMap(input map[uuid.UUID]*models.AcceptanceCriterion) map[uuid.UUID]*models.AcceptanceCriterion {
	out := make(map[uuid.UUID]*models.AcceptanceCriterion, len(input))
	for k, v := range input {
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneEvidenceMap(input map[uuid.UUID]*models.Evidence) map[uuid.UUID]*models.Evidence {
	out := make(map[uuid.UUID]*models.Evidence, len(input))
	for k, v := range input {
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneLinkMap(input map[string]models.TaskLink) map[string]models.TaskLink {
	out := make(map[string]models.TaskLink, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneIdempotencyMap(input map[string]models.IdempotencyRecord) map[string]models.IdempotencyRecord {
	out := make(map[string]models.IdempotencyRecord, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func decodeImportMetadata(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

var _ = decodeImportMetadata
