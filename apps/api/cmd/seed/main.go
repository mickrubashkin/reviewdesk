package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailreview"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

const (
	sequence = "onboarding"
)

type seedEmail struct {
	Slug           string
	Sequence       string
	Title          string
	Subject        *string
	Preheader      *string
	SendTiming     *string
	Stage          string
	SortOrder      int
	Language       string
	Variant        string
	BodyText       string
	ContentParts   EmailContentParts
	OriginalHTML   string
	ReviewHTML     string
	TemplateHTML   string
	TemplateHash   string
	EditableFields []byte
}

type seedEmailMeta struct {
	Title      *string `json:"title"`
	Subject    *string `json:"subject"`
	Preheader  *string `json:"preheader"`
	SendTiming *string `json:"send_timing"`
}

type EmailContentParts = emailtext.ContentParts

func main() {
	_ = godotenv.Load("../../.env")

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	if err := dbpool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to ping database: %v\n", err)
		os.Exit(1)
	}
	logSeedOperationalEvent(ctx, dbpool, "info", "seed_started", "Email seed started", nil)

	seedDir, err := findSeedDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find seed emails directory: %v\n", err)
		os.Exit(1)
	}

	emails, err := loadSeedEmails(seedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to load seed emails: %v\n", err)
		os.Exit(1)
	}

	for _, email := range emails {
		if err := upsertEmail(ctx, dbpool, email); err != nil {
			fmt.Fprintf(os.Stderr, "Unable to seed %s: %v\n", email.Slug, err)
			os.Exit(1)
		}
	}

	deleteStaleEmails := shouldDeleteStaleSeedEmails()
	if deleteStaleEmails {
		logSeedOperationalEvent(ctx, dbpool, "warn", "seed_delete_stale_enabled", "Seed stale email deletion enabled", nil)
		if err := deleteStaleSeedEmails(ctx, dbpool, emails); err != nil {
			fmt.Fprintf(os.Stderr, "Unable to delete stale seed emails: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Skipped deleting stale seed emails; set SEED_DELETE_STALE_EMAILS=true to prune seed-owned data")
	}

	logSeedOperationalEvent(ctx, dbpool, "info", "seed_completed", "Email seed completed", map[string]any{
		"seeded_email_count":   len(emails),
		"delete_stale_enabled": deleteStaleEmails,
	})
	fmt.Printf("Seeded %d emails\n", len(emails))
}

func shouldDeleteStaleSeedEmails() bool {
	return os.Getenv("SEED_DELETE_STALE_EMAILS") == "true"
}

func findSeedDir() (string, error) {
	if value := os.Getenv("SEED_EMAILS_DIR"); value != "" {
		return value, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "db", "seeds", "emails")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", fmt.Errorf("db/seeds/emails was not found from %s or its parents", workingDir)
}

func loadSeedEmails(root string) ([]seedEmail, error) {
	emails := []seedEmail{}

	metaByKey, err := loadSeedEmailMeta(root)
	if err != nil {
		return nil, err
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}

		email, err := parseSeedEmail(root, path, metaByKey)
		if err != nil {
			return err
		}

		emails = append(emails, email)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return emails, nil
}

func loadSeedEmailMeta(root string) (map[string]seedEmailMeta, error) {
	metaPath := filepath.Join(root, "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]seedEmailMeta{}, nil
		}

		return nil, err
	}

	metaByKey := map[string]seedEmailMeta{}
	if err := json.Unmarshal(metaBytes, &metaByKey); err != nil {
		return nil, err
	}

	return metaByKey, nil
}

