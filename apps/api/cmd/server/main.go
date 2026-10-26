package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/demo"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/ai"
	featureauth "github.com/mickrubashkin/reviewdesk/apps/api/internal/features/auth"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/boards"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/comments"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/emails"
	"github.com/mickrubashkin/reviewdesk/apps/api/internal/features/portal"
)

func main() {
	_ = godotenv.Load("../../.env")
	databaseURL := os.Getenv("DATABASE_URL")

	ctx := context.Background()
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	dbpool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	err = dbpool.Ping(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to ping database: %v\n", err)
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Use(featureauth.SameOriginMutationMiddleware)
	r.Use(requestIDMiddleware)
	r.Use(featureauth.Middleware(dbpool))
	demoEnabled := strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_DEMO_LOGIN_ENABLED")), "true")
	if demoEnabled {
		r.Use(demo.LimitVisitorWrites())
	}
	r.Use(operationalEventMiddleware(dbpool))
	aiService, err := ai.NewAIAnalysisService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create AI analysis service: %v\n", err)
		os.Exit(1)
	}

	registerHealthRoute(r, dbpool)
	featureauth.RegisterAuthRoutes(r, dbpool, emails.NewLoginCodeEmailSender())
	registerOperationalRoutes(r, dbpool)

	boards.Logger = serverEventLogger{}
	boards.CopyEmails = emails.CopyBoardEmailsTx
	boards.RegisterBoardRoutes(r, dbpool)

	if err := emails.EnsureDefaultBlocks(ctx, dbpool); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to add the default email blocks: %v\n", err)
		os.Exit(1)
	}
	emails.RegisterEmailRoutes(r, dbpool)
	comments.Logger = commentEventLogger{}
	comments.Mailer = emails.NewNotificationSender()
	comments.RegisterCommentRoutes(r, dbpool)
	ai.RegisterAIRoutes(r, dbpool, aiService)
	portal.RegisterRoutes(r, dbpool)
	if os.Getenv("PORTAL_AUTOSYNC_ENABLED") == "true" {
		go portal.RunAutoSync(ctx, dbpool)
	}
	// Each demo visitor's sandbox is removed a day after the visit.
	if demoEnabled {
		go demo.RunSandboxCleanup(ctx, dbpool)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("API_PORT")
	}
	if port == "" {
		port = "8080"
	}

	addr := ":" + port

	fmt.Printf("API server listening on %s\n", addr)
	logOperationalEvent(ctx, dbpool, operationalEvent{
		Level:     "info",
		EventType: "server_starting",
		Message:   "API server starting",
		Metadata: map[string]any{
			"port": port,
		},
	})
	err = http.ListenAndServe(addr, r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "API server failed: %v\n", err)
		os.Exit(1)
	}
}

type serverEventLogger struct{}

func (s serverEventLogger) LogEvent(ctx context.Context, db boards.EmailEventExecutor, actor boards.AuthUser, action string, board boards.BoardItem, metadata map[string]any, changes map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if _, ok := metadata["board_key"]; !ok {
		metadata["board_key"] = board.Key
	}
	if _, ok := metadata["board_name"]; !ok {
		metadata["board_name"] = board.Name
	}
	return emails.InsertEmailEvent(ctx, db, emails.EmailEventParam{
		ActorUserID: actor.ID,
		ActorEmail:  actor.Email,
		Action:      action,
		Metadata:    metadata,
		Changes:     changes,
	})
}

type commentEventLogger struct{}

func (c commentEventLogger) LogEvent(ctx context.Context, db comments.EmailEventExecutor, actor comments.AuthUser, action string, emailID string, emailSlug string, emailTitle string, metadata map[string]any) error {
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
