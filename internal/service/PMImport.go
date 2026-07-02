package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
)

type PMImportEventRepository interface {
	GetEvent(ctx context.Context, id uuid.UUID) (*models.ConveyorEvent, error)
}

type PMImportService interface {
	ImportPMCanon(ctx context.Context, actor ConveyorActor, req PMImportRequest) (*PMImportSummary, error)
}

type pmImportService struct {
	repo ports.ConveyorRepository
}

type PMImportRequest struct {
	RootPath        string
	DryRun          bool
	StatusByPMState map[string]uuid.UUID
}

type PMImportSummary struct {
	DryRun             bool            `json:"dry_run"`
	SwitchOverAccepted bool            `json:"switch_over_accepted"`
	Tickets            PMImportCounter `json:"tickets"`
	Criteria           PMImportCounter `json:"criteria"`
	Evidence           PMImportCounter `json:"evidence"`
	Events             PMImportCounter `json:"events"`
	Links              PMImportCounter `json:"links"`
	StatusFacts        PMImportCounter `json:"status_facts"`
}

type PMImportCounter struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Reused  int `json:"reused"`
}

type pmCanonSnapshot struct {
	tickets    []pmTicketRow
	criteria   []pmCriterionRow
	evidence   []pmEvidenceRow
	events     []pmPulseRow
	statusText string
	ticketIDs  map[string]struct{}
}

type pmTicketRow struct {
	Line      int
	ID        string
	State     string
	Title     string
	Owner     string
	Scope     string
	SpecPath  string
	CreatedAt time.Time
	UpdatedAt time.Time
	Deps      []string
	Tags      []string
}

type pmCriterionRow struct {
	Line    int
	Ticket  string
	ACID    string
	Checked string
	Text    string
	SpecIDs []string
}

type pmEvidenceRow struct {
	Line   int
	Ticket string
	Date   time.Time
	Kind   string
	Ref    string
	Note   string
}

type pmPulseRow struct {
	Line      int
	Timestamp time.Time
	Type      string
	Ticket    string
	Message   string
}

func NewPMImportService(repo ports.ConveyorRepository) PMImportService {
	return &pmImportService{repo: repo}
}

