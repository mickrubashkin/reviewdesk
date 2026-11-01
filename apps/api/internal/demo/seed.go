// Package demo seeds the synthetic demo scenario of the public demo: the
// shared demo board and the private board each visitor gets.
package demo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailreview"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

const (
	demoBoardKey       = "demo-onboarding"
	demoBoardName      = "Demo Onboarding Review"
	defaultBoardKey    = "onboarding"
	demoReviewerEmail  = "demo-reviewer@example.com"
	demoAdminEmail     = "demo-admin@example.com"
	demoFixtureRootEnv = "DEMO_EMAILS_DIR"
)

var demoStages = []string{"signup", "profile", "activation", "education", "integration", "first-value", "expansion", "retention"}
var demoLanguages = []string{"en", "es"}

// demoOutdatedTranslation is the email whose EN master changed after its ES
// translation was made, so the board shows a translation behind EN.
const demoOutdatedTranslation = "verify-email"

// demoUpdatedIntro is the EN intro of demoOutdatedTranslation after the change.
const demoUpdatedIntro = "Confirm your address now so your workspace checklist, reminders and team invites reach the right inbox."

var demoSpanishMeta = map[string]struct{ Subject, Preheader string }{
	"welcome":             {"Te damos la bienvenida a Acme Cloud", "Empieza el recorrido de incorporación."},
	"verify-email":        {"Confirma tu dirección de correo", "Confirma tu dirección antes de empezar la lista de configuración."},
	"workspace-invite":    {"Invita a tu equipo al espacio de trabajo", "Suma a las personas adecuadas a la configuración."},
	"complete-profile":    {"Completa tu perfil", "Añade los datos que personalizan tu incorporación."},
	"choose-goals":        {"Elige tu primer objetivo", "Escoge el resultado que guiará los siguientes pasos."},
	"complete-setup":      {"Completa la configuración en tres minutos", "Termina los últimos pasos para ver el primer resultado."},
	"connect-data":        {"Conecta tu primera fuente de datos", "Sincroniza una fuente para que el panel muestre tu progreso."},
	"invite-team":         {"Invita a quienes lanzarán contigo", "Comparte el acceso antes de que el primer flujo esté listo."},
	"first-value":         {"Consigue tu primer resultado", "Usa la lista de pasos para lograr un primer resultado útil."},
	"workflow-tips":       {"Tres formas de avanzar más rápido esta semana", "Usa estas prácticas para no perder el ritmo."},
	"install-app":         {"Instala la aplicación complementaria", "Conecta la aplicación para recibir novedades automáticamente."},
	"sync-settings":       {"Revisa los ajustes de sincronización", "Comprueba que las reglas de automatización encajan con tu equipo."},
	"api-ready":           {"Tu espacio de API está listo", "Envía el primer evento de prueba cuando el equipo técnico esté listo."},
	"launch-checklist":    {"Usa la lista de lanzamiento", "Confirma los últimos detalles antes del primer despliegue."},
	"first-report":        {"Tu primer informe está listo", "Revisa el progreso y decide qué mejorar después."},
	"upgrade-nudge":       {"Listo para el siguiente paso", "Revisa las siguientes opciones cuando tu equipo esté listo."},
	"invite-stakeholders": {"Invita a los responsables antes del lanzamiento", "Da a quienes deciden una vista clara del estado del lanzamiento."},
	"advanced-automation": {"Automatiza la siguiente tarea rutinaria", "Convierte una acción repetida en un flujo gestionado."},
	"weekly-summary":      {"Tu resumen semanal de incorporación", "Mira qué avanzó y qué necesita atención."},
	"renewal-reminder":    {"Prepara la próxima revisión de resultados", "Usa las señales de adopción para planificar el siguiente hito."},
}

