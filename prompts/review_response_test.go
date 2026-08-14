package prompts

import (
	"strings"
	"testing"
)

// The review-reply prompt is the single source of truth for three engines (the
// response picker, the public /respond page and the auto-reply worker), and the
// owner's saved preferences reach the model ONLY as text in this string. So the
// tests here are not about tidiness — every one of them pins a way the owner's
// setting used to be silently overruled.

func base() ReviewResponseConfig {
	return ReviewResponseConfig{
		ReviewText:    "Waited 25 minutes for a flat white and it came out cold.",
		Author:        "Marcus T.",
		Sentiment:     "negative",
		Tone:          "professional",
		Length:        "medium",
		Rating:        2,
		ResponseCount: 8,
	}
}

func mustNotContain(t *testing.T, out string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(out, n) {
			t.Errorf("prompt must NOT contain %q\n--- offending line(s) ---\n%s", n, linesWith(out, n))
		}
	}
}

func mustContain(t *testing.T, out string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(out, n) {
			t.Errorf("prompt must contain %q", n)
		}
	}
}

func linesWith(out, needle string) string {
	var hits []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, needle) {
			hits = append(hits, l)
		}
	}
	return strings.Join(hits, "\n")
}

// ── 1. Format artifacts ──────────────────────────────────────────────────────
// The builder used to be one fmt.Sprintf with 13 verbs and 12 args, so EVERY
// prompt shipped "%!s(int=8)" and "All %!d(MISSING) responses ...". go vet
// could not see it because the format string was non-constant.

func TestNoFormatArtifacts(t *testing.T) {
	cases := []ReviewResponseConfig{
		base(),
		{ReviewText: "", Rating: 5, Sentiment: "positive", ResponseCount: 8}, // rating-only + 5 star
		{ReviewText: "Great!", Rating: 5, Sentiment: "positive", ResponseCount: 2},
		{ReviewText: "Bad", Rating: 1, Sentiment: "negative", ResponseCount: 1},
		{ReviewText: "100% perfect, 50%% off?", Rating: 4, Sentiment: "positive"}, // percent signs in the review itself
		{ReviewText: "x", Author: "Emily", Sentiment: "positive", Rating: 5,
			ResponseStyles: []string{"thank", "engage", "personalized_message"},
			Reward:         "10% off", RedemptionInstructions: "Show this reply",
			SpecialInstructions: "Mention we are family-run (100% local)",
			Signoff:             "— Dana, Owner", EmojiPolicy: EmojiAllow,
			LearnedPreferences: "Owner likes: short replies", PreviousResponses: []string{"Thanks!"}},
	}
	for i, cfg := range cases {
		out := BuildReviewResponsePrompt(cfg)
		if strings.Contains(out, "%!") {
			t.Errorf("case %d: format artifact in prompt:\n%s", i, linesWith(out, "%!"))
		}
	}
}

