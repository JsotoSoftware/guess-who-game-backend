package storage

import (
	"context"
	"time"
)

type PackDTO struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Version     int       `json:"version"`
	IsPublic    bool      `json:"isPublic"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CharacterDTO struct {
	ID           string `json:"id"`
	PackID       string `json:"packId"`
	CanonicalKey string `json:"canonicalKey"`
	Name         string `json:"name"`
}

func (s *Storage) ListPacks(ctx context.Context, lang string) ([]PackDTO, error) {
	rows, err := s.PG.Query(ctx, `
		SELECT
			p.id, p.slug, p.version, p.is_public, p.created_at,
			COALESCE(pt_req.name, pt_es.name, pt_en.name, p.slug) AS name,
			COALESCE(pt_req.description, pt_es.description, pt_en.description, '') AS description
		FROM packs p
		LEFT JOIN pack_translations pt_req ON pt_req.pack_id = p.id AND pt_req.lang = $1
		LEFT JOIN pack_translations pt_es  ON pt_es.pack_id  = p.id AND pt_es.lang  = 'es'
		LEFT JOIN pack_translations pt_en  ON pt_en.pack_id  = p.id AND pt_en.lang  = 'en'
		WHERE p.is_public = true
		ORDER BY name ASC
	`, lang)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []PackDTO
	for rows.Next() {
		var p PackDTO
		if err := rows.Scan(&p.ID, &p.Slug, &p.Version, &p.IsPublic, &p.CreatedAt, &p.Name, &p.Description); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Storage) GetPackBySlug(ctx context.Context, slug, lang string) (*PackDTO, error) {
	var p PackDTO
	err := s.PG.QueryRow(ctx, `
		SELECT
			p.id, p.slug, p.version, p.is_public, p.created_at,
			COALESCE(pt_req.name, pt_es.name, pt_en.name, p.slug) AS name,
			COALESCE(pt_req.description, pt_es.description, pt_en.description, '') AS description
		FROM packs p
		LEFT JOIN pack_translations pt_req ON pt_req.pack_id = p.id AND pt_req.lang = $2
		LEFT JOIN pack_translations pt_es  ON pt_es.pack_id  = p.id AND pt_es.lang  = 'es'
		LEFT JOIN pack_translations pt_en  ON pt_en.pack_id  = p.id AND pt_en.lang  = 'en'
		WHERE p.slug = $1 AND p.is_public = true
	`, slug, lang).Scan(&p.ID, &p.Slug, &p.Version, &p.IsPublic, &p.CreatedAt, &p.Name, &p.Description)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Storage) ListCharactersByPackSlug(ctx context.Context, slug, lang string, limit, offset int) ([]CharacterDTO, error) {
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.PG.Query(ctx, `
		SELECT
			c.id, c.pack_id, c.canonical_key,
			COALESCE(ct_req.name, ct_es.name, ct_en.name, c.canonical_key) AS name
		FROM packs p
		JOIN characters c ON c.pack_id = p.id
		LEFT JOIN character_translations ct_req ON ct_req.character_id = c.id AND ct_req.lang = $2
		LEFT JOIN character_translations ct_es  ON ct_es.character_id  = c.id AND ct_es.lang  = 'es'
		LEFT JOIN character_translations ct_en  ON ct_en.character_id  = c.id AND ct_en.lang  = 'en'
		WHERE p.slug = $1 AND p.is_public = true
		ORDER BY name ASC
		LIMIT $3 OFFSET $4
	`, slug, lang, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CharacterDTO
	for rows.Next() {
		var c CharacterDTO
		if err := rows.Scan(&c.ID, &c.PackID, &c.CanonicalKey, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