var demoEmailMetaByName = map[string]demoEmailMeta{
	"welcome": {
		Title:      "Welcome",
		Subject:    "Welcome to Acme Cloud",
		Preheader:  "Start the generic onboarding flow.",
		SendTiming: "immediately",
	},
	"verify-email": {
		Title:      "Verify Email",
		Subject:    "Confirm your email address",
		Preheader:  "Confirm your address before the workspace checklist begins.",
		SendTiming: "immediately",
	},
	"workspace-invite": {
		Title:      "Workspace Invite",
		Subject:    "Invite your team to the workspace",
		Preheader:  "Bring the right teammates into the setup flow.",
		SendTiming: "+2 hours",
	},
	"complete-profile": {
		Title:      "Complete Profile",
		Subject:    "Complete your profile",
		Preheader:  "Add the details that personalize your onboarding path.",
		SendTiming: "+1 day",
	},
	"choose-goals": {
		Title:      "Choose Goals",
		Subject:    "Choose your first success goal",
		Preheader:  "Pick the outcome that should guide the next steps.",
		SendTiming: "+1 day",
	},
	"complete-setup": {
		Title:      "Complete Setup",
		Subject:    "Complete your setup in three minutes",
		Preheader:  "Finish the final setup steps to unlock the first value moment.",
		SendTiming: "1 day after signup",
	},
	"connect-data": {
		Title:      "Connect Data",
		Subject:    "Connect your first data source",
		Preheader:  "Sync a source so the dashboard can show useful progress.",
		SendTiming: "+2 days",
	},
	"invite-team": {
		Title:      "Invite Team",
		Subject:    "Invite the teammates who will launch with you",
		Preheader:  "Share access before the first workflow is ready.",
		SendTiming: "+2 days",
	},
	"first-value": {
		Title:      "First Value",
		Subject:    "Find your first value moment",
		Preheader:  "Use the checklist to reach your first meaningful result.",
		SendTiming: "3 days after signup",
	},
	"workflow-tips": {
		Title:      "Workflow Tips",
		Subject:    "Three ways to move faster this week",
		Preheader:  "Use these patterns to keep onboarding momentum.",
		SendTiming: "+4 days",
	},
	"install-app": {
		Title:      "Install App",
		Subject:    "Install the companion app",
		Preheader:  "Connect the app to keep updates flowing automatically.",
		SendTiming: "+5 days",
	},
	"sync-settings": {
		Title:      "Sync Settings",
		Subject:    "Review your sync settings",
		Preheader:  "Make sure the automation rules match your team's process.",
		SendTiming: "+6 days",
	},
	"api-ready": {
		Title:      "API Ready",
		Subject:    "Your API workspace is ready",
		Preheader:  "Send the first test event when engineering is ready.",
		SendTiming: "+7 days",
	},
	"launch-checklist": {
		Title:      "Launch Checklist",
		Subject:    "Use the launch checklist",
		Preheader:  "Confirm the final details before the first team rollout.",
		SendTiming: "+9 days",
	},
	"first-report": {
		Title:      "First Report",
		Subject:    "Your first report is ready",
		Preheader:  "Review early progress and decide what to improve next.",
		SendTiming: "+10 days",
	},
	"upgrade-nudge": {
		Title:      "Upgrade Nudge",
		Subject:    "Ready for the next step",
		Preheader:  "Review the next workflow options when your team is ready.",
		SendTiming: "7 days after signup",
	},
	"invite-stakeholders": {
		Title:      "Invite Stakeholders",
		Subject:    "Invite stakeholders before rollout",
		Preheader:  "Give decision makers a concise view of launch readiness.",
		SendTiming: "+14 days",
	},
	"advanced-automation": {
		Title:      "Advanced Automation",
		Subject:    "Automate the next routine step",
		Preheader:  "Turn a repeated action into a managed workflow.",
		SendTiming: "+18 days",
	},
	"weekly-summary": {
		Title:      "Weekly Summary",
		Subject:    "Your weekly onboarding summary",
		Preheader:  "See what moved forward and what still needs attention.",
		SendTiming: "+21 days",
	},
	"renewal-reminder": {
		Title:      "Renewal Reminder",
		Subject:    "Prepare the next success review",
		Preheader:  "Use adoption signals to plan the next account milestone.",
		SendTiming: "+30 days",
	},
}

var demoEmailCatalog = []demoEmailConcept{
	{Stage: "signup", Key: "welcome"},
	{Stage: "signup", Key: "verify-email"},
	{Stage: "signup", Key: "workspace-invite"},
	{Stage: "profile", Key: "complete-profile"},
	{Stage: "profile", Key: "choose-goals"},
	{Stage: "activation", Key: "complete-setup"},
	{Stage: "activation", Key: "connect-data"},
	{Stage: "activation", Key: "invite-team"},
	{Stage: "education", Key: "first-value"},
	{Stage: "education", Key: "workflow-tips"},
	{Stage: "integration", Key: "install-app"},
	{Stage: "integration", Key: "sync-settings"},
	{Stage: "integration", Key: "api-ready"},
	{Stage: "first-value", Key: "launch-checklist"},
	{Stage: "first-value", Key: "first-report"},
	{Stage: "expansion", Key: "upgrade-nudge"},
	{Stage: "expansion", Key: "invite-stakeholders"},
	{Stage: "expansion", Key: "advanced-automation"},
	{Stage: "retention", Key: "weekly-summary"},
	{Stage: "retention", Key: "renewal-reminder"},
}

type demoEmailMeta struct {
	Title      string
	Subject    string
	Preheader  string
	SendTiming string
}

type demoEmailConcept struct {
	Stage string
	Key   string
}

type demoEmail struct {
	Slug           string
	Title          string
	Subject        string
	Preheader      string
	SendTiming     string
	Stage          string
	SortOrder      int
	Language       string
	Variant        string
	BodyText       string
	ContentParts   emailtext.ContentParts
	OriginalHTML   string
	ReviewHTML     string
	TemplateHTML   string
	TemplateHash   string
	EditableFields []byte
	// TranslationOf is the slug of the EN master of a translation.
	TranslationOf string
	// Earlier is the master's first version when it changed since, the one
	// its translation was made from.
	Earlier *demoEmail
}

type demoActor struct {
	ID    string
	Email string
}

