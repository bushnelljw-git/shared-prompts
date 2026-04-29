package prompts

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// AutoReplyInput is the neutral input for BuildAutoReplyPrompt — the
// "auto-generated" path used by the platform-default Positive/Negative
// auto-reply workflows. Unlike user-defined or sentiment workflows, this
// builder hands off most of its work to BuildReviewResponsePrompt so the
// auto-reply voice stays consistent with the rest of the review responder.
type AutoReplyInput struct {
	UserID     int
	BusinessID int

	// Comment classification — "positive" or anything else (treated as negative).
	Classification string
	CommentText    string

	// Workflow knobs from ai_settings_socialmedia / workflow_manager.
	Tone        string
	UseEmojis   bool
	UseHashtags bool

	// JSON-encoded arrays of style keys per sentiment, as stored in
	// ai_settings_socialmedia.{positive,negative}_response_style.
	PositiveResponseStyle sql.NullString
	NegativeResponseStyle sql.NullString

	SpecialInstructions sql.NullString

	// Reward fields — populated when the workflow has a reward attached.
	HasReward              bool
	RewardText             sql.NullString
	RedemptionInstructions sql.NullString
}

// BuildAutoReplyPrompt is the shared prompt builder for auto-generated
// Positive/Negative auto-reply workflows. It adapts the inputs and
// delegates to BuildReviewResponsePrompt so the voice matches the rest of
// the review responder.
func BuildAutoReplyPrompt(db *sql.DB, in AutoReplyInput) (string, error) {
	var positiveStyles, negativeStyles []string
	if in.PositiveResponseStyle.Valid && in.PositiveResponseStyle.String != "" {
		_ = json.Unmarshal([]byte(in.PositiveResponseStyle.String), &positiveStyles)
	}
	if in.NegativeResponseStyle.Valid && in.NegativeResponseStyle.String != "" {
		_ = json.Unmarshal([]byte(in.NegativeResponseStyle.String), &negativeStyles)
	}

	isPositive := strings.ToLower(in.Classification) == "positive"
	sentiment := "negative"
	responseStyles := negativeStyles
	if isPositive {
		sentiment = "positive"
		responseStyles = positiveStyles
	}

	specialInstructions := ""
	if in.SpecialInstructions.Valid {
		specialInstructions = in.SpecialInstructions.String
	}

	reward := ""
	redemption := ""
	if in.HasReward && in.RewardText.Valid && in.RedemptionInstructions.Valid {
		reward = in.RewardText.String
		redemption = in.RedemptionInstructions.String
	}

	return BuildReviewResponsePrompt(ReviewResponseConfig{
		ReviewText:             in.CommentText,
		Sentiment:              sentiment,
		Tone:                   in.Tone,
		ResponseStyles:         responseStyles,
		Reward:                 reward,
		RedemptionInstructions: redemption,
		SpecialInstructions:    specialInstructions,
		LearnedPreferences:     GetLearnedPreferencesBlock(db, in.UserID, in.BusinessID),
		Length:                 "short",
		ResponseCount:          2,
	}), nil
}