func parseSeedEmail(root string, path string, metaByKey map[string]seedEmailMeta) (seedEmail, error) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return seedEmail{}, err
	}

	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	if len(parts) != 3 {
		return seedEmail{}, fmt.Errorf("expected {stage}/{email}/{language}.html, got %s", relativePath)
	}

	stageOrder, stage := parseOrderedName(parts[0])
	emailOrder, emailName := parseOrderedName(parts[1])
	fileName := strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))
	language, variant := parseLanguageVariant(fileName)
	emailMeta := lookupSeedEmailMeta(metaByKey, stage, emailName, language, variant)
	title := titleFromName(emailName)
	if emailMeta.Title != nil {
		title = *emailMeta.Title
	}

	htmlBytes, err := os.ReadFile(path)
	if err != nil {
		return seedEmail{}, err
	}
	originalHTML := string(htmlBytes)
	reviewHTML, err := emailreview.AddReviewBlocks(originalHTML)
	if err != nil {
		return seedEmail{}, fmt.Errorf("failed to add review blocks to %s: %w", relativePath, err)
	}
	templateHTML := reviewHTML
	editableFields, err := emailedit.ExtractEditableFields(templateHTML)
	if err != nil {
		return seedEmail{}, fmt.Errorf("failed to extract editable fields from %s: %w", relativePath, err)
	}
	if _, err := emailedit.RenderEditableHTMLWithMetadata(templateHTML, editableFields, emailedit.RenderMetadata{
		Preheader: stringFromPointer(emailMeta.Preheader),
	}); err != nil {
		return seedEmail{}, fmt.Errorf("failed to validate editable fields render for %s: %w", relativePath, err)
	}
	editableFieldsJSON, err := editableFields.JSON()
	if err != nil {
		return seedEmail{}, fmt.Errorf("failed to encode editable fields for %s: %w", relativePath, err)
	}

	bodyText := emailtext.HTMLToText(originalHTML)
	contentParts := extractEmailContentParts(originalHTML, emailMeta, bodyText)

	return seedEmail{
		Slug:           seedEmailSlug(stage, emailName, language, variant),
		Sequence:       sequence,
		Title:          title,
		Subject:        emailMeta.Subject,
		Preheader:      emailMeta.Preheader,
		SendTiming:     emailMeta.SendTiming,
		Stage:          stage,
		SortOrder:      stageOrder*100 + emailOrder,
		Language:       language,
		Variant:        variant,
		BodyText:       bodyText,
		ContentParts:   contentParts,
		OriginalHTML:   originalHTML,
		ReviewHTML:     reviewHTML,
		TemplateHTML:   templateHTML,
		TemplateHash:   templateHash(templateHTML),
		EditableFields: editableFieldsJSON,
	}, nil
}

func parseLanguageVariant(fileName string) (string, string) {
	if fileName == "old" {
		return "en", "old"
	}

	if language, ok := strings.CutSuffix(fileName, "-old"); ok {
		return language, "old"
	}

	return fileName, "new"
}

func lookupSeedEmailMeta(metaByKey map[string]seedEmailMeta, stage string, emailName string, language string, variant string) seedEmailMeta {
	keys := []string{}
	if variant == "old" {
		keys = append(keys, strings.Join([]string{sequence, stage, emailName, language + "-old"}, "/"))
	}
	if variant == "old" && language == "en" {
		keys = append(keys, strings.Join([]string{sequence, stage, emailName, "old"}, "/"))
	}
	keys = append(keys, strings.Join([]string{sequence, stage, emailName, language}, "/"))

	for _, key := range keys {
		if meta, ok := metaByKey[key]; ok {
			return meta
		}
	}

	return seedEmailMeta{}
}

func seedEmailSlug(stage string, emailName string, language string, variant string) string {
	parts := []string{sequence, stage, emailName, language}
	if variant == "old" {
		parts = append(parts, "old")
	}

	return strings.Join(parts, "-")
}

func parseOrderedName(name string) (int, string) {
	for i, char := range name {
		if !unicode.IsDigit(char) {
			if i > 0 && (char == '-' || char == '_') {
				order, err := strconv.Atoi(name[:i])
				if err == nil {
					return order, name[i+1:]
				}
			}

			return 0, name
		}
	}

	return 0, name
}

func titleFromName(name string) string {
	words := strings.FieldsFunc(name, func(char rune) bool {
		return char == '-' || char == '_'
	})

	for i, word := range words {
		words[i] = capitalize(word)
	}

	return strings.Join(words, " ")
}