// Board is a board the demo scenario is seeded into.
type Board struct {
	Key  string
	Name string
}

// SharedBoard is the demo board everyone sees, seeded by cmd/demoseed.
var SharedBoard = Board{Key: demoBoardKey, Name: demoBoardName}

// slug is the stored slug of a dataset email on this board: the dataset
// names emails after the shared board.
func (board Board) slug(datasetSlug string) string {
	return board.Key + strings.TrimPrefix(datasetSlug, demoBoardKey)
}

var dataset struct {
	once   sync.Once
	emails []demoEmail
	err    error
}

// datasetEmails loads the demo emails once: the fixtures do not change
// while the server runs.
func datasetEmails() ([]demoEmail, error) {
	dataset.once.Do(func() {
		dataset.emails, dataset.err = loadDemoEmailDataset()
		if dataset.err == nil && len(dataset.emails) == 0 {
			dataset.err = fmt.Errorf("no demo emails generated")
		}
	})
	return dataset.emails, dataset.err
}

// Seed puts the demo scenario on board: its stages, the emails with their
// first versions, comments and history.
// Seeding again updates the emails in place. It returns the email count.
func Seed(ctx context.Context, tx pgx.Tx, board Board) (int, error) {
	emails, err := datasetEmails()
	if err != nil {
		return 0, err
	}

	admin, reviewer, err := ensureDemoUsers(ctx, tx)
	if err != nil {
		return 0, err
	}
	if _, err := ensureDemoBoard(ctx, tx, board); err != nil {
		return 0, err
	}

	emailIDs := map[string]string{}
	for _, email := range emails {
		emailID, err := upsertDemoEmail(ctx, tx, board, email)
		if err != nil {
			return 0, fmt.Errorf("upsert %s: %w", email.Slug, err)
		}
		emailIDs[email.Slug] = emailID
		if err := ensureDemoVersions(ctx, tx, emailID, email, admin); err != nil {
			return 0, fmt.Errorf("ensure versions for %s: %w", email.Slug, err)
		}
	}
	for _, email := range emails {
		if email.TranslationOf == "" {
			continue
		}
		masterID, ok := emailIDs[email.TranslationOf]
		if !ok {
			return 0, fmt.Errorf("master %s of %s was not seeded", email.TranslationOf, email.Slug)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE emails SET translation_of = $2, translated_from_version = 1 WHERE id = $1;
		`, emailIDs[email.Slug], masterID); err != nil {
			return 0, fmt.Errorf("link %s to its master: %w", email.Slug, err)
		}
	}

	if err := seedDemoScenario(ctx, tx, board, emailIDs, admin, reviewer); err != nil {
		return 0, err
	}
	return len(emails), nil
}

// Reset deletes the demo-owned data: the shared board, every visitor's
// board and visitor, and the demo sessions.
func Reset(ctx context.Context, tx pgx.Tx) error {
	if err := resetDemoData(ctx, tx); err != nil {
		return err
	}
	_, err := deleteSandboxes(ctx, tx, nil)
	return err
}

// ValidateSeedEnv guards the seed command against running by accident.
func ValidateSeedEnv(reset bool) error {
	return validateDemoSeedEnv(reset)
}

func validateDemoSeedEnv(reset bool) error {
	if os.Getenv("DEMO_SEED_ENABLED") != "true" {
		return fmt.Errorf("DEMO_SEED_ENABLED=true is required")
	}
	if reset && os.Getenv("DEMO_RESET_CONFIRM") != "demo" {
		return fmt.Errorf("DEMO_RESET_CONFIRM=demo is required when using -reset")
	}
	return nil
}

func loadDemoEmailDataset() ([]demoEmail, error) {
	generated, err := generateDemoEmails()
	if err != nil {
		return nil, err
	}

	emailBySlug := map[string]demoEmail{}
	slugOrder := []string{}
	for _, email := range generated {
		emailBySlug[email.Slug] = email
		slugOrder = append(slugOrder, email.Slug)
	}

	fixtureDir, err := findDemoFixtureDir()
	if err != nil {
		return nil, err
	}
	fixtures, err := loadDemoEmails(fixtureDir)
	if err != nil {
		return nil, err
	}
	for _, fixture := range fixtures {
		generated, exists := emailBySlug[fixture.Slug]
		if !exists {
			continue
		}
		// A fixture replaces the HTML only: the email keeps its place, next
		// to its translations.
		fixture.SortOrder = generated.SortOrder
		emailBySlug[fixture.Slug] = fixture
	}

	emails := make([]demoEmail, 0, len(slugOrder))
	for _, slug := range slugOrder {
		emails = append(emails, emailBySlug[slug])
	}
	return emails, nil
}

func generateDemoEmails() ([]demoEmail, error) {
	stageOrder := map[string]int{}
	for index, stage := range demoStages {
		stageOrder[stage] = index + 1
	}

	emails := []demoEmail{}
	for conceptIndex, concept := range demoEmailCatalog {
		meta, ok := demoEmailMetaByName[concept.Key]
		if !ok {
			return nil, fmt.Errorf("missing demo metadata for %s", concept.Key)
		}
		order, ok := stageOrder[concept.Stage]
		if !ok {
			return nil, fmt.Errorf("demo concept %s uses unknown stage %s", concept.Key, concept.Stage)
		}
		for _, language := range demoLanguages {
			localizedMeta := localizeDemoMeta(concept.Key, meta, language)
			copy := demoTemplateCopy(language)
			email, err := buildDemoEmail(order, conceptIndex+1, concept.Stage, concept.Key, language, localizedMeta, generateDemoHTML(concept.Stage, concept.Key, language, localizedMeta, copy))
			if err != nil {
				return nil, err
			}
			if language != "en" {
				email.TranslationOf = demoEmailSlug(concept.Stage, concept.Key, "en")
			}
			if language == "en" && concept.Key == demoOutdatedTranslation {
				earlier := email
				copy.Intro = demoUpdatedIntro
				if email, err = buildDemoEmail(order, conceptIndex+1, concept.Stage, concept.Key, language, localizedMeta, generateDemoHTML(concept.Stage, concept.Key, language, localizedMeta, copy)); err != nil {
					return nil, err
				}
				email.Earlier = &earlier
			}
			emails = append(emails, email)
		}
	}

	return emails, nil
}

func buildDemoEmail(stageOrder int, emailOrder int, stage string, emailName string, language string, meta demoEmailMeta, originalHTML string) (demoEmail, error) {
	reviewHTML, err := emailreview.AddReviewBlocks(originalHTML)
	if err != nil {
		return demoEmail{}, fmt.Errorf("add review blocks to %s/%s/%s: %w", stage, emailName, language, err)
	}
	editableFields, err := emailedit.ExtractEditableFields(reviewHTML)
	if err != nil {
		return demoEmail{}, fmt.Errorf("extract editable fields from %s/%s/%s: %w", stage, emailName, language, err)
	}
	if len(editableFields) == 0 {
		return demoEmail{}, fmt.Errorf("demo email %s/%s/%s must expose editable fields", stage, emailName, language)
	}
	editableFieldsJSON, err := editableFields.JSON()
	if err != nil {
		return demoEmail{}, err
	}

	bodyText := emailtext.HTMLToText(originalHTML)
	return demoEmail{
		Slug:           demoEmailSlug(stage, emailName, language),
		Title:          meta.Title,
		Subject:        meta.Subject,
		Preheader:      meta.Preheader,
		SendTiming:     meta.SendTiming,
		Stage:          stage,
		SortOrder:      stageOrder*100 + emailOrder,
		Language:       language,
		Variant:        "v1",
		BodyText:       bodyText,
		ContentParts:   emailtext.ExtractContentParts(originalHTML, meta.Subject, meta.Preheader, bodyText),
		OriginalHTML:   originalHTML,
		ReviewHTML:     reviewHTML,
		TemplateHTML:   reviewHTML,
		TemplateHash:   contentHash(reviewHTML),
		EditableFields: editableFieldsJSON,
	}, nil
}

func localizeDemoMeta(key string, meta demoEmailMeta, language string) demoEmailMeta {
	if language != "es" {
		return meta
	}
	localized := meta
	localized.Subject = demoSpanishMeta[key].Subject
	localized.Preheader = demoSpanishMeta[key].Preheader
	return localized
}

func generateDemoHTML(stage string, emailName string, language string, meta demoEmailMeta, copy demoTemplateText) string {
	bannerText := strings.ReplaceAll(meta.Title, " ", "+")
	return fmt.Sprintf(`<!doctype html>
<html lang="%s">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s</title>
</head>
<body style="margin:0;padding:0;background:#eef2f7;font-family:Arial,Helvetica,sans-serif;color:#172033;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#eef2f7;padding:32px 0;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="width:100%%;max-width:600px;background:#ffffff;border-radius:12px;overflow:hidden;">
          <tr>
            <td>
              <img data-review-block="hero_banner" data-edit-attr-src="hero_banner_src" data-edit-attr-alt="hero_banner_alt" src="https://placehold.co/1200x480?text=%s" alt="%s" width="600" style="width:100%%;display:block;" />
            </td>
          </tr>
          <tr>
            <td style="padding:32px;">
              <p data-review-block="eyebrow" data-edit-text="eyebrow_text" style="margin:0 0 10px;color:#2364db;font-size:13px;font-weight:bold;text-transform:uppercase;letter-spacing:.04em;">%s</p>
              <h1 data-review-block="headline" data-edit-text="headline_text" style="margin:0 0 16px;font-size:28px;line-height:1.2;">%s</h1>
              <p data-review-block="intro" data-edit-text="intro_text" style="margin:0 0 20px;font-size:16px;line-height:1.6;">%s</p>
              <p data-review-block="risk_copy" data-edit-text="risk_copy_text" style="margin:0 0 20px;font-size:16px;line-height:1.6;">%s</p>
              <p data-review-block="proof_point" data-edit-text="proof_point_text" style="margin:0 0 20px;font-size:16px;line-height:1.6;">%s</p>
              <p data-review-block="secondary_detail" data-edit-text="secondary_detail_text" style="margin:0 0 24px;font-size:14px;line-height:1.6;color:#526174;">%s</p>
              <a data-review-block="primary_cta" data-edit-text="primary_cta_text" data-edit-attr-href="primary_cta_url" href="https://example.com/demo/%s/%s" style="display:inline-block;background:#2364db;color:#ffffff;text-decoration:none;border-radius:6px;padding:12px 18px;font-weight:bold;">%s</a>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		html.EscapeString(language),
		html.EscapeString(meta.Subject),
		html.EscapeString(bannerText),
		html.EscapeString(meta.Title+" demo banner"),
		html.EscapeString(stage),
		html.EscapeString(meta.Subject),
		html.EscapeString(copy.Intro),
		html.EscapeString(copy.RiskCopy),
		html.EscapeString(copy.ProofPoint),
		html.EscapeString(copy.SecondaryDetail),
		html.EscapeString(stage),
		html.EscapeString(emailName),
		html.EscapeString(copy.CTA),
	)
}

type demoTemplateText struct {
	Intro           string
	RiskCopy        string
	ProofPoint      string
	SecondaryDetail string
	CTA             string
}

func demoTemplateCopy(language string) demoTemplateText {
	switch language {
	case "es":
		return demoTemplateText{
			Intro:           "Tu espacio de demostración está listo. Usa este paso para avanzar en la incorporación con una acción clara.",
			RiskCopy:        "Este párrafo está escrito a propósito como texto de revisión, para que los comentarios, las ediciones y los estados de revisión se vean con facilidad.",
			ProofPoint:      "Los equipos pueden revisar el progreso, asignar responsables y preparar el siguiente hito antes del lanzamiento.",
			SecondaryDetail: "Todos los nombres, enlaces y textos son genéricos y no contienen datos de producción.",
			CTA:             "Abrir el siguiente paso",
		}
	default:
		return demoTemplateText{
			Intro:           "Your demo workspace is ready. Use this step to keep the onboarding journey moving with a clear next action.",
			RiskCopy:        "This paragraph is intentionally written as reviewable demo copy so comments, edits, and the review status are easy to inspect.",
			ProofPoint:      "Teams can review progress, assign ownership, and prepare the next milestone before launch.",
			SecondaryDetail: "All names, links, and message content are generic and do not contain production data.",
			CTA:             "Open next step",
		}
	}
}

func findDemoFixtureDir() (string, error) {
	if value := os.Getenv(demoFixtureRootEnv); value != "" {
		return value, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "db", "seeds", "demo-emails")
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

	return "", fmt.Errorf("db/seeds/demo-emails was not found from %s or its parents", workingDir)
}

func loadDemoEmails(root string) ([]demoEmail, error) {
	emails := []demoEmail{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}

		email, err := parseDemoEmail(root, path)
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

func parseDemoEmail(root string, path string) (demoEmail, error) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return demoEmail{}, err
	}
	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	if len(parts) != 3 {
		return demoEmail{}, fmt.Errorf("expected {stage}/{email}/{language}.html, got %s", relativePath)
	}

	stageOrder, stage := parseOrderedName(parts[0])
	emailOrder, emailName := parseOrderedName(parts[1])
	language := strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))
	meta, ok := demoEmailMetaByName[emailName]
	if !ok {
		return demoEmail{}, fmt.Errorf("missing demo metadata for %s", emailName)
	}

	htmlBytes, err := os.ReadFile(path)
	if err != nil {
		return demoEmail{}, err
	}
	originalHTML := string(htmlBytes)
	email, err := buildDemoEmail(stageOrder, emailOrder, stage, emailName, language, meta, originalHTML)
	if err != nil {
		return demoEmail{}, fmt.Errorf("%s: %w", relativePath, err)
	}
	return email, nil
}

