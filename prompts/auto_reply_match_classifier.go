package prompts

import (
	"fmt"
	"strings"
)

// BuildMatchClassifierPrompt produces a tiny YES/NO classification prompt
// used as a hard pre-LLM gate before generating a reply. Splitting the gate
// out from the reply prompt prevents the model from being "helpful" and
// answering off-criteria comments because the rest of the prompt told it
// how to respond to other scenarios.
//
// Callers should invoke their LLM with this prompt, look for a leading
// YES/NO token, and only proceed to the full reply prompt when the answer
// is YES.
func BuildMatchClassifierPrompt(criteria, commentText string) string {
	criteria = strings.TrimSpace(criteria)
	commentText = strings.TrimSpace(commentText)

	return fmt.Sprintf(`You are a strict matcher. Decide whether a single social media COMMENT satisfies a CRITERIA. Output exactly one token: YES or NO. No explanation.

CRITERIA: %q

COMMENT: %q

Rules:
- If the comment clearly satisfies the criteria, output YES.
- If the comment does not satisfy the criteria, output NO.
- If you are unsure, output NO.
- Treat language as irrelevant: a comment in any language can satisfy the criteria.
- Respond with the single token YES or NO and nothing else.

Answer:`, criteria, commentText)
}

// MatchClassifierAnswer interprets the LLM's response to a match classifier
// prompt. Returns true only if the response begins with an unambiguous YES.
// Anything else (including errors at the call site) should be treated as
// "did not match" by the caller.
func MatchClassifierAnswer(raw string) bool {
	t := strings.ToUpper(strings.TrimSpace(raw))
	t = strings.Trim(t, "\"' \t\n\r.")
	if t == "YES" {
		return true
	}
	// Tolerate "YES." / "YES,..." / "YES — because..." but require YES first.
	if strings.HasPrefix(t, "YES") && (len(t) == 3 || !isLetter(t[3])) {
		return true
	}
	return false
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
