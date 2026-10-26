package boards

import (
	"context"
	"encoding/json"
)

// The server wires Logger in cmd/server; emails.InsertEmailEvent can't be used
// here (emails imports boards), so tests write the same row shape directly.
type testEventLogger struct{}

func (testEventLogger) LogEvent(ctx context.Context, db EmailEventExecutor, actor AuthUser, action string, board BoardItem, metadata map[string]any, changes map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if _, ok := metadata["board_key"]; !ok {
		metadata["board_key"] = board.Key
	}
	if _, ok := metadata["board_name"]; !ok {
		metadata["board_name"] = board.Name
	}
	if changes == nil {
		changes = map[string]any{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO email_events (actor_user_id, actor_email, action, metadata, changes)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb);
	`, actor.ID, actor.Email, action, metadataJSON, changesJSON)
	return err
}

func init() {
	Logger = testEventLogger{}
}
