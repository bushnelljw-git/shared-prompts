// Package prompts provides shared prompt-building logic for review response generation.
//
// WHY THIS PACKAGE EXISTS:
// Both reviewprocessor and fetcher need to generate AI-powered review responses.
// Previously, each service had its own copy of the prompt-building code, which led to
// drift — the reviewprocessor prompt evolved with richer instructions (name personalization,
// rating-only handling, 5-star acknowledgment, previous response avoidance) while the
// fetcher's copy stayed simpler and outdated. When a business owner trains the AI via the
// Review Simulator, those learned preferences must apply identically whether the response
// is generated interactively (reviewprocessor) or by the auto-reply worker (fetcher).
//
// This shared module is the SINGLE SOURCE OF TRUTH for all review response prompting.
// If you need to change how review responses are generated, change it HERE — not in
// reviewprocessor or fetcher. Both services import this package.
package prompts

import (
	"database/sql"
	"fmt"
	"strings"
)

// GetLearnedPreferencesBlock fetches accumulated user feedback from the
// review_response_feedback table and builds a prompt block that teaches the LLM
// what the business owner wants. Returns empty string if no feedback exists.
//
// This is the core of the "training loop": business owners use the Review Simulator
// to critique AI-generated responses, and that feedback is stored in the database.
// Every future response — whether from the simulator, the "Pick a Response" modal,
// or the auto-reply worker — includes this block so the LLM learns the owner's style.
func GetLearnedPreferencesBlock(db *sql.DB, userID, businessID int) string {
	rows, err := db.Query(
		`SELECT review_text, ai_response, feedback_text
		 FROM review_response_feedback
		 WHERE user_id = ? AND business_id = ?
		 ORDER BY created_at DESC
		 LIMIT 10`,
		userID, businessID,
	)
	if err != nil {
		// Caller should log if needed; we return empty to degrade gracefully
		return ""
	}
	defer rows.Close()

	var examples []string
	for rows.Next() {
		var reviewText, aiResponse, feedbackText string
		if err := rows.Scan(&reviewText, &aiResponse, &feedbackText); err != nil {
			continue
		}
		// Truncate long texts to keep prompt size reasonable
		if len(reviewText) > 150 {
			reviewText = reviewText[:150] + "..."
		}
		if len(aiResponse) > 200 {
			aiResponse = aiResponse[:200] + "..."
		}
		examples = append(examples, fmt.Sprintf(
			"- Review: %q → AI wrote: %q → Owner said: %q",
			reviewText, aiResponse, feedbackText,
		))
	}

	if len(examples) == 0 {
		return ""
	}

	return fmt.Sprintf(`

LEARNED PREFERENCES FROM BUSINESS OWNER:
The business owner has reviewed past AI responses and provided specific feedback. You MUST incorporate these preferences into your response style:

%s

Apply these preferences to shape your tone, wording, and approach. These are direct instructions from the business owner about how they want their responses to sound.`, strings.Join(examples, "\n"))
}
