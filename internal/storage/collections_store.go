package storage

import (
	"context"
	"time"
)

type CollectionDTO struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	IsPublic    bool      `json:"isPublic"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CollectionPackDTO struct {
	PackID string `json:"packId"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
}

func (s *Storage) ListCollections(ctx context.Context, lang string) ([]CollectionDTO, error) {
	rows, err := s.PG.Query(ctx, `
		SELECT
			c.id, c.slug, c.is_public, c.created_at,
			COALESCE(ct_req.name, ct_es.name, ct_en.name, c.slug) AS name,
			COALESCE(ct_req.description, ct_es.description, ct_en.description, '') AS description
		FROM collections c
		LEFT JOIN collection_translations ct_req ON ct_req.collection_id = c.id AND ct_req.lang = $1
		LEFT JOIN collection_translations ct_es  ON ct_es.collection_id  = c.id AND ct_es.lang  = 'es'
		LEFT JOIN collection_translations ct_en  ON ct_en.collection_id  = c.id AND ct_en.lang  = 'en'
		WHERE c.is_public = true
		ORDER BY name ASC
	`, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CollectionDTO
	for rows.Next() {
		var c CollectionDTO
		if err := rows.Scan(&c.ID, &c.Slug, &c.IsPublic, &c.CreatedAt, &c.Name, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Storage) ListPacksForCollection(ctx context.Context, collectionSlug, lang string) ([]CollectionPackDTO, error) {
	rows, err := s.PG.Query(ctx, `
		SELECT
			p.id, p.slug,
			COALESCE(pt_req.name, pt_es.name, pt_en.name, p.slug) AS name
		FROM collections c
		JOIN collection_packs cp ON cp.collection_id = c.id
		JOIN packs p ON p.id = cp.pack_id
		LEFT JOIN pack_translations pt_req ON pt_req.pack_id = p.id AND pt_req.lang = $2
		LEFT JOIN pack_translations pt_es  ON pt_es.pack_id  = p.id AND pt_es.lang  = 'es'
		LEFT JOIN pack_translations pt_en  ON pt_en.pack_id  = p.id AND pt_en.lang  = 'en'
		WHERE c.slug = $1 AND c.is_public = true AND p.is_public = true
		ORDER BY name ASC
	`, collectionSlug, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CollectionPackDTO
	for rows.Next() {
		var p CollectionPackDTO
		if err := rows.Scan(&p.PackID, &p.Slug, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
