package emails

import (
	"encoding/json"
	"time"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/core/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailblocks"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/emailtext"
)

type AuthUser = auth.AuthUser

type EmailListItem struct {
	ID              string  `json:"id"`
	Sequence        string  `json:"sequence"`
	Title           string  `json:"title"`
	Subject         *string `json:"subject"`
	Preheader       *string `json:"preheader"`
	SendTiming      *string `json:"send_timing"`
	SendCondition   *string `json:"send_condition"`
	Stage           string  `json:"stage"`
	SortOrder       int     `json:"sort_order"`
	Language        string  `json:"language"`
	Variant         string  `json:"variant"`
	AdaptationKey   string  `json:"adaptation_key"`
	AdaptationLabel string  `json:"adaptation_label"`
	// RegionKey is the board region the email belongs to; nil when not set.
	RegionKey                *string `json:"region_key"`
	ReviewStatus             string  `json:"review_status"`
	OwnerEmail               *string `json:"owner_email"`
	ReviewerEmail            *string `json:"reviewer_email"`
	DueDate                  *string `json:"due_date"`
	ImplementationNotes      *string `json:"implementation_notes"`
	OpenCommentCount         int     `json:"open_comment_count"`
	OpenBlockingCommentCount int     `json:"open_blocking_comment_count"`
	// From the last portal sync: how often the robots sent this email and
	// when last; zero when it was not seen. LiveMarkedAt is a manual mark.
	// TranslationOf is the EN master this email was translated from;
	// TranslationStale is true when the master's text changed since.
	TranslationOf    *string `json:"translation_of"`
	TranslationStale bool    `json:"translation_stale"`
	// BasedOn is the reference email a region's email adapts; BasedOnStale
	// is true when the reference's text changed since it last matched.
	BasedOn          *string    `json:"based_on"`
	BasedOnStale     bool       `json:"based_on_stale"`
	PortalSendCount  int        `json:"portal_send_count"`
	PortalLastSentAt *time.Time `json:"portal_last_sent_at"`
	LiveMarkedAt     *time.Time `json:"live_marked_at"`
	// LatestVersion is the current content version; UploadedVersion is the
	// oldest one a super admin pasted into one of its Bitrix24 robots, if any.
	LatestVersion   *int `json:"latest_version"`
	UploadedVersion *int `json:"uploaded_version"`
	// UploadedAt is when the first robot was marked as pasted; the board
	// works out from it when the first send can show on the portal.
	UploadedAt *time.Time `json:"uploaded_at"`
	// UploadRobots are the email's robots, each with its own upload mark.
	UploadRobots []RobotUpload `json:"upload_robots"`
	// Set only for archived emails (the board's history).
	ArchivedAt      *time.Time `json:"archived_at"`
	ArchivedByEmail *string    `json:"archived_by_email"`
}

type EmailDetail struct {
	ID                       string          `json:"id"`
	Slug                     string          `json:"slug"`
	Sequence                 string          `json:"sequence"`
	Title                    string          `json:"title"`
	Subject                  *string         `json:"subject"`
	Preheader                *string         `json:"preheader"`
	SendTiming               *string         `json:"send_timing"`
	SendCondition            *string         `json:"send_condition"`
	Stage                    string          `json:"stage"`
	SortOrder                int             `json:"sort_order"`
	Language                 string          `json:"language"`
	Variant                  string          `json:"variant"`
	AdaptationKey            string          `json:"adaptation_key"`
	AdaptationLabel          string          `json:"adaptation_label"`
	RegionKey                *string         `json:"region_key"`
	TranslationOf            *string         `json:"translation_of"`
	BasedOn                  *string         `json:"based_on"`
	ReviewStatus             string          `json:"review_status"`
	OwnerEmail               *string         `json:"owner_email"`
	ReviewerEmail            *string         `json:"reviewer_email"`
	DueDate                  *string         `json:"due_date"`
	ImplementationNotes      *string         `json:"implementation_notes"`
	OpenCommentCount         int             `json:"open_comment_count"`
	OpenBlockingCommentCount int             `json:"open_blocking_comment_count"`
	LiveMarkedAt             *time.Time      `json:"live_marked_at"`
	LiveMarkedBy             *string         `json:"live_marked_by"`
	LiveNote                 *string         `json:"live_note"`
	LatestVersion            *int            `json:"latest_version"`
	UploadedVersion          *int            `json:"uploaded_version"`
	UploadedAt               *time.Time      `json:"uploaded_at"`
	UploadRobots             []RobotUpload   `json:"upload_robots"`
	ArchivedAt               *time.Time      `json:"archived_at"`
	ArchivedByEmail          *string         `json:"archived_by_email"`
	BodyText                 *string         `json:"-"`
	ContentParts             *string         `json:"-"`
	UpdatedAt                time.Time       `json:"-"`
	OriginalHTML             string          `json:"original_html"`
	ReviewHTML               *string         `json:"review_html"`
	TemplateHTML             string          `json:"template_html"`
	TemplateHash             *string         `json:"template_hash"`
	TemplateVersion          *string         `json:"template_version"`
	EditableFields           json.RawMessage `json:"editable_fields"`
	// Layout is the blocks the email is built from; null for an email with
	// its own template.
	Layout emailblocks.Layout `json:"layout"`
	// Checks are the rules of form the email breaks; it cannot be marked
	// ready until they are fixed. Only the single-email endpoint fills them.
	Checks []emailblocks.Issue `json:"checks,omitempty"`
}

type EmailContentParts = emailtext.ContentParts
type LinkGroups = emailtext.LinkGroups

type EmailVersionListItem struct {
	ID                        string    `json:"id"`
	EmailID                   string    `json:"email_id"`
	VersionNumber             int       `json:"version_number"`
	CreatedAt                 time.Time `json:"created_at"`
	CreatedByUserID           *string   `json:"created_by_user_id"`
	CreatedByEmail            string    `json:"created_by_email"`
	Source                    string    `json:"source"`
	RestoredFromVersionID     *string   `json:"restored_from_version_id"`
	RestoredFromVersionNumber *int      `json:"restored_from_version_number"`
	ChangedFieldCount         int       `json:"changed_field_count"`
	ChangedMetadataCount      int       `json:"changed_metadata_count"`
	HTMLChanged               bool      `json:"html_changed"`
}

type EmailVersionDetail struct {
	EmailVersionListItem
	Title          string             `json:"title"`
	Subject        *string            `json:"subject"`
	Preheader      *string            `json:"preheader"`
	OriginalHTML   string             `json:"original_html"`
	TemplateHTML   string             `json:"template_html"`
	EditableFields json.RawMessage    `json:"editable_fields"`
	ReviewHTML     string             `json:"review_html"`
	Layout         emailblocks.Layout `json:"layout"`
}

type EmailEventItem struct {
	ID          string          `json:"id"`
	ActorUserID *string         `json:"actor_user_id"`
	ActorEmail  string          `json:"actor_email"`
	Action      string          `json:"action"`
	EmailID     *string         `json:"email_id"`
	EmailSlug   *string         `json:"email_slug"`
	EmailTitle  *string         `json:"email_title"`
	Metadata    json.RawMessage `json:"metadata"`
	Changes     json.RawMessage `json:"changes"`
	CreatedAt   time.Time       `json:"created_at"`
}

type EmailActivityItem struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	ActorEmail *string         `json:"actor_email"`
	CreatedAt  time.Time       `json:"created_at"`
	Summary    string          `json:"summary"`
	Metadata   json.RawMessage `json:"metadata"`
	Changes    json.RawMessage `json:"changes"`
}