func demoEmailSlug(stage string, emailName string, language string) string {
	return strings.Join([]string{demoBoardKey, stage, emailName, language}, "-")
}

func resetDemoData(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text
		FROM emails
		WHERE sequence = $1 OR slug LIKE $2;
	`, demoBoardKey, demoBoardKey+"-%")
	if err != nil {
		return err
	}
	emailIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		emailIDs = append(emailIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(emailIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM email_events
			WHERE email_id = ANY($1)
				OR email_slug LIKE $2;
		`, emailIDs, demoBoardKey+"-%"); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM email_events WHERE email_slug LIKE $1;`, demoBoardKey+"-%"); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM emails
		WHERE sequence = $1 OR slug LIKE $2;
	`, demoBoardKey, demoBoardKey+"-%"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM boards WHERE key = $1;`, demoBoardKey); err != nil {
		return err
	}
	if err := deleteEmptyDefaultBoard(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM sessions
		USING users
		WHERE sessions.user_id = users.id
			AND users.email = ANY($1);
	`, []string{demoAdminEmail, demoReviewerEmail}); err != nil {
		return err
	}

	return nil
}

func deleteEmptyDefaultBoard(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM boards
		WHERE key = $1
			AND NOT EXISTS (
				SELECT 1
				FROM emails
				WHERE coalesce(nullif(trim(sequence), ''), $1) = $1
			);
	`, defaultBoardKey)
	return err
}

func ensureDemoUsers(ctx context.Context, tx pgx.Tx) (demoActor, demoActor, error) {
	adminID, err := upsertDemoUser(ctx, tx, demoAdminEmail, "admin")
	if err != nil {
		return demoActor{}, demoActor{}, err
	}
	reviewerID, err := upsertDemoUser(ctx, tx, demoReviewerEmail, "reviewer")
	if err != nil {
		return demoActor{}, demoActor{}, err
	}

	return demoActor{ID: adminID, Email: demoAdminEmail}, demoActor{ID: reviewerID, Email: demoReviewerEmail}, nil
}

func upsertDemoUser(ctx context.Context, tx pgx.Tx, email string, role string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO users (email, role)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET
			role = EXCLUDED.role,
			updated_at = now()
		RETURNING id::text;
	`, email, role).Scan(&id)
	return id, err
}

