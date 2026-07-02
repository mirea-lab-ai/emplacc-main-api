package postgres

import (
	"errors"
	"time"

	models "emplacc-api/internal/domain"
	"emplacc-api/internal/ports"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type apiTokenRepository struct{ db *gorm.DB }

func NewAPITokenRepository(db *gorm.DB) ports.APITokenRepository {
	return &apiTokenRepository{db: db}
}

func (r *apiTokenRepository) Create(t models.APIToken) error {
	return r.db.Exec(`
		INSERT INTO api_tokens (id, user_id, name, token_hash, last_used_at, expires_at, deleted, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID.String(), t.UserID.String(), t.Name, t.TokenHash,
		t.LastUsedAt, t.ExpiresAt, t.Deleted, t.CreatedAt, t.UpdatedAt,
	).Error
}

func (r *apiTokenRepository) ListByUser(userID uuid.UUID) ([]models.APIToken, error) {
	var rows []models.APIToken
	err := r.db.Raw(`
		SELECT id, user_id, name, token_hash, last_used_at, expires_at, deleted, created_at, updated_at
		FROM api_tokens WHERE user_id = $1::uuid AND deleted = false
		ORDER BY created_at DESC`, userID.String(),
	).Scan(&rows).Error
	return rows, err
}

func (r *apiTokenRepository) FindByHash(hash string) (*models.APIToken, error) {
	var rows []models.APIToken
	err := r.db.Raw(`
		SELECT id, user_id, name, token_hash, last_used_at, expires_at, deleted, created_at, updated_at
		FROM api_tokens WHERE token_hash = $1 AND deleted = false LIMIT 1`, hash,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("not found")
	}
	return &rows[0], nil
}

func (r *apiTokenRepository) Revoke(userID uuid.UUID, tokenID uuid.UUID) error {
	res := r.db.Exec(`
		UPDATE api_tokens SET deleted = true, updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND deleted = false`,
		tokenID.String(), userID.String(),
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("token not found")
	}
	return nil
}

func (r *apiTokenRepository) UpdateLastUsed(tokenID uuid.UUID, at time.Time) error {
	return r.db.Exec(`
		UPDATE api_tokens SET last_used_at = $1 WHERE id = $2::uuid`,
		at, tokenID.String(),
	).Error
}