func (s *pmImportService) ImportPMCanon(ctx context.Context, actor ConveyorActor, req PMImportRequest) (*PMImportSummary, error) {
	root := strings.TrimSpace(req.RootPath)
	if root == "" {
		return nil, fmt.Errorf("%w: pm root is required", ErrValidation)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: pm root is not readable: %s", ErrValidation, root)
	}
	snapshot, err := readPMCanonSnapshot(root, req.StatusByPMState)
	if err != nil {
		return nil, err
	}
	summary := &PMImportSummary{DryRun: req.DryRun, SwitchOverAccepted: false}
	summary.Tickets.Total = len(snapshot.tickets)
	summary.Criteria.Total = len(snapshot.criteria)
	summary.Evidence.Total = len(snapshot.evidence)
	summary.Events.Total = len(snapshot.events)
	summary.StatusFacts.Total = 1
	for _, ticket := range snapshot.tickets {
		summary.Links.Total += len(ticket.Deps)
	}
	if req.DryRun {
		return summary, nil
	}
	err = s.repo.WithTransaction(ctx, func(repo ports.ConveyorRepository) error {
		for _, ticket := range snapshot.tickets {
			created, err := importPMTicket(ctx, repo, actor, ticket, req.StatusByPMState[ticket.State], snapshot.statusText)
			if err != nil {
				return err
			}
			addCounter(&summary.Tickets, created)
		}
		for _, criterion := range snapshot.criteria {
			created, err := importPMCriterion(ctx, repo, actor, criterion)
			if err != nil {
				return err
			}
			addCounter(&summary.Criteria, created)
		}
		for _, evidence := range snapshot.evidence {
			created, err := importPMEvidence(ctx, repo, actor, evidence)
			if err != nil {
				return err
			}
			addCounter(&summary.Evidence, created)
		}
		for _, event := range snapshot.events {
			created, err := s.importPMEvent(ctx, repo, actor, event)
			if err != nil {
				return err
			}
			addCounter(&summary.Events, created)
		}
		for _, ticket := range snapshot.tickets {
			for _, dep := range ticket.Deps {
				created, err := importPMDependencyLink(ctx, repo, actor, dep, ticket.ID)
				if err != nil {
					return err
				}
				addCounter(&summary.Links, created)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return summary, nil
}

func PMImportStableID(kind string, source string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("conveyor:pm-import:"+kind+":"+source))
}

func readPMCanonSnapshot(root string, statusByState map[string]uuid.UUID) (*pmCanonSnapshot, error) {
	for _, name := range []string{"tickets.tsv", "criteria.tsv", "evidence.tsv", "pulse.log", "status.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			return nil, fmt.Errorf("%w: required PM source file is missing: %s", ErrValidation, name)
		}
	}
	tickets, ticketIDs, err := readPMTickets(filepath.Join(root, "tickets.tsv"), statusByState)
	if err != nil {
		return nil, err
	}
	criteria, err := readPMCriteria(filepath.Join(root, "criteria.tsv"), ticketIDs)
	if err != nil {
		return nil, err
	}
	evidence, err := readPMEvidence(filepath.Join(root, "evidence.tsv"), ticketIDs)
	if err != nil {
		return nil, err
	}
	events, err := readPMPulse(filepath.Join(root, "pulse.log"), ticketIDs)
	if err != nil {
		return nil, err
	}
	statusBytes, err := os.ReadFile(filepath.Join(root, "status.md"))
	if err != nil {
		return nil, fmt.Errorf("%w: status.md is not readable", ErrValidation)
	}
	return &pmCanonSnapshot{tickets: tickets, criteria: criteria, evidence: evidence, events: events, statusText: string(statusBytes), ticketIDs: ticketIDs}, nil
}

func readPMTickets(path string, statusByState map[string]uuid.UUID) ([]pmTicketRow, map[string]struct{}, error) {
	rows, err := readTSVRows(path, []string{"id", "state", "title", "owner", "scope", "spec_path", "created_at", "updated_at", "deps", "tags"})
	if err != nil {
		return nil, nil, err
	}
	out := make([]pmTicketRow, 0, len(rows))
	ids := map[string]struct{}{}
	for _, row := range rows {
		id := strings.TrimSpace(row.Fields[0])
		state := strings.TrimSpace(row.Fields[1])
		title := strings.TrimSpace(row.Fields[2])
		if id == "" || state == "" || title == "" {
			return nil, nil, fmt.Errorf("%w: tickets.tsv:%d requires id, state, and title", ErrValidation, row.Line)
		}
		if _, ok := statusByState[state]; !ok {
			return nil, nil, fmt.Errorf("%w: tickets.tsv:%d unknown PM state %q", ErrValidation, row.Line, state)
		}
		if _, exists := ids[id]; exists {
			return nil, nil, fmt.Errorf("%w: tickets.tsv:%d duplicate ticket %s", ErrValidation, row.Line, id)
		}
		created, err := parsePMTime(row.Fields[6], row.Line, "tickets.tsv")
		if err != nil {
			return nil, nil, err
		}
		updated, err := parsePMTime(row.Fields[7], row.Line, "tickets.tsv")
		if err != nil {
			return nil, nil, err
		}
		ids[id] = struct{}{}
		out = append(out, pmTicketRow{Line: row.Line, ID: id, State: state, Title: title, Owner: strings.TrimSpace(row.Fields[3]), Scope: strings.TrimSpace(row.Fields[4]), SpecPath: strings.TrimSpace(row.Fields[5]), CreatedAt: created, UpdatedAt: updated, Deps: splitPMList(row.Fields[8]), Tags: splitPMList(row.Fields[9])})
	}
	for _, ticket := range out {
		for _, dep := range ticket.Deps {
			if _, ok := ids[dep]; !ok {
				return nil, nil, fmt.Errorf("%w: tickets.tsv:%d missing dependency ticket %s", ErrValidation, ticket.Line, dep)
			}
		}
	}
	return out, ids, nil
}

// parsePMChecked accepts both the boolean form (true/false) and the markdown
// checkbox form ([x]/[ ]) used in the project's actual criteria.tsv canon.
func parsePMChecked(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "[x]", "x", "yes", "1", "done", "checked":
		return "true", true
	case "false", "[ ]", "[]", "", "no", "0", "unchecked":
		return "false", true
	default:
		return "", false
	}
}

func readPMCriteria(path string, ticketIDs map[string]struct{}) ([]pmCriterionRow, error) {
	rows, err := readTSVRows(path, []string{"ticket_id", "ac_id", "checked", "text", "spec_ids"})
	if err != nil {
		return nil, err
	}
	out := make([]pmCriterionRow, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		ticketID := strings.TrimSpace(row.Fields[0])
		acID := strings.TrimSpace(row.Fields[1])
		checked := strings.TrimSpace(row.Fields[2])
		text := strings.TrimSpace(row.Fields[3])
		if _, ok := ticketIDs[ticketID]; !ok {
			return nil, fmt.Errorf("%w: criteria.tsv:%d orphan criterion for ticket %s", ErrValidation, row.Line, ticketID)
		}
		if acID == "" || text == "" {
			return nil, fmt.Errorf("%w: criteria.tsv:%d requires ac_id and text", ErrValidation, row.Line)
		}
		normalizedChecked, ok := parsePMChecked(checked)
		if !ok {
			return nil, fmt.Errorf("%w: criteria.tsv:%d checked must be true/false or [x]/[ ]", ErrValidation, row.Line)
		}
		checked = normalizedChecked
		key := ticketID + ":" + acID
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: criteria.tsv:%d duplicate criterion %s", ErrValidation, row.Line, key)
		}
		seen[key] = struct{}{}
		out = append(out, pmCriterionRow{Line: row.Line, Ticket: ticketID, ACID: acID, Checked: checked, Text: text, SpecIDs: splitPMList(row.Fields[4])})
	}
	return out, nil
}

