package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"
)

// GitFlicProvider — адаптер российского хостинга GitFlic (docs.gitflic.ru/latest/api).
// Реализует тот же порт ports.CommitProvider, что и GitHub — независимость от
// конкретного хранилища кода.
//
// API: база https://api.gitflic.ru, авторизация `Authorization: token <access token>`,
// ответы — Spring-HATEOAS (`_embedded.commitList` + объект `page`), пагинация page(с 0)/size.
type GitFlicProvider struct {
	token   string
	baseURL string
	webURL  string
	client  *http.Client
}

func NewGitFlicProvider(token, baseURL string) *GitFlicProvider {
	if baseURL == "" {
		baseURL = "https://api.gitflic.ru"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	// Человекочитаемый домен для ссылок на коммиты (API живёт на api.gitflic.ru,
	// веб — на gitflic.ru; для self-hosted отрезаем /rest-api).
	webURL := "https://gitflic.ru"
	if !strings.Contains(baseURL, "api.gitflic.ru") {
		webURL = strings.TrimSuffix(baseURL, "/rest-api")
	}
	return &GitFlicProvider{
		token:   token,
		baseURL: baseURL,
		webURL:  webURL,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *GitFlicProvider) Name() string { return "gitflic" }

type gfIdent struct {
	Name         string     `json:"name"`
	EmailAddress string     `json:"emailAddress"`
	When         *time.Time `json:"when"`
}

type gfCommit struct {
	Hash           string     `json:"hash"`
	Message        string     `json:"message"`
	ShortMessage   string     `json:"shortMessage"`
	CreatedAt      *time.Time `json:"createdAt"`
	AuthorIdent    gfIdent    `json:"authorIdent"`
	CommitterIdent gfIdent    `json:"committerIdent"`
	User           struct {
		Username string `json:"username"`
	} `json:"user"`
}

func (c gfCommit) message() string {
	if c.Message != "" {
		return c.Message
	}
	return c.ShortMessage
}

func (c gfCommit) when() time.Time {
	if c.AuthorIdent.When != nil {
		return *c.AuthorIdent.When
	}
	if c.CreatedAt != nil {
		return *c.CreatedAt
	}
	if c.CommitterIdent.When != nil {
		return *c.CommitterIdent.When
	}
	return time.Time{}
}

type gfResponse struct {
	Embedded struct {
		CommitList []gfCommit `json:"commitList"`
	} `json:"_embedded"`
	Page struct {
		TotalPages int `json:"totalPages"`
		Number     int `json:"number"`
	} `json:"page"`
}

const gfMaxPages = 10 // до 1000 коммитов за синк (размер 100) — защита от безлимитного обхода

func (p *GitFlicProvider) FetchCommits(ctx context.Context, repo models.CodeRepository, since *time.Time) ([]ports.ProviderCommit, error) {
	out := make([]ports.ProviderCommit, 0, 100)
	for page := 0; page < gfMaxPages; page++ {
		endpoint := fmt.Sprintf("%s/project/%s/%s/commits?page=%d&size=100", p.baseURL, repo.Owner, repo.Name, page)
		if repo.Branch != "" {
			endpoint += "&branch=" + url.QueryEscape(repo.Branch)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if p.token != "" {
			req.Header.Set("Authorization", "token "+p.token)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gitflic %s/%s: status %d: %s", repo.Owner, repo.Name, resp.StatusCode, string(body))
		}

		var parsed gfResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		list := parsed.Embedded.CommitList
		if len(list) == 0 {
			break
		}
		for _, c := range list {
			if c.Hash == "" {
				continue
			}
			committed := c.when()
			// GitFlic не фильтрует по дате на сервере — отсекаем старое на клиенте.
			if since != nil && !committed.After(*since) {
				continue
			}
			out = append(out, ports.ProviderCommit{
				SHA:         c.Hash,
				Message:     c.message(),
				AuthorName:  c.AuthorIdent.Name,
				AuthorEmail: c.AuthorIdent.EmailAddress,
				AuthorLogin: c.User.Username,
				URL:         fmt.Sprintf("%s/project/%s/%s/commit/%s", p.webURL, repo.Owner, repo.Name, c.Hash),
				CommittedAt: committed,
			})
		}
		if parsed.Page.TotalPages > 0 && page+1 >= parsed.Page.TotalPages {
			break
		}
		if len(list) < 100 {
			break
		}
	}
	return out, nil
}
