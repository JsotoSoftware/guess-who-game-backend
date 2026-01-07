package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Input struct {
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
	Items     []struct {
		Name      string `json:"name"`
		Franchise string `json:"franchise"`
	} `json:"items"`
}

func main() {
	var (
		file   = flag.String("file", "", "path to json file")
		dsn    = flag.String("dsn", "", "postgres dsn")
		lang   = flag.String("lang", "es", "language for translations (default es)")
		public = flag.Bool("public", true, "set packs is_public")
	)
	flag.Parse()

	if *file == "" || *dsn == "" {
		fmt.Println("usage: importjson --file <path> --dsn <postgres dsn> [--lang es] [--public true]")
		os.Exit(2)
	}

	b, err := os.ReadFile(*file)
	must(err)

	var in Input
	must(json.Unmarshal(b, &in))

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	must(err)
	defer pool.Close()

	// group by franchise -> list of names
	byFranchise := map[string][]string{}
	for _, it := range in.Items {
		f := strings.TrimSpace(it.Franchise)
		n := strings.TrimSpace(it.Name)
		if f == "" || n == "" {
			continue
		}
		byFranchise[f] = append(byFranchise[f], n)
	}

	fmt.Printf("Franchises: %d\n", len(byFranchise))

	for franchise, names := range byFranchise {
		slug := slugify(franchise)
		packID := upsertPack(ctx, pool, slug, *public)
		upsertPackTranslation(ctx, pool, packID, *lang, franchise, "")

		seen := map[string]bool{}
		for _, name := range names {
			if seen[name] {
				continue
			}
			seen[name] = true

			canon := slug + "." + slugify(name)
			charID := upsertCharacter(ctx, pool, packID, canon)
			upsertCharacterTranslation(ctx, pool, charID, *lang, name)
		}

		fmt.Printf("Imported pack %-24s (%s): %d characters\n", franchise, slug, len(seen))
	}

	fmt.Println("done")
}

func upsertPack(ctx context.Context, db *pgxpool.Pool, slug string, isPublic bool) string {
	// keep version=1 for now; later we can bump version on changes
	var id string
	err := db.QueryRow(ctx, `
		INSERT INTO packs (slug, version, is_public, created_at)
		VALUES ($1, 1, $2, now())
		ON CONFLICT (slug) DO UPDATE SET is_public = EXCLUDED.is_public
		RETURNING id
	`, slug, isPublic).Scan(&id)
	must(err)
	return id
}

func upsertPackTranslation(ctx context.Context, db *pgxpool.Pool, packID, lang, name, desc string) {
	_, err := db.Exec(ctx, `
		INSERT INTO pack_translations (pack_id, lang, name, description)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (pack_id, lang) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description
	`, packID, lang, name, desc)
	must(err)
}

func upsertCharacter(ctx context.Context, db *pgxpool.Pool, packID, canonicalKey string) string {
	var id string
	err := db.QueryRow(ctx, `
		INSERT INTO characters (pack_id, canonical_key, created_at)
		VALUES ($1, $2, now())
		ON CONFLICT (pack_id, canonical_key) DO UPDATE SET canonical_key = EXCLUDED.canonical_key
		RETURNING id
	`, packID, canonicalKey).Scan(&id)
	must(err)
	return id
}

func upsertCharacterTranslation(ctx context.Context, db *pgxpool.Pool, charID, lang, name string) {
	_, err := db.Exec(ctx, `
		INSERT INTO character_translations (character_id, lang, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (character_id, lang) DO UPDATE SET name = EXCLUDED.name
	`, charID, lang, name)
	must(err)
}

func must(err error) {
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ñ", "n")
	// quick normalize for common accents (good enough for slugs)
	repl := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"ü", "u",
	)
	s = repl.Replace(s)
	s = nonAlnum.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = fmt.Sprintf("pack_%d", time.Now().Unix())
	}
	return s
}