func ensureDemoBoard(ctx context.Context, tx pgx.Tx, board Board) (string, error) {
	stagesJSON, err := json.Marshal(demoStages)
	if err != nil {
		return "", err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO boards (key, name, stages)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (key) DO UPDATE SET
			name = EXCLUDED.name,
			stages = EXCLUDED.stages,
			updated_at = now()
		RETURNING id::text;
	`, board.Key, board.Name, stagesJSON).Scan(&id)
	return id, err
}

func upsertDemoEmail(ctx context.Context, tx pgx.Tx, board Board, email demoEmail) (string, error) {
	contentPartsJSON, err := json.Marshal(email.ContentParts)
	if err != nil {
		return "", err
	}

	reviewStatus := demoReviewStatus(email)

	var id string
	err = tx.QueryRow(ctx, `
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
			adaptation_key,
			adaptation_label,
			body_text,
			content_parts,
			original_html,
			review_html,
			template_html,
			template_hash,
			editable_fields,
			review_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'default', 'Default', $11, $12::jsonb, $13, $14, $15, $16, $17::jsonb, $18)
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
			adaptation_key = EXCLUDED.adaptation_key,
			adaptation_label = EXCLUDED.adaptation_label,
			body_text = EXCLUDED.body_text,
			content_parts = EXCLUDED.content_parts,
			original_html = EXCLUDED.original_html,
			review_html = EXCLUDED.review_html,
			template_html = EXCLUDED.template_html,
			template_hash = EXCLUDED.template_hash,
			editable_fields = EXCLUDED.editable_fields,
			review_status = EXCLUDED.review_status,
			archived_at = NULL,
			archived_by = NULL,
			updated_at = now()
		RETURNING id::text;
	`, board.slug(email.Slug), board.Key, email.Title, email.Subject, email.Preheader, email.SendTiming, email.Stage, email.SortOrder, email.Language, email.Variant, email.BodyText, contentPartsJSON, email.OriginalHTML, email.ReviewHTML, email.TemplateHTML, email.TemplateHash, email.EditableFields, reviewStatus).Scan(&id)

	return id, err
}

func demoReviewStatus(email demoEmail) string {
	switch email.Slug {
	case demoEmailSlug("activation", "complete-setup", "en"),
		demoEmailSlug("integration", "sync-settings", "es"),
		demoEmailSlug("retention", "renewal-reminder", "es"):
		return "draft"
	case demoEmailSlug("expansion", "upgrade-nudge", "en"),
		demoEmailSlug("first-value", "first-report", "es"),
		demoEmailSlug("retention", "weekly-summary", "en"),
		demoEmailSlug("signup", "welcome", "en"),
		demoEmailSlug("profile", "complete-profile", "es"),
		demoEmailSlug("education", "first-value", "es"),
		demoEmailSlug("integration", "install-app", "en"),
		demoEmailSlug("expansion", "invite-stakeholders", "es"):
		return "ready"
	default:
		return "draft"
	}
}

// ensureDemoVersions stores the email as its first version, or, when it
// changed since, its earlier text first and the current one as an edit.
func ensureDemoVersions(ctx context.Context, tx pgx.Tx, emailID string, email demoEmail, admin demoActor) error {
	if email.Earlier == nil {
		return ensureDemoVersion(ctx, tx, emailID, 1, "initial", "demo-seed", email)
	}
	if err := ensureDemoVersion(ctx, tx, emailID, 1, "initial", "demo-seed", *email.Earlier); err != nil {
		return err
	}
	return ensureDemoVersion(ctx, tx, emailID, 2, "manual", admin.Email, email)
}

func ensureDemoVersion(ctx context.Context, tx pgx.Tx, emailID string, number int, source string, author string, email demoEmail) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO email_versions (
			email_id,
			version_number,
			created_by_email,
			source,
			title,
			subject,
			preheader,
			original_html,
			template_html,
			editable_fields
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)
		ON CONFLICT (email_id, version_number) DO UPDATE SET
			title = EXCLUDED.title,
			subject = EXCLUDED.subject,
			preheader = EXCLUDED.preheader,
			original_html = EXCLUDED.original_html,
			template_html = EXCLUDED.template_html,
			editable_fields = EXCLUDED.editable_fields;
	`, emailID, number, author, source, email.Title, email.Subject, email.Preheader, email.OriginalHTML, email.TemplateHTML, email.EditableFields)
	return err
}