func readPMEvidence(path string, ticketIDs map[string]struct{}) ([]pmEvidenceRow, error) {
	rows, err := readTSVRows(path, []string{"ticket_id", "date", "kind", "ref", "note"})
	if err != nil {
		return nil, err
	}
	out := make([]pmEvidenceRow, 0, len(rows))
	for _, row := range rows {
		ticketID := strings.TrimSpace(row.Fields[0])
		if _, ok := ticketIDs[ticketID]; !ok {
			return nil, fmt.Errorf("%w: evidence.tsv:%d orphan evidence for ticket %s", ErrValidation, row.Line, ticketID)
		}
		kind := strings.TrimSpace(row.Fields[2])
		if _, ok := pmEvidenceType(kind); !ok {
			return nil, fmt.Errorf("%w: evidence.tsv:%d unknown evidence kind %q", ErrValidation, row.Line, kind)
		}
		date, err := parsePMDate(row.Fields[1], row.Line, "evidence.tsv")
		if err != nil {
			return nil, err
		}
		out = append(out, pmEvidenceRow{Line: row.Line, Ticket: ticketID, Date: date, Kind: kind, Ref: strings.TrimSpace(row.Fields[3]), Note: strings.TrimSpace(row.Fields[4])})
	}
	return out, nil
}

func readPMPulse(path string, ticketIDs map[string]struct{}) ([]pmPulseRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: pulse.log is not readable", ErrValidation)
	}
	lines := strings.Split(string(data), "\n")
	out := make([]pmPulseRow, 0, len(lines))
	for i, line := range lines {
		lineNo := i + 1
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("%w: pulse.log:%d expected 3 tab-separated fields", ErrValidation, lineNo)
		}
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("%w: pulse.log:%d invalid timestamp", ErrValidation, lineNo)
		}
		message := strings.TrimSpace(parts[2])
		ticketID := firstToken(message)
		if _, ok := ticketIDs[ticketID]; !ok {
			// Not every pulse line is ticket-scoped (e.g. the INIT line). Keep
			// these as project-level events rather than failing the migration.
			ticketID = ""
		}
		out = append(out, pmPulseRow{Line: lineNo, Timestamp: at, Type: strings.TrimSpace(parts[1]), Ticket: ticketID, Message: message})
	}
	return out, nil
}

type pmTSVRow struct {
	Line   int
	Fields []string
}

func readTSVRows(path string, expectedHeader []string) ([]pmTSVRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s is not readable", ErrValidation, filepath.Base(path))
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("%w: %s header is required", ErrValidation, filepath.Base(path))
	}
	header := strings.Split(strings.TrimSuffix(lines[0], "\r"), "\t")
	if !sameStringSlice(header, expectedHeader) {
		return nil, fmt.Errorf("%w: %s header mismatch", ErrValidation, filepath.Base(path))
	}
	rows := []pmTSVRow{}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != len(expectedHeader) {
			return nil, fmt.Errorf("%w: %s:%d expected %d fields, got %d", ErrValidation, filepath.Base(path), i+1, len(expectedHeader), len(fields))
		}
		rows = append(rows, pmTSVRow{Line: i + 1, Fields: fields})
	}
	return rows, nil
}

