package prompts

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// SentimentReplyInput is the neutral input for BuildSentimentReplyPrompt.
// Called for sentiment-routed workflows — when a workflow is configured to
// reply to positive XOR negative comments only (the auto-generated
// "Positive Auto-Reply" / "Negative Auto-Reply" workflows in particular).
type SentimentReplyInput struct {
	UserID     int
	BusinessID int

	// IsPositiveReply / IsNegativeReply describe which sentiment side this
	// workflow handles. Exactly one should be true.
	IsPositiveReply bool
	IsNegativeReply bool

	// AISettings is the raw workflow_manager.ai_settings JSON. The sentiment
	// builder pulls positive_response_styles, negative_response_styles, and
	// the engagement thresholds out of it.
	AISettings sql.NullString

	CommentText string
	PostContent sql.NullString

	Tone           string
	ResponseLength string
	UseEmojis      bool
	UseHashtags    bool

	UserPrompt   sql.NullString
	UserResponse sql.NullString

	BusinessName    sql.NullString
	BusinessAddress sql.NullString
	BusinessWebsite sql.NullString
	BusinessPhone   sql.NullString
}

// BuildSentimentReplyPrompt assembles the prompt for sentiment-routed
// workflows. Mirrors BuildUserDefinedReplyPrompt's safety + language gates
// so behavior is consistent across all auto-reply paths.
func BuildSentimentReplyPrompt(db *sql.DB, in SentimentReplyInput) (string, error) {
	var aiSettings struct {
		PositiveResponseStyles      []string `json:"positive_response_styles"`
		NegativeResponseStyles      []string `json:"negative_response_styles"`
		PositiveEngagementThreshold string   `json:"positive_engagement_threshold"`
		NegativeEngagementThreshold string   `json:"negative_engagement_threshold"`
	}
	if in.AISettings.Valid && in.AISettings.String != "" {
		if err := json.Unmarshal([]byte(in.AISettings.String), &aiSettings); err != nil {
			return "", fmt.Errorf("failed to parse ai_settings JSON: %w", err)
		}
	}

	sentiment := "positive"
	if in.IsNegativeReply {
		sentiment = "negative"
	}

	tone := in.Tone
	if tone == "" {
		tone = "friendly"
	}
	responseLength := in.ResponseLength
	if responseLength == "" {
		responseLength = "medium"
	}

	var b strings.Builder

	// Reply in the comment's language. We no longer gate cross-language
	// replies — the business owner explicitly wants replies wherever the
	// commenter happens to be writing from.
	b.WriteString("LANGUAGE: Detect the primary language of the COMMENT (ignore emojis, hashtags, URLs, and proper nouns) and write the entire reply in that language. No mixing.\n\n")

	b.WriteString("You are replying to a social media comment (e.g., Facebook, Instagram, TikTok) on behalf of a business.\n")
	b.WriteString(fmt.Sprintf("The comment has been identified as %s. Generate a response that:\n", sentiment))
	b.WriteString(fmt.Sprintf("- Uses a *%s* tone.\n", tone))
	b.WriteString(fmt.Sprintf("- Is *%s* in length (short ≈ 50 words, medium ≈ 100 words, long ≈ 150 words).\n", responseLength))
	if in.UseEmojis {
		b.WriteString(fmt.Sprintf("- Includes relevant emojis, if appropriate for the %s sentiment.\n", sentiment))
	}
	if in.UseHashtags {
		b.WriteString(fmt.Sprintf("- Includes relevant hashtags, if appropriate for the %s sentiment.\n", sentiment))
	}
	b.WriteString(fmt.Sprintf("- Addresses the %s sentiment and intent of the user's comment specifically.\n", sentiment))
	b.WriteString("- Remains factual, helpful, and aligned with the business's provided information.\n")

	if in.IsPositiveReply && len(aiSettings.PositiveResponseStyles) > 0 {
		b.WriteString("- Incorporates the following positive response styles: ")
		for i, style := range aiSettings.PositiveResponseStyles {
			if i > 0 {
				b.WriteString(", ")
			}
			switch style {
			case "thank":
				b.WriteString("express gratitude")
			case "engage":
				b.WriteString("encourage further interaction")
			case "personalized_message":
				b.WriteString("include a personalized touch")
			default:
				b.WriteString(style)
			}
		}
		b.WriteString(".\n")
		if aiSettings.PositiveEngagementThreshold != "" {
			engagementDesc := map[string]string{
				"high":   "highly enthusiastic, very positive tone, or strong praise",
				"medium": "moderately positive tone, general appreciation",
				"any":    "any level of positive sentiment",
			}
			desc, ok := engagementDesc[aiSettings.PositiveEngagementThreshold]
			if !ok {
				desc = "any level of positive sentiment"
			}
			b.WriteString(fmt.Sprintf("- Targets comments with %s positive engagement.\n", desc))
		}
	} else if in.IsNegativeReply && len(aiSettings.NegativeResponseStyles) > 0 {
		b.WriteString("- Incorporates the following negative response styles: ")
		for i, style := range aiSettings.NegativeResponseStyles {
			if i > 0 {
				b.WriteString(", ")
			}
			switch style {
			case "apologize":
				b.WriteString("offer an apology")
			case "request_details":
				b.WriteString("request more details to understand the issue")
			case "escalate":
				b.WriteString("indicate escalation to the team")
			case "personalized_message":
				b.WriteString("include a personalized touch")
			default:
				b.WriteString(style)
			}
		}
		b.WriteString(".\n")
		if aiSettings.NegativeEngagementThreshold != "" {
			engagementDesc := map[string]string{
				"high":   "strong dissatisfaction, anger, or significant criticism",
				"medium": "moderate dissatisfaction or mild criticism",
				"any":    "any level of negative sentiment",
			}
			desc, ok := engagementDesc[aiSettings.NegativeEngagementThreshold]
			if !ok {
				desc = "any level of negative sentiment"
			}
			b.WriteString(fmt.Sprintf("- Targets comments with %s negative engagement.\n", desc))
		}
	}

	b.WriteString("\n--- Business Information ---\n")
	if in.BusinessName.Valid {
		b.WriteString(fmt.Sprintf("- Name: %s\n", in.BusinessName.String))
	}
	if in.BusinessAddress.Valid {
		b.WriteString(fmt.Sprintf("- Address: %s\n", in.BusinessAddress.String))
	}
	if in.BusinessWebsite.Valid {
		b.WriteString(fmt.Sprintf("- Website: %s\n", in.BusinessWebsite.String))
	}
	if in.BusinessPhone.Valid {
		b.WriteString(fmt.Sprintf("- Phone: %s\n", in.BusinessPhone.String))
	}

	b.WriteString("\n--- Original Post ---\n")
	if in.PostContent.Valid && in.PostContent.String != "" {
		b.WriteString(fmt.Sprintf("%q\n", in.PostContent.String))
	} else {
		b.WriteString("No specific post was provided. Assume it's about the business's general offerings or promotions.\n")
	}

	b.WriteString("\n--- Comment to Respond To ---\n")
	b.WriteString(fmt.Sprintf("%q\n", in.CommentText))

	if in.UserPrompt.Valid && in.UserPrompt.String != "" {
		b.WriteString("\n--- Business Context for This Response ---\n")
		b.WriteString(in.UserPrompt.String)
		b.WriteString("\n")
	}

	if in.UserResponse.Valid && in.UserResponse.String != "" {
		b.WriteString("\n--- Example of a Suitable Reply Style ---\n")
		b.WriteString(in.UserResponse.String)
		b.WriteString("\n")
	}

	if learned := GetLearnedPreferencesBlock(db, in.UserID, in.BusinessID); learned != "" {
		b.WriteString(learned)
		b.WriteString("\n")
	}

	b.WriteString("\n--- Output Format ---\n")
	b.WriteString("Return the response as a valid JSON object with a single field `response` containing the reply text. Example:\n")
	b.WriteString("{\"response\": \"Your reply text here.\"}\n")
	b.WriteString("Ensure the JSON is valid, contains only the `response` field, and is not wrapped in markdown code blocks or any other text.\n")
	b.WriteString("If you cannot write a relevant, truthful response, return: {\"response\": \"\"}\n")

	b.WriteString("\nFinal Instructions:\n")
	b.WriteString(fmt.Sprintf("1. Generate a concise, accurate, and engaging reply that addresses the %s sentiment of the comment.\n", sentiment))
	b.WriteString("2. NEVER fabricate or invent details (hours, menu items, prices, locations, products, services, names, facts) that are NOT explicitly provided in the business information above.\n")
	b.WriteString("3. If the comment is hateful, threatening, political, spam, gibberish, or otherwise not worth responding to — return an empty response: {\"response\": \"\"}\n")
	b.WriteString("4. Avoid placeholders like [insert hours here]; the response should feel human-written.\n")
	b.WriteString(fmt.Sprintf("5. Ensure the response is appropriate for the %s sentiment and aligns with the specified styles and engagement level.\n", sentiment))
	b.WriteString("6. Write the reply in the same language as the COMMENT — never reply in a different language than the commenter used.\n")

	return b.String(), nil
}