func seedDemoScenario(ctx context.Context, tx pgx.Tx, board Board, emailIDs map[string]string, admin demoActor, reviewer demoActor) error {
	heroSlug := demoEmailSlug("activation", "complete-setup", "en")
	handoffSlug := demoEmailSlug("expansion", "upgrade-nudge", "en")

	heroID, ok := emailIDs[heroSlug]
	if !ok {
		return fmt.Errorf("hero email %s was not seeded", heroSlug)
	}
	handoffID, ok := emailIDs[handoffSlug]
	if !ok {
		return fmt.Errorf("handoff email %s was not seeded", handoffSlug)
	}

	if err := ensureDemoComment(ctx, tx, heroID, reviewer, demoComment{
		ReviewBlock:  "risk_copy",
		SelectedText: "reviewable copy",
		StartOffset:  52,
		EndOffset:    67,
		Body:         "This paragraph needs a clearer user benefit before it is ready.",
		Status:       "open",
		Severity:     "issue",
	}); err != nil {
		return err
	}
	if err := ensureDemoComment(ctx, tx, heroID, reviewer, demoComment{
		ReviewBlock:  "primary_cta",
		SelectedText: "Complete setup",
		StartOffset:  0,
		EndOffset:    14,
		Body:         "Resolved: CTA wording is specific enough for the activation step.",
		Status:       "resolved",
		Severity:     "suggestion",
		ResolvedBy:   admin.ID,
	}); err != nil {
		return err
	}

	if err := seedSupportingDemoScenarios(ctx, tx, board, emailIDs, admin, reviewer); err != nil {
		return err
	}

	heroTitle := "Complete Setup"
	handoffTitle := "Upgrade Nudge"
	events := []demoEvent{
		{
			Key:        "hero-comment-created",
			Actor:      reviewer,
			Action:     "comment_created",
			EmailID:    heroID,
			EmailSlug:  board.slug(heroSlug),
			EmailTitle: heroTitle,
			Metadata: map[string]any{
				"review_block": "risk_copy",
				"severity":     "issue",
			},
			Changes: map[string]any{},
		},
		{
			Key:        "hero-status-changes-requested",
			Actor:      admin,
			Action:     "email_review_status_updated",
			EmailID:    heroID,
			EmailSlug:  board.slug(heroSlug),
			EmailTitle: heroTitle,
			Metadata: map[string]any{
				"reason": "approval_stale_after_edit",
			},
			Changes: map[string]any{
				"review_status": map[string]any{"before": "ready", "after": "draft"},
			},
		},
		{
			Key:        "handoff-production-approved",
			Actor:      admin,
			Action:     "email_review_status_updated",
			EmailID:    handoffID,
			EmailSlug:  board.slug(handoffSlug),
			EmailTitle: handoffTitle,
			Metadata: map[string]any{
				"reason": "demo_handoff_ready",
			},
			Changes: map[string]any{
				"review_status": map[string]any{"before": "draft", "after": "ready"},
			},
		},
	}

	for _, event := range events {
		if err := insertDemoEventIfMissing(ctx, tx, event); err != nil {
			return err
		}
	}

	return nil
}