func importPMTicket(ctx context.Context, repo ports.ConveyorRepository, actor ConveyorActor, row pmTicketRow, statusID uuid.UUID, statusText string) (bool, error) {
	id := PMImportStableID("ticket", row.ID)
	if _, err := repo.GetTask(ctx, id); err == nil {
		return false, nil
	} else if err != nil && err != ErrNotFound {
		return false, err
	}
	name := row.Title
	description := pmTicketDescription(row, statusText)
	deleted := false
	createdBy := actor.ActorID
	assignedTo := actor.ActorID
	task := &models.Task{ID: id, Name: &name, Description: &description, CreatedBy: &createdBy, AssignedTo: &assignedTo, StatusID: statusID, Deleted: &deleted, CreatedAt: &row.CreatedAt, UpdatedAt: &row.UpdatedAt}
	if err := repo.CreateTask(ctx, task); err != nil {
		return false, err
	}
	return true, nil
}

func importPMCriterion(ctx context.Context, repo ports.ConveyorRepository, actor ConveyorActor, row pmCriterionRow) (bool, error) {
	id := PMImportStableID("criterion", row.Ticket+":"+row.ACID)
	if _, err := repo.GetAcceptanceCriterion(ctx, id); err == nil {
		return false, nil
	} else if err != nil && err != ErrNotFound {
		return false, err
	}
	now := time.Now()
	state := models.AcceptanceCriterionStateUnchecked
	if row.Checked == "true" {
		state = models.AcceptanceCriterionStatePassed
	}
	criterion := &models.AcceptanceCriterion{ID: id, TaskID: PMImportStableID("ticket", row.Ticket), ACID: row.ACID, Title: row.Text, Required: true, State: state, CreatedBy: actor.ActorID, CreatedAt: now, UpdatedAt: now}
	if len(row.SpecIDs) > 0 {
		criterion.SpecIDs = mustMarshalJSON(row.SpecIDs)
	}
	if err := repo.CreateAcceptanceCriterion(ctx, criterion); err != nil {
		return false, err
	}
	return true, nil
}

func importPMEvidence(ctx context.Context, repo ports.ConveyorRepository, actor ConveyorActor, row pmEvidenceRow) (bool, error) {
	id := PMImportStableID("evidence", fmt.Sprintf("%s:%d", row.Ticket, row.Line-1))
	if _, err := repo.GetEvidence(ctx, id); err == nil {
		return false, nil
	} else if err != nil && err != ErrNotFound {
		return false, err
	}
	evidenceType, _ := pmEvidenceType(row.Kind)
	metadata := mustMarshalJSON(map[string]any{"source": "pm-import", "pm_ticket_id": row.Ticket, "source_locator": fmt.Sprintf("evidence.tsv:%d", row.Line), "kind": row.Kind})
	title := row.Note
	if title == "" {
		title = row.Ref
	}
	if containsSecretLikeEvidence(row.Ref, title, nil) {
		return false, fmt.Errorf("%w: evidence.tsv:%d contains secret-like data", ErrValidation, row.Line)
	}
	evidence := &models.Evidence{ID: id, TaskID: PMImportStableID("ticket", row.Ticket), Type: evidenceType, Verdict: models.EvidenceVerdictInformational, URI: row.Ref, Title: title, Metadata: metadata, CreatedBy: actor.ActorID, CreatedAt: row.Date}
	if err := repo.CreateEvidence(ctx, evidence); err != nil {
		return false, err
	}
	return true, nil
}