// The count the model is told to produce must be the count it is asked for, in
// every place the number appears.
func TestResponseCountIsConsistent(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 8} {
		cfg := base()
		cfg.ResponseCount = n
		out := BuildReviewResponsePrompt(cfg)
		mustNotContain(t, out, "%!")
		// The JSON skeleton must declare exactly n fields.
		if got := strings.Count(out, "\"response"); got < n {
			t.Errorf("count %d: expected at least %d response keys, saw %d", n, n, got)
		}
		if strings.Contains(out, "response"+itoa(n+1)+"\"") {
			t.Errorf("count %d: prompt asks for a response%d it did not budget for", n, n+1)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ── 2. Chosen styles must not be contradicted ────────────────────────────────
// The prompt used to carry a hardcoded "KEEP IT SIMPLE AND GENERIC" block —
// "DO NOT call out specific actions or details from their review" — 30 lines
// BELOW the instruction asking for a personalized message. The owner ticked
// "Make it personal" and the prompt banned it.

func TestPersonalizedMessageIsNotBannedByGenericRules(t *testing.T) {
	cfg := base()
	cfg.ResponseStyles = []string{"apologize", "personalized_message"}
	out := BuildReviewResponsePrompt(cfg)

	mustNotContain(t, out,
		"DO NOT call out specific actions or details from their review",
		"DO NOT reference specific menu items or dishes they mentioned",
		"Keep responses GENERIC",
	)
	mustContain(t, out, "personalized")
}

// With no style that requires naming specifics (neither personalized_message
// nor apologize), the generic guidance is the right default and must survive —
// this is the legacy behaviour.
func TestGenericRulesSurviveWhenNoSpecificityIsRequired(t *testing.T) {
	cfg := base()
	cfg.ResponseStyles = []string{"request_details"}
	out := BuildReviewResponsePrompt(cfg)
	mustContain(t, out, "Keep responses GENERIC")
}

// "engage" means invite them back. The prompt used to ban exactly that with
// "DO NOT use pushy return language like 'come back' ... 'visit again'".
func TestEngageIsNotBannedByReturnLanguageRule(t *testing.T) {
	cfg := base()
	cfg.Sentiment = "positive"
	cfg.Rating = 5
	cfg.ResponseStyles = []string{"thank", "engage"}
	out := BuildReviewResponsePrompt(cfg)
	mustNotContain(t, out, "DO NOT use pushy return language")
	mustContain(t, out, "invite them back")
}

func TestReturnLanguageBanSurvivesWithoutEngage(t *testing.T) {
	cfg := base()
	cfg.Sentiment = "positive"
	cfg.ResponseStyles = []string{"thank"}
	out := BuildReviewResponsePrompt(cfg)
	mustContain(t, out, "DO NOT use pushy return language")
}

// The hardcoded "For POSITIVE reviews: Just thank them warmly / For NEGATIVE
// reviews: Apologize briefly" pair fired even when the owner had unticked
// "thank" or "apologize". It may only stand in when NO style was chosen.
func TestNoHardcodedThankWhenThankIsUnchosen(t *testing.T) {
	cfg := base()
	cfg.Sentiment = "positive"
	cfg.Rating = 5
	cfg.ResponseStyles = []string{"personalized_message"} // deliberately NOT "thank"
	out := BuildReviewResponsePrompt(cfg)
	mustNotContain(t, out, "Just thank them warmly")
}

func TestNoHardcodedApologyWhenApologizeIsUnchosen(t *testing.T) {
	cfg := base()
	cfg.ResponseStyles = []string{"request_details"} // deliberately NOT "apologize"
	out := BuildReviewResponsePrompt(cfg)
	mustNotContain(t, out, "Apologize briefly")
}

func TestFallbackGuidanceWhenNoStylesChosen(t *testing.T) {
	cfg := base()
	cfg.ResponseStyles = nil
	out := BuildReviewResponsePrompt(cfg)
	mustContain(t, out, "Apologize briefly")
}

// A rating-only review used to inject "invite them to share more details" for
// 1-3 stars whether or not the owner chose request_details.
func TestRatingOnlyDoesNotForceRequestDetails(t *testing.T) {
	cfg := base()
	cfg.ReviewText = ""
	cfg.Rating = 2
	cfg.ResponseStyles = []string{"apologize"}
	out := BuildReviewResponsePrompt(cfg)
	mustContain(t, out, "RATING-ONLY REVIEW")
	mustNotContain(t, out, "invite them to share more details")
}

// Personalization and the "no names or roles" ban collide when the reviewer
// praised a named member of staff: one says name the thing they wrote about,
// the other forbids it. The ban must be about people the reviewer did NOT
// mention — inventing a manager — not about echoing their own words.
func TestPersonalizationMayEchoAPersonTheReviewerNamed(t *testing.T) {
	cfg := base()
	cfg.Sentiment = "positive"
	cfg.ResponseStyles = []string{"thank", "personalized_message"}
	out := BuildReviewResponsePrompt(cfg)
	mustNotContain(t, out, "DO NOT mention specific people by name or role (manager, owner, staff names, etc.)\n")
	mustContain(t, out, "did not mention")
}

// "at least 2 of the 2 responses" is not a rule, it is every response — and it
// reads as broken. The share must leave room for the sentence's own caveat.
func TestFiveStarShareIsSaneForEveryCount(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 8} {
		cfg := base()
		cfg.Rating = 5
		cfg.Sentiment = "positive"
		cfg.ResponseCount = n
		out := BuildReviewResponsePrompt(cfg)
		if n > 1 && strings.Contains(out, "at least "+itoa(n)+" of the "+itoa(n)+" responses") {
			t.Errorf("count %d: five-star share asks for all of them while claiming not every one needs it", n)
		}
		mustNotContain(t, out, "at least 0 of")
	}
}

// Three consecutive newlines are a seam from an omitted optional block. They
// are harmless to a model and sloppy to a human reading the prompt in a log.
func TestNoTripleBlankLines(t *testing.T) {
	cfgs := []ReviewResponseConfig{base(), {ReviewText: "", Rating: 5, ResponseCount: 8}}
	for i, cfg := range cfgs {
		if strings.Contains(BuildReviewResponsePrompt(cfg), "\n\n\n") {
			t.Errorf("case %d: prompt contains a triple newline seam", i)
		}
	}
}

// Found by testing the live service, not by any earlier unit test. With
// `apologize` chosen but not `personalized_message`, the generic block still
// said "DO NOT call out specific actions or details from their review" — which
// forbids the apologize style's own "for the specific thing that went wrong".
// The observed reply to a 2-star complaint about a 40-minute wait and cold food
// opened "thanks for the feedback" and never apologised.
func TestApologizeIsNotBannedFromNamingWhatWentWrong(t *testing.T) {
	cfg := base()
	cfg.ResponseStyles = []string{"apologize", "request_details"}
	out := BuildReviewResponsePrompt(cfg)

	mustNotContain(t, out,
		"DO NOT call out specific actions or details from their review",
		"DO NOT reference specific menu items or dishes they mentioned",
		"Keep responses GENERIC",
	)
	mustContain(t, out, "Say sorry for the specific thing they described")
	// A vague apology is the failure mode actually observed, so name it.
	mustContain(t, out, "sorry you feel that way")
}

// The ❌ BAD examples punish specificity too, so they must not sit next to an
// instruction to name what went wrong.
func TestBadExamplesAreDroppedWhenSpecificityIsRequired(t *testing.T) {
	for _, styles := range [][]string{{"apologize"}, {"personalized_message"}, {"apologize", "personalized_message"}} {
		cfg := base()
		cfg.ResponseStyles = styles
		out := BuildReviewResponsePrompt(cfg)
		if strings.Contains(out, "EXAMPLES - WHAT NOT TO DO") {
			t.Errorf("styles %v: the anti-specificity examples are still present", styles)
		}
	}
	// With neither, they are the right house style and must survive.
	cfg := base()
	cfg.ResponseStyles = []string{"request_details"}
	mustContain(t, BuildReviewResponsePrompt(cfg), "EXAMPLES - WHAT NOT TO DO")
}

// ── 3. Sign-off ──────────────────────────────────────────────────────────────
// The owner's sign-off is their signature. It used to be smuggled in through
// special_instructions and then lost to two hardcoded rules further down:
// "DO NOT mention specific people by name or role" and "NEVER use em dashes".

func TestSignoffIsInstructedAndExemptedFromNameBan(t *testing.T) {
	cfg := base()
	cfg.Signoff = "— Dana, Owner"
	out := BuildReviewResponsePrompt(cfg)

	mustContain(t, out, "— Dana, Owner")
	// If the name ban is present at all, it must carry the sign-off exception.
	if strings.Contains(out, "DO NOT mention specific people by name or role") {
		mustContain(t, out, "except the sign-off")
	}
	// The em-dash ban must not silently outlaw an em-dash sign-off.
	if strings.Contains(out, "NEVER use em dashes") {
		mustContain(t, out, "except the sign-off")
	}
}

func TestNoSignoffLineWhenEmpty(t *testing.T) {
	cfg := base()
	cfg.Signoff = ""
	out := BuildReviewResponsePrompt(cfg)
	mustNotContain(t, out, "SIGN-OFF")
}

// ── 4. Emoji policy ──────────────────────────────────────────────────────────
// Tri-state on purpose: an unset policy must stay silent, because use_emojis is
// false on every business that has never opened the wizard and asserting a ban
// there would change replies nobody asked to change.

func TestEmojiPolicy(t *testing.T) {
	t.Run("unset says nothing", func(t *testing.T) {
		out := BuildReviewResponsePrompt(base())
		mustNotContain(t, out, "emoji", "Emoji")
	})
	t.Run("forbid is explicit", func(t *testing.T) {
		cfg := base()
		cfg.EmojiPolicy = EmojiForbid
		out := BuildReviewResponsePrompt(cfg)
		mustContain(t, out, "Do NOT use emojis")
	})
	t.Run("allow does not ban", func(t *testing.T) {
		cfg := base()
		cfg.EmojiPolicy = EmojiAllow
		out := BuildReviewResponsePrompt(cfg)
		mustNotContain(t, out, "Do NOT use emojis")
		mustContain(t, out, "emoji")
	})
}

// ── 5. Tone ──────────────────────────────────────────────────────────────────
// The wizard offers professional/friendly/casual/empathetic/humorous. Two of
// those five were not in the switch and rendered as a bare label
// ("- Use a empathetic tone ..."), with no rubric behind them.

func TestEveryWizardToneHasARubric(t *testing.T) {
	// Each wizard tone must describe HOW to sound, not just repeat its own name.
	rubric := map[string]string{
		"professional": "like a friendly business owner",
		"friendly":     "first-person",
		"casual":       "like chatting with a neighbor",
		"empathetic":   "acknowledge how the experience felt",
		"humorous":     "never at the customer's expense",
	}
	for tone, marker := range rubric {
		cfg := base()
		cfg.Tone = tone
		out := BuildReviewResponsePrompt(cfg)
		if strings.Contains(out, "- Use a "+tone+" tone to convey") {
			t.Errorf("tone %q renders as a bare label with no rubric", tone)
		}
		mustContain(t, out, marker)
		mustNotContain(t, out, "Use a  tone")
	}
}

func TestUnknownToneStillReadsAsEnglish(t *testing.T) {
	cfg := base()
	cfg.Tone = "swashbuckling"
	out := BuildReviewResponsePrompt(cfg)
	mustContain(t, out, "swashbuckling")
	mustNotContain(t, out, "%!")
}

// ── 6. Length ────────────────────────────────────────────────────────────────
// The sizes the owner is PROMISED in the wizard are the contract:
//
//	short = a quick phrase, 5-15 words | medium = 1-2 sentences
//	long  = 2-3 sentences, more detail | adaptive = match the review
//
// "adaptive" used to fall into the default branch and be rendered as
// "EXTREMELY brief ... 5-10 words max" — the opposite of matching the review.

func TestLengthInstructions(t *testing.T) {
	cases := map[string][]string{
		"short":    {"5-15 words"},
		"medium":   {"1-2 sentences"},
		"long":     {"2-3 sentences"},
		"adaptive": {"Match the length", "detailed"},
	}
	for length, needles := range cases {
		cfg := base()
		cfg.Length = length
		out := BuildReviewResponsePrompt(cfg)
		mustContain(t, out, needles...)
	}
}

func TestAdaptiveIsNotTreatedAsShort(t *testing.T) {
	cfg := base()
	cfg.Length = "adaptive"
	out := BuildReviewResponsePrompt(cfg)
	mustNotContain(t, out, "EXTREMELY brief")
}

// ── 7. Owner free text ───────────────────────────────────────────────────────

func TestSpecialInstructionsAppearVerbatim(t *testing.T) {
	cfg := base()
	cfg.SpecialInstructions = "Never mention the closure. We are family-run."
	out := BuildReviewResponsePrompt(cfg)
	mustContain(t, out, "Never mention the closure. We are family-run.")
}

// The owner's own instruction is the later, more specific word — it must sit
// AFTER the generic house rules, not be buried above them.
func TestSpecialInstructionsComeAfterHouseRules(t *testing.T) {
	cfg := base()
	cfg.SpecialInstructions = "ZZUNIQUEZZ"
	out := BuildReviewResponsePrompt(cfg)
	house := strings.Index(out, "NATURAL HUMAN LANGUAGE REQUIREMENTS")
	owner := strings.Index(out, "ZZUNIQUEZZ")
	if house < 0 || owner < 0 {
		t.Fatalf("missing blocks: house=%d owner=%d", house, owner)
	}
	if owner < house {
		t.Errorf("owner instructions (%d) must come after the house rules (%d)", owner, house)
	}
}

// ── 8. Business type ─────────────────────────────────────────────────────────
// Every reply for every industry used to open "You are a customer service AI
// assistant for a restaurant".

func TestBusinessTypeIsNotHardcodedToRestaurant(t *testing.T) {
	out := BuildReviewResponsePrompt(base())
	mustNotContain(t, out, "for a restaurant")

	cfg := base()
	cfg.BusinessType = "dental clinic"
	mustContain(t, BuildReviewResponsePrompt(cfg), "dental clinic")
}

// ── 9. The whole contract, end to end ────────────────────────────────────────
// One prompt carrying every preference at once: nothing dropped, nothing
// contradicted.

func TestFullPreferenceSetSurvivesTogether(t *testing.T) {
	cfg := ReviewResponseConfig{
		ReviewText:          "The barista remembered my order — made my week!",
		Author:              "Dana M.",
		Sentiment:           "positive",
		Tone:                "friendly",
		Length:              "long",
		ResponseStyles:      []string{"thank", "engage", "personalized_message"},
		SpecialInstructions: "Mention we open at 6:30am.",
		Signoff:             "— Dana, Owner",
		EmojiPolicy:         EmojiAllow,
		Rating:              5,
		ResponseCount:       8,
		BusinessType:        "coffee shop",
	}
	out := BuildReviewResponsePrompt(cfg)

	mustNotContain(t, out,
		"%!",
		"Keep responses GENERIC",
		"DO NOT call out specific actions or details from their review",
		"DO NOT use pushy return language",
		"Do NOT use emojis",
	)
	mustContain(t, out,
		"coffee shop",
		"2-3 sentences",
		"invite them back",
		"Mention we open at 6:30am.",
		"— Dana, Owner",
	)
}