func capitalize(word string) string {
	if word == "" {
		return word
	}

	runes := []rune(word)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func upsertEmail(ctx context.Context, dbpool *pgxpool.Pool, email seedEmail) error {
	_, err := dbpool.Exec(ctx, `
		INSERT INTO emails (
			slug,
			sequence,
			title,
			subject,
			preheader,
			send_timing,
			stage,
			sort_order,
			language,
			variant,
			body_text,
			content_parts,
			original_html,
			review_html,
			template_html,
			template_hash,
			editable_fields
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17::jsonb)
		ON CONFLICT (slug) DO UPDATE SET
			sequence = EXCLUDED.sequence,
			title = EXCLUDED.title,
			subject = EXCLUDED.subject,
			preheader = EXCLUDED.preheader,
			send_timing = EXCLUDED.send_timing,
			stage = EXCLUDED.stage,
			sort_order = EXCLUDED.sort_order,
			language = EXCLUDED.language,
			variant = EXCLUDED.variant,
			body_text = EXCLUDED.body_text,
			content_parts = EXCLUDED.content_parts,
			original_html = EXCLUDED.original_html,
			review_html = EXCLUDED.review_html,
			template_html = EXCLUDED.template_html,
			template_hash = EXCLUDED.template_hash,
			editable_fields = EXCLUDED.editable_fields,
			updated_at = now()
		WHERE emails.sequence IS DISTINCT FROM EXCLUDED.sequence
			OR emails.title IS DISTINCT FROM EXCLUDED.title
			OR emails.subject IS DISTINCT FROM EXCLUDED.subject
			OR emails.preheader IS DISTINCT FROM EXCLUDED.preheader
			OR emails.send_timing IS DISTINCT FROM EXCLUDED.send_timing
			OR emails.stage IS DISTINCT FROM EXCLUDED.stage
			OR emails.sort_order IS DISTINCT FROM EXCLUDED.sort_order
			OR emails.language IS DISTINCT FROM EXCLUDED.language
			OR emails.variant IS DISTINCT FROM EXCLUDED.variant
			OR emails.body_text IS DISTINCT FROM EXCLUDED.body_text
			OR emails.content_parts IS DISTINCT FROM EXCLUDED.content_parts
			OR emails.original_html IS DISTINCT FROM EXCLUDED.original_html
			OR emails.review_html IS DISTINCT FROM EXCLUDED.review_html
			OR emails.template_html IS DISTINCT FROM EXCLUDED.template_html
			OR emails.template_hash IS DISTINCT FROM EXCLUDED.template_hash
			OR emails.editable_fields IS DISTINCT FROM EXCLUDED.editable_fields;
	`, email.Slug, email.Sequence, email.Title, email.Subject, email.Preheader, email.SendTiming, email.Stage, email.SortOrder, email.Language, email.Variant, email.BodyText, email.ContentParts, email.OriginalHTML, email.ReviewHTML, email.TemplateHTML, email.TemplateHash, email.EditableFields)

	return err
}

func deleteStaleSeedEmails(ctx context.Context, dbpool *pgxpool.Pool, emails []seedEmail) error {
	slugs := make([]string, 0, len(emails))
	for _, email := range emails {
		slugs = append(slugs, email.Slug)
	}

	_, err := dbpool.Exec(ctx, `
		DELETE FROM emails
		WHERE sequence = $1
			AND NOT (slug = ANY($2));
	`, sequence, slugs)

	return err
}

func logSeedOperationalEvent(ctx context.Context, dbpool *pgxpool.Pool, level string, eventType string, message string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to encode seed operational event metadata: %v\n", err)
		return
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO operational_events (
			level,
			event_type,
			message,
			metadata
		)
		VALUES ($1, $2, $3, $4::jsonb);
	`, level, eventType, message, metadataBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to write seed operational event %s: %v\n", eventType, err)
	}
}

func extractEmailContentParts(originalHTML string, emailMeta seedEmailMeta, bodyText string) EmailContentParts {
	return emailtext.ExtractContentParts(
		originalHTML,
		stringFromPointer(emailMeta.Subject),
		stringFromPointer(emailMeta.Preheader),
		bodyText,
	)
}

func stringFromPointer(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func templateHash(templateHTML string) string {
	hash := sha256.Sum256([]byte(templateHTML))
	return fmt.Sprintf("%x", hash)
}