func seedSupportingDemoScenarios(ctx context.Context, tx pgx.Tx, board Board, emailIDs map[string]string, admin demoActor, reviewer demoActor) error {
	scenarios := []struct {
		Slug     string
		Title    string
		Comment  demoComment
		EventKey string
	}{
		{
			Slug:  demoEmailSlug("signup", "workspace-invite", "en"),
			Title: "Workspace Invite",
			Comment: demoComment{
				ReviewBlock:  "intro",
				SelectedText: "clear next action",
				StartOffset:  60,
				EndOffset:    77,
				Body:         "Suggestion: make ownership clearer for the teammate receiving this invite.",
				Status:       "open",
				Severity:     "suggestion",
			},
			EventKey: "workspace-invite-comment",
		},
		{
			Slug:  demoEmailSlug("profile", "complete-profile", "es"),
			Title: "Complete Profile",
			Comment: demoComment{
				ReviewBlock:  "proof_point",
				SelectedText: "revisar el progreso",
				StartOffset:  19,
				EndOffset:    38,
				Body:         "Resolved: proof point now matches the profile completion step.",
				Status:       "resolved",
				Severity:     "suggestion",
				ResolvedBy:   admin.ID,
			},
			EventKey: "complete-profile-approved",
		},
		{
			Slug:  demoEmailSlug("integration", "sync-settings", "es"),
			Title: "Sync Settings",
			Comment: demoComment{
				ReviewBlock:  "risk_copy",
				SelectedText: "estados de revisión",
				StartOffset:  108,
				EndOffset:    127,
				Body:         "Legal wants this automation claim softened before it is ready.",
				Status:       "open",
				Severity:     "issue",
			},
			EventKey: "sync-settings-changes-requested",
		},
		{
			Slug:  demoEmailSlug("first-value", "first-report", "es"),
			Title: "First Report",
			Comment: demoComment{
				ReviewBlock:  "primary_cta",
				SelectedText: "Abrir el siguiente paso",
				StartOffset:  0,
				EndOffset:    23,
				Body:         "Approved: CTA is actionable and fits the first-report handoff.",
				Status:       "resolved",
				Severity:     "suggestion",
				ResolvedBy:   admin.ID,
			},
			EventKey: "first-report-production-approved",
		},
		{
			Slug:  demoEmailSlug("retention", "renewal-reminder", "es"),
			Title: "Renewal Reminder",
			Comment: demoComment{
				ReviewBlock:  "secondary_detail",
				SelectedText: "datos de producción",
				StartOffset:  65,
				EndOffset:    84,
				Body:         "Confirm this disclaimer stays visible in every language.",
				Status:       "open",
				Severity:     "issue",
			},
			EventKey: "renewal-reminder-stale",
		},
	}

	for _, scenario := range scenarios {
		emailID, ok := emailIDs[scenario.Slug]
		if !ok {
			return fmt.Errorf("supporting demo email %s was not seeded", scenario.Slug)
		}
		if err := ensureDemoComment(ctx, tx, emailID, reviewer, scenario.Comment); err != nil {
			return err
		}
		if err := insertDemoEventIfMissing(ctx, tx, demoEvent{
			Key:        scenario.EventKey,
			Actor:      reviewer,
			Action:     "comment_created",
			EmailID:    emailID,
			EmailSlug:  board.slug(scenario.Slug),
			EmailTitle: scenario.Title,
			Metadata: map[string]any{
				"review_block": scenario.Comment.ReviewBlock,
				"severity":     scenario.Comment.Severity,
			},
			Changes: map[string]any{},
		}); err != nil {
			return err
		}
	}

	return nil
}

