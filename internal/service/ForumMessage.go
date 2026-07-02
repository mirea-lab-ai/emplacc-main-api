package service

import (
	models "emplacc-api/internal/domain"
	"emplacc-api/internal/dto/request"
	"emplacc-api/internal/ports"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Маркап упоминаний в тексте сообщения: @[Имя](user:uuid) и #[Имя](team:uuid).
var (
	forumUserMentionRe  = regexp.MustCompile(`@\[[^\]]*\]\(user:([0-9a-fA-F-]+)\)`)
	forumTeamMentionRe  = regexp.MustCompile(`#\[[^\]]*\]\(team:([0-9a-fA-F-]+)\)`)
	forumMentionStripRe = regexp.MustCompile(`[@#]\[([^\]]*)\]\((?:user|team|task|project):[^)]+\)`)
)

type ForumMessageService interface {
	GetAllForumMessages(page, pageSize int) ([]models.ForumMessage, int64, error)
	GetForumMessagesByProblemId(problemID uuid.UUID, page, pageSize int) ([]models.ForumMessage, int64, error)
	GetForumMessageById(messageID uuid.UUID) (*models.ForumMessage, error)
	CreateForumMessage(req request.CreateForumMessageRequest) (uuid.UUID, error)
	UpdateForumMessage(messageID uuid.UUID, req request.UpdateForumMessageRequest) error
	DeleteForumMessage(messageID uuid.UUID) error
}

type forumMessageService struct {
	repo     ports.ForumMessageRepository
	notifier ports.Notifier
	teamRepo ports.TeamRepository
}

func NewForumMessageService(repo ports.ForumMessageRepository, notifier ports.Notifier, teamRepo ports.TeamRepository) ForumMessageService {
	return &forumMessageService{
		repo:     repo,
		notifier: notifier,
		teamRepo: teamRepo,
	}
}

func (s *forumMessageService) GetAllForumMessages(page, pageSize int) ([]models.ForumMessage, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetAllForumMessages(pageSize, offset)
}

func (s *forumMessageService) GetForumMessagesByProblemId(problemID uuid.UUID, page, pageSize int) ([]models.ForumMessage, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.GetForumMessagesByProblemId(problemID, pageSize, offset)
}

func (s *forumMessageService) GetForumMessageById(messageID uuid.UUID) (*models.ForumMessage, error) {
	return s.repo.GetForumMessageById(messageID)
}

func (s *forumMessageService) CreateForumMessage(req request.CreateForumMessageRequest) (uuid.UUID, error) {
	problemId, err := uuid.Parse(req.ProblemID)
	if err != nil {
		return uuid.Nil, errors.New("invalid problem id")
	}

	var creatorId *uuid.UUID
	if req.CreatorID != "" {
		v, err := uuid.Parse(req.CreatorID)
		if err != nil {
			return uuid.Nil, errors.New("invalid creator id")
		}
		creatorId = &v
	}

	now := time.Now()
	del := false

	var description pq.StringArray
	if req.Description != nil {
		description = pq.StringArray(*req.Description)
	} else {
		description = pq.StringArray{}
	}

	var replyToID *uuid.UUID
	if req.ReplyToID != nil && *req.ReplyToID != "" {
		v, err := uuid.Parse(*req.ReplyToID)
		if err == nil {
			replyToID = &v
		}
	}

	fm := models.ForumMessage{
		ID:          uuid.New(),
		ProblemID:   problemId,
		Description: description,
		CreatorID:   creatorId,
		ReplyToID:   replyToID,
		CreatedAt:   &now,
		UpdatedAt:   &now, // равно CreatedAt на создании, иначе GORM проставит свой time.Now() при INSERT и сообщение будет выглядеть «изменённым»
		Deleted:     &del,
	}

	err = s.repo.CreateForumMessage(fm)
	if err != nil {
		return uuid.Nil, err
	}

	publishGlobal(StreamEvent{Type: "forum.message.created", WorkItemID: problemId.String()})

	if s.notifier != nil {
		s.notifyForumMessage(problemId, creatorId, replyToID, []string(description))
	}
	return fm.ID, nil
}

// notifyForumMessage шлёт уведомления: автору родителя (ответ/цитата), упомянутым
// пользователям (@[..](user:id)) и всем участникам упомянутых команд
// (#[..](team:id)). Каждый получатель уведомляется один раз, автор — никогда.
func (s *forumMessageService) notifyForumMessage(problemId uuid.UUID, creatorId, replyToID *uuid.UUID, description []string) {
	notified := map[uuid.UUID]bool{}
	if creatorId != nil {
		notified[*creatorId] = true // себя не уведомляем
	}
	preview := forumNotifyPreview(description)

	// Ответ/цитата — автору родительского сообщения.
	if replyToID != nil {
		if parent, err := s.repo.GetForumMessageById(*replyToID); err == nil && parent != nil && parent.CreatorID != nil && !notified[*parent.CreatorID] {
			notified[*parent.CreatorID] = true
			s.notifier.Notify(*parent.CreatorID, "forum.reply", "Новый ответ на ваше сообщение", preview, "problem", &problemId)
		}
	}

	text := strings.Join(description, "\n")

	// @упоминания пользователей.
	for _, m := range forumUserMentionRe.FindAllStringSubmatch(text, -1) {
		uid, err := uuid.Parse(m[1])
		if err != nil || notified[uid] {
			continue
		}
		notified[uid] = true
		s.notifier.Notify(uid, "forum.mention", "Вас упомянули в форуме", preview, "problem", &problemId)
	}

	// #упоминания команд → всем участникам команды.
	if s.teamRepo != nil {
		for _, m := range forumTeamMentionRe.FindAllStringSubmatch(text, -1) {
			tid, err := uuid.Parse(m[1])
			if err != nil {
				continue
			}
			team, err := s.teamRepo.GetTeamByID(tid)
			if err != nil || team == nil {
				continue
			}
			for _, tm := range team.TeamMembers {
				if notified[tm.UserID] {
					continue
				}
				notified[tm.UserID] = true
				s.notifier.Notify(tm.UserID, "forum.team_mention", "Упомянута ваша команда в форуме", preview, "problem", &problemId)
			}
		}
	}
}

// forumNotifyPreview — короткое читаемое превью сообщения для тела уведомления
// (mention-маркап сворачивается до имени, длина ограничена).
func forumNotifyPreview(description []string) string {
	text := strings.TrimSpace(strings.Join(description, " "))
	text = forumMentionStripRe.ReplaceAllString(text, "$1")
	text = strings.TrimSpace(text)
	r := []rune(text)
	if len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return text
}

func (s *forumMessageService) UpdateForumMessage(messageID uuid.UUID, req request.UpdateForumMessageRequest) error {
	updateData := make(map[string]interface{})
	if req.ProblemID != nil {
		pid, err := uuid.Parse(*req.ProblemID)
		if err != nil {
			return errors.New("invalid problem id")
		}
		updateData["problem_id"] = pid
	}
	if req.Description != nil {
		updateData["description"] = pq.StringArray(*req.Description)
	}
	if req.CreatorID != nil {
		cid, err := uuid.Parse(*req.CreatorID)
		if err != nil {
			return errors.New("invalid creator id")
		}
		updateData["creator_id"] = cid
	}

	if len(updateData) == 0 {
		return errors.New("no fields to update")
	}

	now := time.Now()
	updateData["updated_at"] = &now

	updated, err := s.repo.UpdateForumMessage(messageID, updateData)
	if err != nil {
		return err
	}

	if !updated {
		return errors.New("forum message not found")
	}

	publishGlobal(StreamEvent{Type: "forum.message.updated"})
	return nil
}

func (s *forumMessageService) DeleteForumMessage(messageID uuid.UUID) error {
	deleted, err := s.repo.DeleteForumMessage(messageID)
	if err != nil {
		return err
	}

	if !deleted {
		return errors.New("forum message not found")
	}

	publishGlobal(StreamEvent{Type: "forum.message.deleted"})
	return nil
}
