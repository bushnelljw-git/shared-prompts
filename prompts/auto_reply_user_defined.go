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

	// MatchInstruction is an optional, owner-authored gate that runs BEFORE
	// reply generation. Use it to say things like "only reply if the
	// commenter typed 'CODE' in their comment" or "only reply when the
	// commenter is asking about pricing". When non-empty the LLM must
	// return an empty response if the comment doesn't match.
	MatchInstruction string
}

// BuildUserDefinedReplyPrompt assembles the prompt sent to the LLM when a
// business owner has authored a "user-defined" workflow (i.e. a custom
// auto-reply with their own instructions). Both reviewprocessor (simulator,
// preview) and fetcher (live posting) call this so that prompt content stays
// in lockstep across services.
func BuildUserDefinedReplyPrompt(db *sql.DB, in UserDefinedReplyInput) (string, error) {
	var b strings.Builder

	// Optional owner-authored match rule. When set, this is the FIRST gate
	// so we don't pay for an LLM completion just to throw it away.
	if strings.TrimSpace(in.MatchInstruction) != "" {
		b.WriteString("MATCH RULE — THIS OVERRIDES EVERYTHING ELSE:\n")
		b.WriteString("Step 1. Read the COMMENT below.\n")
		b.WriteString("Step 2. Decide whether the comment matches this criteria: \"")
		b.WriteString(strings.TrimSpace(in.MatchInstruction))
		b.WriteString("\".\n")
		b.WriteString("Step 3a. If it does NOT match, your ONLY valid output is {\"response\": \"\"}. Do NOT write a reply. Do NOT consider any of the business context, hours, menu, or owner instructions below. Match check fails → empty response. Stop.\n")
		b.WriteString("Step 3b. If it DOES match, continue with the rest of these instructions and write a reply.\n")
		b.WriteString("This rule takes priority over any other guidance about being helpful, answering questions, or thanking customers.\n\n")
	}

	// Language: reply in whatever language the comment is written in. The
	// LLM is good at language detection from a single message; we don't
	// need to gate cross-language any more — the business owner asked for
	// replies in any language the commenter uses.
	b.WriteString("LANGUAGE: Detect the primary language of the COMMENT (ignore emojis, hashtags, URLs, and proper nouns) and write the entire reply in that language. No mixing.\n\n")

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
	b.WriteString("7. Write the reply in the same language as the COMMENT — never reply in a different language than the commenter used.\n")
	if strings.TrimSpace(in.MatchInstruction) != "" {
		b.WriteString("8. Final self-check: re-read the MATCH RULE at the top. If the comment doesn't satisfy the criteria, your output MUST be {\"response\": \"\"} — do not reply just because you have answers to other questions.\n")
	}

	return b.String(), nil
}
