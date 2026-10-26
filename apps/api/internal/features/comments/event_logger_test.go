package comments

import (
	"context"

	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
)

// Mirrors commentEventLogger in cmd/server, which tests cannot import.
type testEventLogger struct{}

func (testEventLogger) LogEvent(ctx context.Context, db EmailEventExecutor, actor AuthUser, action string, emailID string, emailSlug string, emailTitle string, metadata map[string]any) error {
	return emails.InsertEmailEvent(ctx, db, emails.EmailEventParam{
		ActorUserID: actor.ID,
		ActorEmail:  actor.Email,
		Action:      action,
		EmailID:     &emailID,
		EmailSlug:   &emailSlug,
		EmailTitle:  &emailTitle,
		Metadata:    metadata,
	})
}

func init() {
	Logger = testEventLogger{}
}
