package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	xhtml "golang.org/x/net/html"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailedit"
)

// LayoutConversion is one email in the old format and what switching it to
// blocks gives.
type LayoutConversion struct {
	ID       string
	Place    string
	Title    string
	Language string
	Live     bool
	emailblocks.Conversion
	// LostComments are open comments on review blocks the new template does
	// not have and no block took the text of; MovedComments are comments,
	// open or resolved, that move to the block that did (see
	// emailblocks.Conversion.MovedReviewBlocks).
	LostComments  int
	MovedComments int
}

// LayoutFamily is an email in the old format with its active translations:
// they switch together, since a translation takes its blocks from the EN
// email.
type LayoutFamily struct {
	Master       LayoutConversion
	Translations []LayoutConversion
}

// Members are the master and its translations.
func (family LayoutFamily) Members() []LayoutConversion {
	return append([]LayoutConversion{family.Master}, family.Translations...)
}

// Blocker says why the family stays in the old format, or "" when it can
// switch. With strict, only emails that render the same HTML switch;
// without it, also those with the same text, links and images.
func (family LayoutFamily) Blocker(strict bool) string {
	for _, member := range family.Members() {
		prefix := ""
		if member.ID != family.Master.ID {
			prefix = strings.ToUpper(member.Language) + ": "
		}
		switch {
		case member.Match == emailblocks.MatchNone:
			return prefix + member.Reason
		case strict && member.Match != emailblocks.MatchSame:
			return prefix + "the " + member.Reason + " (switches with -allow-markup)"
		case member.LostComments > 0:
			return fmt.Sprintf("%s%d open comments on %s, which the blocks do not have",
				prefix, member.LostComments, strings.Join(member.LostReviewBlocks, ", "))
		case member.ID != family.Master.ID && !sameLayout(member.Layout, family.Master.Layout):
			// A translation takes its blocks from the EN email: one with
			// other blocks would lose them.
			return fmt.Sprintf("%sother blocks than EN: %s (EN: %s)", prefix,
				strings.Join(member.Layout.Keys(), " → "), strings.Join(family.Master.Layout.Keys(), " → "))
		}
	}
	return ""
}

