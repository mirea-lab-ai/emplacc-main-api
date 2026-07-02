// Package git holds outbound adapters for code-hosting providers (GitHub, GitFlic).
// Each implements ports.CommitProvider; the net/http driver stays at this adapter
// layer and never leaks into the service layer.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
)

// GitHubProvider — адаптер GitHub REST API. Токен опционален (без него — публичные
// репо с лимитом). Реализует порт ports.CommitProvider.
type GitHubProvider struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewGitHubProvider(token string) *GitHubProvider {
	return &GitHubProvider{
		token:   token,
		baseURL: "https://api.github.com",
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *GitHubProvider) Name() string { return "github" }

type ghCommit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

const ghMaxPages = 10 // до 1000 коммитов за синк — защита от безлимитного обхода

func (p *GitHubProvider) FetchCommits(ctx context.Context, repo models.CodeRepository, since *time.Time) ([]ports.ProviderCommit, error) {
	out := make([]ports.ProviderCommit, 0, 100)
	for page := 1; page <= ghMaxPages; page++ {
		q := url.Values{}
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		if repo.Branch != "" {
			q.Set("sha", repo.Branch)
		}
		if since != nil {
			q.Set("since", since.UTC().Format(time.RFC3339))
		}
		endpoint := fmt.Sprintf("%s/repos/%s/%s/commits?%s", p.baseURL, repo.Owner, repo.Name, q.Encode())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if p.token != "" {
			req.Header.Set("Authorization", "Bearer "+p.token)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github %s/%s: status %d: %s", repo.Owner, repo.Name, resp.StatusCode, string(body))
		}

		var batch []ghCommit
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, err
		}
		for _, c := range batch {
			pc := ports.ProviderCommit{
				SHA:         c.SHA,
				Message:     c.Commit.Message,
				AuthorName:  c.Commit.Author.Name,
				AuthorEmail: c.Commit.Author.Email,
				URL:         c.HTMLURL,
				CommittedAt: c.Commit.Author.Date,
			}
			if c.Author != nil {
				pc.AuthorLogin = c.Author.Login
			}
			out = append(out, pc)
		}
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}
