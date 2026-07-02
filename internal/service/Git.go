package service

import (
	"context"
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type LinkRepositoryRequest struct {
	ProjectID uuid.UUID
	Provider  string
	Owner     string
	Name      string
	URL       string
	Branch    string
}

type SyncSummary struct {
	Repositories int `json:"repositories"`
	Fetched      int `json:"fetched"`
	Created      int `json:"created"`
}

type GitService interface {
	Providers() []string
	LinkRepository(req LinkRepositoryRequest) (*models.CodeRepository, error)
	ListRepositories(projectID uuid.UUID) ([]models.CodeRepository, error)
	UnlinkRepository(id uuid.UUID) error
	SyncRepository(ctx context.Context, repoID uuid.UUID) (*SyncSummary, error)
	SyncProject(ctx context.Context, projectID uuid.UUID) (*SyncSummary, error)
	ListCommitsByTask(taskID uuid.UUID, page, pageSize int) ([]models.Commit, int64, error)
	ListCommitsByProject(projectID uuid.UUID, page, pageSize int) ([]models.Commit, int64, error)
	LinkCommitToTask(commitID uuid.UUID, taskID *uuid.UUID) error
}

type gitService struct {
	repo      ports.GitRepository
	userRepo  ports.UserRepository
	providers map[string]ports.CommitProvider
}

func NewGitService(repo ports.GitRepository, userRepo ports.UserRepository, providers ...ports.CommitProvider) GitService {
	reg := make(map[string]ports.CommitProvider, len(providers))
	for _, p := range providers {
		if p != nil {
			reg[strings.ToLower(p.Name())] = p
		}
	}
	return &gitService{repo: repo, userRepo: userRepo, providers: reg}
}

// Привязка задачи к коммиту по соглашению в сообщении: "task:<uuid>", "task <uuid>" или "#<uuid>".
var taskRefRe = regexp.MustCompile(`(?i)(?:task[:#\s]+|#)([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

func parseTaskRef(message string) *uuid.UUID {
	m := taskRefRe.FindStringSubmatch(message)
	if len(m) < 2 {
		return nil
	}
	id, err := uuid.Parse(m[1])
	if err != nil {
		return nil
	}
	return &id
}

func (s *gitService) Providers() []string {
	out := make([]string, 0, len(s.providers))
	for name := range s.providers {
		out = append(out, name)
	}
	return out
}

func (s *gitService) LinkRepository(req LinkRepositoryRequest) (*models.CodeRepository, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if _, ok := s.providers[provider]; !ok {
		return nil, errors.New("unsupported provider")
	}
	owner := strings.TrimSpace(req.Owner)
	name := strings.TrimSpace(req.Name)
	if req.ProjectID == uuid.Nil || owner == "" || name == "" {
		return nil, errors.New("project_id, owner and name are required")
	}
	repo := &models.CodeRepository{
		ID:        uuid.New(),
		ProjectID: req.ProjectID,
		Provider:  provider,
		Owner:     owner,
		Name:      name,
		URL:       strings.TrimSpace(req.URL),
		Branch:    strings.TrimSpace(req.Branch),
		CreatedAt: time.Now(),
	}
	if err := s.repo.CreateRepository(repo); err != nil {
		return nil, err
	}
	return repo, nil
}

func (s *gitService) ListRepositories(projectID uuid.UUID) ([]models.CodeRepository, error) {
	return s.repo.ListRepositoriesByProject(projectID)
}

func (s *gitService) UnlinkRepository(id uuid.UUID) error {
	ok, err := s.repo.SoftDeleteRepository(id)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("repository not found")
	}
	return nil
}

func (s *gitService) SyncRepository(ctx context.Context, repoID uuid.UUID) (*SyncSummary, error) {
	repo, err := s.repo.GetRepository(repoID)
	if err != nil {
		return nil, err
	}
	summary := &SyncSummary{Repositories: 1}
	if err := s.syncOne(ctx, *repo, summary); err != nil {
		return nil, err
	}
	return summary, nil
}

func (s *gitService) SyncProject(ctx context.Context, projectID uuid.UUID) (*SyncSummary, error) {
	repos, err := s.repo.ListRepositoriesByProject(projectID)
	if err != nil {
		return nil, err
	}
	summary := &SyncSummary{}
	for _, repo := range repos {
		summary.Repositories++
		if err := s.syncOne(ctx, repo, summary); err != nil {
			// одна сломанная репа не валит весь синк
			continue
		}
	}
	return summary, nil
}

func (s *gitService) syncOne(ctx context.Context, repo models.CodeRepository, summary *SyncSummary) error {
	provider, ok := s.providers[repo.Provider]
	if !ok {
		return errors.New("unsupported provider")
	}
	raw, err := provider.FetchCommits(ctx, repo, repo.LastSyncAt)
	if err != nil {
		return err
	}
	summary.Fetched += len(raw)

	now := time.Now()
	commits := make([]models.Commit, 0, len(raw))
	for _, pc := range raw {
		c := models.Commit{
			ID:           uuid.New(),
			RepositoryID: repo.ID,
			SHA:          pc.SHA,
			Message:      pc.Message,
			AuthorName:   pc.AuthorName,
			AuthorEmail:  pc.AuthorEmail,
			AuthorLogin:  pc.AuthorLogin,
			URL:          pc.URL,
			CommittedAt:  pc.CommittedAt,
			CreatedAt:    now,
			TaskID:       parseTaskRef(pc.Message),
		}
		// Резолвим автора в пользователя emplacc по email (best-effort).
		if pc.AuthorEmail != "" {
			if u, e := s.userRepo.GetUserByEmail(pc.AuthorEmail); e == nil && u != nil {
				c.AuthorUserID = &u.ID
			}
		}
		commits = append(commits, c)
	}

	created, err := s.repo.UpsertCommits(commits)
	if err != nil {
		return err
	}
	summary.Created += created
	_ = s.repo.TouchRepositorySync(repo.ID, now)
	return nil
}

func (s *gitService) ListCommitsByTask(taskID uuid.UUID, page, pageSize int) ([]models.Commit, int64, error) {
	return s.repo.ListCommitsByTask(taskID, pageSize, (page-1)*pageSize)
}

func (s *gitService) ListCommitsByProject(projectID uuid.UUID, page, pageSize int) ([]models.Commit, int64, error) {
	return s.repo.ListCommitsByProject(projectID, pageSize, (page-1)*pageSize)
}

func (s *gitService) LinkCommitToTask(commitID uuid.UUID, taskID *uuid.UUID) error {
	ok, err := s.repo.LinkCommitToTask(commitID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("commit not found")
	}
	return nil
}