// PlanLayoutConversion works out the blocks for every active email of the
// board in the old format and its translations. It reads only.
func PlanLayoutConversion(ctx context.Context, db emailEventExecutor, boardKey string) ([]LayoutFamily, error) {
	library, err := loadBlockLibrary(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT e.id, coalesce(e.translation_of::text, ''), e.title, lower(e.language),
			e.stage, e.sort_order, coalesce(e.variant, ''), coalesce(e.adaptation_key, ''),
			e.template_html, e.editable_fields, coalesce(e.subject, ''), coalesce(e.preheader, ''),
			`+emailLiveSQL+`,
			coalesce((SELECT array_agg(c.review_block) FROM comments c
				WHERE c.email_id = e.id AND c.status = 'open'), '{}'),
			coalesce((SELECT array_agg(c.review_block) FROM comments c
				WHERE c.email_id = e.id), '{}')
		FROM emails e
		WHERE e.sequence = $1
			AND e.archived_at IS NULL
			AND e.layout IS NULL
		ORDER BY e.stage, e.sort_order, e.variant, e.language, e.created_at;
	`, boardKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		conversion    LayoutConversion
		translationOf string
		convert       func(emailblocks.ConvertOptions) emailblocks.Conversion
		comments      func()
	}
	all := []*row{}
	for rows.Next() {
		item := &row{}
		var stage, variant, adaptation, template, subject, preheader string
		var sortOrder int
		var fieldsJSON []byte
		var openComments, allComments []string
		if err := rows.Scan(&item.conversion.ID, &item.translationOf, &item.conversion.Title, &item.conversion.Language,
			&stage, &sortOrder, &variant, &adaptation, &template, &fieldsJSON, &subject, &preheader,
			&item.conversion.Live, &openComments, &allComments); err != nil {
			return nil, err
		}
		var fields emailedit.EditableFields
		if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
			return nil, fmt.Errorf("email %s editable fields: %w", item.conversion.ID, err)
		}
		item.conversion.Place = conversionPlace(stage, sortOrder, item.conversion.Language, variant, adaptation)
		item.convert = func(options emailblocks.ConvertOptions) emailblocks.Conversion {
			return emailblocks.ConvertWith(library, template, fields,
				emailedit.RenderMetadata{Subject: subject, Preheader: preheader}, options)
		}
		item.comments = func() {
			item.conversion.LostComments, item.conversion.MovedComments = 0, 0
			for _, block := range item.conversion.LostReviewBlocks {
				if _, moves := item.conversion.MovedReviewBlocks[block]; moves {
					item.conversion.MovedComments += count(allComments, block)
					continue
				}
				item.conversion.LostComments += count(openComments, block)
			}
		}
		item.conversion.Conversion = item.convert(emailblocks.ConvertOptions{})
		item.comments()
		all = append(all, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	masters := map[string]*row{}
	for _, item := range all {
		if item.translationOf == "" {
			masters[item.conversion.ID] = item
		}
	}
	// Most translations sign in one field like EN; one that signs with a
	// greeting and sender in two takes EN's one-field signature, so it gets
	// EN's blocks.
	for _, item := range all {
		master := masters[item.translationOf]
		if master == nil || sameLayout(item.conversion.Layout, master.conversion.Layout) ||
			!master.conversion.Layout.Uses("signature_text") || !item.conversion.Layout.Uses("signature") {
			continue
		}
		if aligned := item.convert(emailblocks.ConvertOptions{OneFieldSignature: true}); sameLayout(aligned.Layout, master.conversion.Layout) {
			item.conversion.Conversion = aligned
			item.comments()
		}
	}

	families := []LayoutFamily{}
	index := map[string]int{}
	for _, item := range all {
		if item.translationOf == "" {
			index[item.conversion.ID] = len(families)
			families = append(families, LayoutFamily{Master: item.conversion})
		}
	}
	for _, item := range all {
		if i, ok := index[item.translationOf]; ok {
			families[i].Translations = append(families[i].Translations, item.conversion)
		}
	}
	return families, nil
}

func count(values []string, value string) int {
	n := 0
	for _, v := range values {
		if v == value {
			n++
		}
	}
	return n
}

func conversionPlace(stage string, sortOrder int, language, variant, adaptation string) string {
	place := fmt.Sprintf("%s #%d · %s", stage, sortOrder, strings.ToUpper(language))
	if variant != "" && variant != "new" {
		place += " · " + variant
	}
	if adaptation != "" && adaptation != "default" {
		place += " · " + adaptation
	}
	return place
}

// LayoutConversionOptions say how ApplyLayoutConversion switches emails.
type LayoutConversionOptions struct {
	// Strict switches only emails that render the same HTML.
	Strict bool
	// KeepUpload moves the upload mark of an email whose robot has its
	// latest version to the version the switch writes: readers get the same
	// text, links and images, so the robot need not be pasted again; the new
	// markup goes in with the next real edit.
	KeepUpload bool
}

// ApplyLayoutConversion switches the families that can switch to blocks and
// returns how many emails switched and how many kept their upload mark.
// Every email keeps its status: it renders as before. The translations go
// first with their own fields, so the EN email does not hand them its text.
func ApplyLayoutConversion(ctx context.Context, tx pgx.Tx, actor AuthUser, families []LayoutFamily, options LayoutConversionOptions) (switched, keptUpload int, err error) {
	strict := options.Strict
	for _, family := range families {
		if family.Blocker(strict) != "" {
			continue
		}
		// The latest version of emails whose robots have it, before the
		// switch writes a new one.
		robotCurrent := map[string]int{}
		for _, member := range family.Members() {
			if robotCurrent[member.ID], err = robotsLatestVersion(ctx, tx, member.ID); err != nil {
				return 0, 0, err
			}
		}
		for _, member := range append(family.Translations, family.Master) {
			_, err := saveEmailContent(ctx, tx, actor, member.ID,
				emailContentUpdate{Layout: member.Layout, EditableFields: member.Fields},
				emailContentSaveOptions{
					Source:     "manual",
					KeepStatus: true,
					SameText:   true,
					Metadata:   map[string]any{"converted_to_blocks": string(member.Match)},
				})
			var contentErr emailContentError
			if errors.As(err, &contentErr) {
				return 0, 0, fmt.Errorf("%s: %s", member.Place, contentErr.message)
			}
			if err != nil {
				return 0, 0, fmt.Errorf("%s: %w", member.Place, err)
			}
			if err := moveComments(ctx, tx, member.ID, member.MovedReviewBlocks); err != nil {
				return 0, 0, fmt.Errorf("%s comments: %w", member.Place, err)
			}
			switched++
		}
		if err := syncTranslationLayouts(ctx, tx, actor, family.Master.ID); err != nil {
			return 0, 0, fmt.Errorf("%s translations: %w", family.Master.Place, err)
		}
		// After the translations follow EN, which may write their version.
		for _, member := range family.Members() {
			if !options.KeepUpload || robotCurrent[member.ID] == 0 {
				continue
			}
			if err := keepUploadMark(ctx, tx, actor, member.ID, robotCurrent[member.ID]); err != nil {
				return 0, 0, fmt.Errorf("%s upload mark: %w", member.Place, err)
			}
			keptUpload++
		}
	}
	return switched, keptUpload, nil
}

// robotsLatestVersion returns the email's latest version when a robot has
// it, else 0.
func robotsLatestVersion(ctx context.Context, tx pgx.Tx, emailID string) (int, error) {
	var version int
	err := tx.QueryRow(ctx, `
		SELECT coalesce(max(u.version), 0)
		FROM email_robot_uploads u
		WHERE u.email_id = $1
			AND u.version = (SELECT max(version_number) FROM email_versions v WHERE v.email_id = $1);
	`, emailID).Scan(&version)
	return version, err
}

// keepUploadMark moves the robots that had version from to the version the
// switch wrote; when and by whom they were pasted stay as they were.
func keepUploadMark(ctx context.Context, tx pgx.Tx, actor AuthUser, emailID string, from int) error {
	var slug, title string
	var to int
	var robots []string
	if err := tx.QueryRow(ctx, `
		WITH latest AS (SELECT max(version_number) AS version FROM email_versions WHERE email_id = $1),
		moved AS (
			UPDATE email_robot_uploads u
			SET version = latest.version
			FROM latest
			WHERE u.email_id = $1 AND u.version = $2
			RETURNING u.robot
		)
		SELECT e.slug, e.title, latest.version, coalesce((SELECT array_agg(robot ORDER BY robot) FROM moved), '{}')
		FROM emails e, latest
		WHERE e.id = $1;
	`, emailID, from).Scan(&slug, &title, &to, &robots); err != nil {
		return err
	}
	metadata := map[string]any{
		"version":      strconv.Itoa(to),
		"from_version": strconv.Itoa(from),
		"reason":       "converted_to_blocks",
	}
	if names := robotNames(robots); names != "" {
		metadata["robots"] = names
	}
	return InsertEmailEvent(ctx, tx, EmailEventParam{
		ActorUserID: actor.ID,
		ActorEmail:  actor.Email,
		Action:      emailEventUploaded,
		EmailID:     &emailID,
		EmailSlug:   &slug,
		EmailTitle:  &title,
		Metadata:    metadata,
	})
}

// moveComments puts the comments of review blocks the email no longer has on
// the block that took their text, and anchors each at its selected text in
// that block, or at the whole block when the text is not there any more.
func moveComments(ctx context.Context, tx pgx.Tx, emailID string, moved map[string]string) error {
	if len(moved) == 0 {
		return nil
	}
	var reviewHTML string
	if err := tx.QueryRow(ctx, `SELECT coalesce(review_html, '') FROM emails WHERE id = $1;`, emailID).Scan(&reviewHTML); err != nil {
		return err
	}
	// The preview counts offsets in the block's text as the browser reads
	// it, untrimmed.
	root, err := xhtml.Parse(strings.NewReader(reviewHTML))
	if err != nil {
		return err
	}
	texts := map[string]string{}
	var collect func(*xhtml.Node)
	collect = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			if block := htmlAttrValue(node, "data-review-block"); block != "" {
				if _, seen := texts[block]; !seen {
					texts[block] = htmlTextContent(node)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(root)
	for from, to := range moved {
		rows, err := tx.Query(ctx, `
			SELECT id, selected_text FROM comments WHERE email_id = $1 AND review_block = $2;
		`, emailID, from)
		if err != nil {
			return err
		}
		type comment struct{ id, selected string }
		comments := []comment{}
		for rows.Next() {
			var c comment
			if err := rows.Scan(&c.id, &c.selected); err != nil {
				rows.Close()
				return err
			}
			comments = append(comments, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		blockText := texts[to]
		for _, c := range comments {
			selected, start := c.selected, 0
			if at := strings.Index(blockText, c.selected); c.selected != "" && at >= 0 {
				start = jsStringLength(blockText[:at])
			} else {
				selected = blockText
			}
			if _, err := tx.Exec(ctx, `
				UPDATE comments
				SET review_block = $2, selected_text = $3, start_offset = $4, end_offset = $5
				WHERE id = $1;
			`, c.id, to, selected, start, start+jsStringLength(selected)); err != nil {
				return err
			}
		}
	}
	return nil
}
