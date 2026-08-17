package prompts

import "strings"

// NoEmDashRuleBlock is the one wording of the product's hardest style rule:
// a reply that goes out under the owner's name never contains an em dash.
//
// It lives here, alone, because the rule is not about reviews or about
// comments — it is about anything a customer reads with the business's name on
// it. Every reply builder in this package renders this block, and the comment
// drafter in reviewprocessor renders the same text, so a business cannot get a
// dash-free review reply and a dashed comment reply on the same afternoon.
//
// Why it is a BLOCK and not one line at the bottom: the ban used to be item
// four of seven inside a section about JSON validity, and the model treated it
// like a formatting footnote. Live drafts came back with "Thanks for the 5
// stars—delicious food and a comeback made our day!". A rule the model is meant
// to obey every single time has to read like a rule, be stated early, name the
// character, show the fix, and ask for a re-read.
//
// signoff is the owner's signature line. It is usually "— Dana, Owner", which
// is the one place a dash is not the model's choice to make, so passing it here
// rewrites the ban to carry an explicit exception. Pass "" when the caller has
// no sign-off concept at all.
func NoEmDashRuleBlock(signoff string) string {
	var b strings.Builder

	b.WriteString("CRITICAL RULE - PUNCTUATION. THIS IS NOT OPTIONAL AND IT OVERRIDES STYLE:\n")
	b.WriteString("- NEVER use em dashes (—). Not one, nowhere, for no reason.\n")
	b.WriteString("- NEVER use en dashes (–) or semicolons (;) either.\n")
	b.WriteString("- Use a comma, a period, or simply two short sentences instead.\n")
	b.WriteString("- WRONG: \"Thanks for the 5 stars—delicious food and a comeback made our day!\"\n")
	b.WriteString("- RIGHT: \"Thanks for the 5 stars! Delicious food and a comeback made our day.\"\n")
	b.WriteString("- The only punctuation allowed is periods, commas, exclamation points, question marks, apostrophes and ordinary hyphens inside a hyphenated word (5-star, mac-and-cheese).\n")

	if strings.TrimSpace(signoff) != "" {
		// The sign-off is the owner's own signature, reproduced verbatim. Banning
		// its dash would mean silently editing a business's name for it.
		b.WriteString("- The single exception is the mandatory sign-off line specified below, which is reproduced exactly as given. Nothing except the sign-off may contain a dash of any kind.\n")
	}

	b.WriteString("- Before you output anything, re-read every reply you wrote and rewrite any sentence containing a — or a – so it uses a comma, a period, or two sentences.\n")

	return b.String()
}

// NoEmDashReminder is the short restatement for the tail of a prompt, where the
// final checklist lives. The block above teaches the rule; this is the last
// thing the model reads before it writes.
func NoEmDashReminder(signoff string) string {
	if strings.TrimSpace(signoff) != "" {
		return "- Re-read the CRITICAL RULE on punctuation: zero em dashes (—), zero en dashes (–), zero semicolons, anywhere except the sign-off line. Write naturally, like a real person would.\n"
	}
	return "- Re-read the CRITICAL RULE on punctuation: zero em dashes (—), zero en dashes (–), zero semicolons, anywhere in the output. Write naturally, like a real person would.\n"
}
