package prompts

import (
	"database/sql"
	"fmt"
	"strings"
)

// UserDefinedReplyInput is the neutral input for BuildUserDefinedReplyPrompt.
//
// It is intentionally decoupled from any service's internal Workflow / Comment
// types so reviewprocessor and fetcher can both call this without sharing
// types.
type UserDefinedReplyInput struct {
	UserID     int
	BusinessID int

	// Comment & post under reply
	CommentText string
	PostContent sql.NullString

	// AI knobs
	Tone           string
	ResponseLength string
	UseEmojis      bool
	UseHashtags    bool

	// Workflow-supplied custom guidance
	UserPrompt   sql.NullString
	UserResponse sql.NullString

	// Business context (looked up by the caller from user_businesses)
	BusinessName    sql.NullString
	BusinessAddress sql.NullString
	BusinessWebsite sql.NullString
	BusinessPhone   sql.NullString

	// Optional supporting copy distinct from UserResponse — most callers will
	// just leave this empty and rely on UserResponse / UserPrompt.
	Details string
}

// BuildUserDefinedReplyPrompt assembles the prompt sent to the LLM when a
// business owner has authored a "user-defined" workflow (i.e. a custom
// auto-reply with their own instructions). Both reviewprocessor (simulator,
// preview) and fetcher (live posting) call this so that prompt content stays
// in lockstep across services.
func BuildUserDefinedReplyPrompt(db *sql.DB, in UserDefinedReplyInput) (string, error) {
	var b strings.Builder

	// Hard language rule first: applied before any other guidance so the
	// model gates cross-language replies up front instead of generating a
	// reply and then second-guessing it.
	b.WriteString("LANGUAGE RULE (apply BEFORE writing anything else):\n")
	b.WriteString("1. Identify the primary language of the ORIGINAL POST (ignore emojis, hashtags, URLs, and proper nouns).\n")
	b.WriteString("2. Identify the primary language of the COMMENT TO RESPOND TO (same rules).\n")
	b.WriteString("3. If those two languages do not match, you MUST return exactly {\"response\": \"\"} and stop. Do not translate, do not reply across languages.\n")
	b.WriteString("4. If they match, the entire reply MUST be written in that shared language. No mixing.\n\n")

	b.WriteString("You are replying to a comment left on a social media post (e.g., Facebook, Instagram, TikTok) on behalf of a business.\n")
	b.WriteString("Generate a response that:\n")
	tone := in.Tone
	if tone == "" {
		tone = "friendly"
	}
	responseLength := in.ResponseLength
	if responseLength == "" {
		responseLength = "medium"
	}
	b.WriteString(fmt.Sprintf("- Uses a *%s* tone.\n", tone))
	b.WriteString(fmt.Sprintf("- Is *%s* in length (short ≈ 50 words, medium ≈ 100 words, long ≈ 150 words).\n", responseLength))
	if in.UseEmojis {
		b.WriteString("- Includes relevant emojis, if appropriate.\n")
	}
	if in.UseHashtags {
		b.WriteString("- Includes relevant hashtags, if appropriate.\n")
	}
	b.WriteString("- Responds specifically to the content and intent of the user's comment.\n")
	b.WriteString("- Remains factual, helpful, and consistent with the business's provided information.\n")

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
	if in.Details != "" {
		b.WriteString(fmt.Sprintf("- Supporting Details: %s\n", in.Details))
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
	b.WriteString("1. Generate a concise, accurate, and engaging reply that incorporates the relevant information above.\n")
	b.WriteString("2. NEVER fabricate or invent details (hours, menu items, prices, locations, products, services, names, facts) that are NOT explicitly provided in the business information above. If you do not have a specific detail, do NOT include it.\n")
	b.WriteString("3. If the comment is hateful, threatening, political, spam, gibberish, or otherwise not worth responding to — return an empty response: {\"response\": \"\"}\n")
	b.WriteString("4. Use the supporting details IF it makes sense.\n")
	b.WriteString("5. Do not add placeholders like [insert hours here] — it should feel like a human responded.\n")
	b.WriteString("6. Keep the response relevant to the comment AND the original post context. Do not go off-topic.\n")
	b.WriteString("7. Re-check the LANGUAGE RULE at the top before emitting JSON. If the comment language does not match the post language, your output MUST be {\"response\": \"\"}.\n")

	return b.String(), nil
}