func (s *pmImportService) importPMEvent(ctx context.Context, repo ports.ConveyorRepository, actor ConveyorActor, row pmPulseRow) (bool, error) {
	id := PMImportStableID("event", fmt.Sprintf("pulse:%d:%s:%s", row.Line, row.Ticket, row.Type))
	eventRepo, ok := repo.(PMImportEventRepository)
	if !ok {
		eventRepo, ok = s.repo.(PMImportEventRepository)
	}
	if !ok {
		return false, fmt.Errorf("%w: PM import event lookup is not available", ErrValidation)
	}
	if _, err := eventRepo.GetEvent(ctx, id); err == nil {
		return false, nil
	} else if err != nil && err != ErrNotFound {
		return false, err
	}
	payload := mustMarshalJSON(map[string]any{"source": "pm-import", "pm_ticket_id": row.Ticket, "source_locator": fmt.Sprintf("pulse.log:%d", row.Line), "message": row.Message})
	event := &models.ConveyorEvent{ID: id, WorkItemID: PMImportStableID("ticket", row.Ticket), Type: "pm_import." + row.Type, Timestamp: row.Timestamp, ActorType: actorTypeOrDefault(actor.ActorType), ActorID: actor.ActorID, Payload: payload, SchemaVersion: 1, Source: "pm-import"}
	if err := repo.CreateEvent(ctx, event); err != nil {
		return false, err
	}
	return true, nil
}

func importPMDependencyLink(ctx context.Context, repo ports.ConveyorRepository, actor ConveyorActor, sourceTicket string, targetTicket string) (bool, error) {
	sourceID := PMImportStableID("ticket", sourceTicket)
	targetID := PMImportStableID("ticket", targetTicket)
	exists, err := repo.TaskLinkExists(ctx, sourceID, targetID, models.TaskLinkTypeBlocks)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	link := &models.TaskLink{ID: PMImportStableID("link", sourceTicket+":blocks:"+targetTicket), SourceTaskID: sourceID, TargetTaskID: targetID, LinkType: models.TaskLinkTypeBlocks, CreatedBy: actor.ActorID, CreatedAt: time.Now()}
	if err := repo.CreateTaskLink(ctx, link); err != nil {
		return false, err
	}
	return true, nil
}

func pmTicketDescription(row pmTicketRow, statusText string) string {
	parts := []string{
		"PM Source ID: " + row.ID,
		fmt.Sprintf("PM Source Locator: tickets.tsv:%d", row.Line),
		"PM State: " + row.State,
	}
	if row.Owner != "" {
		parts = append(parts, "PM Owner: "+row.Owner)
	}
	if row.Scope != "" {
		parts = append(parts, "PM Scope: "+row.Scope)
	}
	if row.SpecPath != "" {
		parts = append(parts, "PM Spec Path: "+row.SpecPath)
	}
	if len(row.Tags) > 0 {
		parts = append(parts, "PM Tags: "+strings.Join(row.Tags, ","))
	}
	if strings.TrimSpace(statusText) != "" {
		parts = append(parts, "PM Status Snapshot: status.md")
	}
	return strings.Join(parts, "\n")
}

func addCounter(counter *PMImportCounter, created bool) {
	if created {
		counter.Created++
		return
	}
	counter.Reused++
}

func splitPMList(value string) []string {
	items := []string{}
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func parsePMTime(value string, line int, file string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s:%d invalid timestamp", ErrValidation, file, line)
	}
	return parsed, nil
}

func parsePMDate(value string, line int, file string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s:%d invalid date", ErrValidation, file, line)
	}
	return parsed, nil
}

// pmEvidenceType maps the canon's free-form evidence kinds onto the nearest
// domain EvidenceType. The original kind is preserved in the imported evidence
// metadata (see importPMEvidence), so unknown kinds fall back to a generic log
// type rather than failing the one-time migration.
func pmEvidenceType(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "link":
		return models.EvidenceTypeLink, true
	case "file":
		return models.EvidenceTypeFile, true
	case "log", "build", "lint", "test", "tests", "qa", "audit":
		return models.EvidenceTypeLog, true
	case "markdown_report", "note", "spec", "trace", "implementation", "experiment", "report", "review", "doc":
		return models.EvidenceTypeMarkdownReport, true
	case "pr":
		return models.EvidenceTypePR, true
	case "commit":
		return models.EvidenceTypeCommit, true
	case "screenshot":
		return models.EvidenceTypeScreenshot, true
	case "healthcheck", "live":
		return models.EvidenceTypeHealthcheck, true
	case "deployment_log", "deploy":
		return models.EvidenceTypeDeploymentLog, true
	default:
		return models.EvidenceTypeLog, true
	}
}

func firstToken(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func sameStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
