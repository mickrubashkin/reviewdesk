package ai

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getCachedAIAnalysis(ctx context.Context, dbpool *pgxpool.Pool, email EmailDetail, aiService AIAnalysisService) (EmailAnalysis, bool, error) {
	var analysisText string

	err := dbpool.QueryRow(ctx, `
		SELECT analysis::text
		FROM ai_analysis_cache
		WHERE email_id = $1
			AND model = $2
			AND response_language = $3
			AND prompt_hash = $4
			AND updated_at > $5
		LIMIT 1;
	`,
		email.ID,
		aiService.Model,
		aiService.ResponseLanguage,
		aiService.PromptHash(),
		email.UpdatedAt,
	).Scan(&analysisText)
	if err != nil {
		if err == pgx.ErrNoRows {
			return EmailAnalysis{}, false, nil
		}
		return EmailAnalysis{}, false, err
	}

	var analysis EmailAnalysis
	if err := json.Unmarshal([]byte(analysisText), &analysis); err != nil {
		return EmailAnalysis{}, false, err
	}

	return analysis, true, nil
}

func upsertAIAnalysisCache(ctx context.Context, dbpool *pgxpool.Pool, emailID string, aiService AIAnalysisService, analysis EmailAnalysis) error {
	analysisBytes, err := json.Marshal(analysis)
	if err != nil {
		return err
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO ai_analysis_cache (
			email_id,
			model,
			response_language,
			prompt_hash,
			analysis
		)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (email_id, model, response_language, prompt_hash) DO UPDATE SET
			analysis = EXCLUDED.analysis,
			updated_at = now();
	`,
		emailID,
		aiService.Model,
		aiService.ResponseLanguage,
		aiService.PromptHash(),
		string(analysisBytes),
	)

	return err
}