type demoComment struct {
	ReviewBlock  string
	SelectedText string
	StartOffset  int
	EndOffset    int
	Body         string
	Status       string
	Severity     string
	ResolvedBy   string
}

func ensureDemoComment(ctx context.Context, tx pgx.Tx, emailID string, actor demoActor, comment demoComment) error {
	var existingID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM comments
		WHERE email_id = $1
			AND review_block = $2
			AND body = $3
		LIMIT 1;
	`, emailID, comment.ReviewBlock, comment.Body).Scan(&existingID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		return nil
	}

	resolvedBy := any(nil)
	if comment.Status == "resolved" {
		resolvedBy = comment.ResolvedBy
	}

	var commentID string
	if comment.Status == "resolved" {
		err = tx.QueryRow(ctx, `
			INSERT INTO comments (
				email_id,
				user_id,
				review_block,
				selected_text,
				start_offset,
				end_offset,
				body,
				status,
				severity,
				resolved_by,
				resolved_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
			RETURNING id::text;
		`, emailID, actor.ID, comment.ReviewBlock, comment.SelectedText, comment.StartOffset, comment.EndOffset, comment.Body, comment.Status, comment.Severity, resolvedBy).Scan(&commentID)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO comments (
				email_id,
				user_id,
				review_block,
				selected_text,
				start_offset,
				end_offset,
				body,
				status,
				severity,
				resolved_by,
				resolved_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, NULL)
			RETURNING id::text;
		`, emailID, actor.ID, comment.ReviewBlock, comment.SelectedText, comment.StartOffset, comment.EndOffset, comment.Body, comment.Status, comment.Severity).Scan(&commentID)
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO comment_messages (comment_id, user_id, body)
		VALUES ($1, $2, $3);
	`, commentID, actor.ID, comment.Body)
	return err
}

type demoEvent struct {
	Key        string
	Actor      demoActor
	Action     string
	EmailID    string
	EmailSlug  string
	EmailTitle string
	Metadata   map[string]any
	Changes    map[string]any
}

func insertDemoEventIfMissing(ctx context.Context, tx pgx.Tx, event demoEvent) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM email_events
			WHERE metadata->>'demo_event_key' = $1
				AND email_slug = $2
		);
	`, event.Key, event.EmailSlug).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	metadata := map[string]any{"demo_event_key": event.Key}
	for key, value := range event.Metadata {
		metadata[key] = value
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	changesJSON, err := json.Marshal(event.Changes)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO email_events (
			actor_user_id,
			actor_email,
			action,
			email_id,
			email_slug,
			email_title,
			metadata,
			changes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb);
	`, event.Actor.ID, event.Actor.Email, event.Action, event.EmailID, event.EmailSlug, event.EmailTitle, metadataJSON, changesJSON)
	return err
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

func stringFromPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func contentHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
