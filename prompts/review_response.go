package prompts

import (
	"fmt"
	"strings"
)

// ReviewResponseConfig holds all the inputs needed to build a review response prompt.
// Both reviewprocessor and fetcher populate this struct from their own data sources
// (HTTP request fields, database rows, etc.) and pass it to BuildReviewResponsePrompt.
//
// This struct is the contract between the services and the shared prompt logic.
// Add new fields here when the prompt needs new inputs — don't add parameters to
// the function signature, which would break callers in the other service.
type ReviewResponseConfig struct {
	// ReviewText is the customer's review content. May be empty for rating-only reviews.
	ReviewText string

	// Author is the reviewer's display name (e.g., "Emily Smith"). Used for name
	// personalization — the LLM decides whether the name is real enough to use.
	Author string

	// Sentiment is "positive" or "negative", derived from star rating or LLM classification.
	Sentiment string

	// Tone controls the voice: "professional", "casual", "fun", "witty", "humorous", "wild".
	Tone string

	// ResponseStyles lists the enabled style keys for this sentiment.
	// Positive: "thank", "engage", "offer_reward", "personalized_message"
	// Negative: "apologize", "request_details", "escalate"
	ResponseStyles []string

	// Reward is the reward text (e.g., "10% off your next visit"). Empty if no reward.
	Reward string

	// RedemptionInstructions tells the customer how to claim the reward.
	RedemptionInstructions string

	// RandomizeReward means offer a generic reward instead of a specific one.
	RandomizeReward bool

	// SpecialInstructions are free-text rules from the business owner
	// (e.g., "Always mention our email: support@example.com").
	SpecialInstructions string

	// LearnedPreferences is the output of GetLearnedPreferencesBlock — a formatted
	// block of owner feedback examples. Empty string if no feedback exists yet.
	LearnedPreferences string

	// PreviousResponses are the business's recently used responses, so the LLM can
	// avoid repetition. Empty slice if none.
	PreviousResponses []string

	// Length controls response verbosity: "short" (5-10 words), "medium" (1 sentence),
	// "long" (2-3 sentences).
	Length string

	// Rating is the star rating (1-5). Used for rating-only detection and 5-star acknowledgment.
	Rating int

	// ResponseCount is how many unique responses to generate. reviewprocessor uses 8
	// (for the response picker UI), fetcher uses 2 (for auto-reply selection).
	// Defaults to 8 if zero.
	ResponseCount int
}

// BuildReviewResponsePrompt constructs the full LLM prompt for generating review responses.
//
// THIS IS THE SINGLE SOURCE OF TRUTH for review response prompting across all services.
// The prompt includes tone, styles, rewards, special instructions, learned preferences,
// name personalization, rating-only handling, 5-star acknowledgment, repetition avoidance,
// and strict JSON output formatting.
//
// Both reviewprocessor (interactive response generation) and fetcher (auto-reply worker)
// call this function. Any changes to prompt logic should be made HERE so that the
// Review Simulator, "Pick a Response" modal, and auto-reply workflows all behave
// consistently — which is the whole point of the learned preferences training loop.
func BuildReviewResponsePrompt(cfg ReviewResponseConfig) string {
	tone := cfg.Tone
	if tone == "" {
		tone = "professional"
	}

	responseCount := cfg.ResponseCount
	if responseCount <= 0 {
		responseCount = 8
	}

	// Build style instructions from the enabled style keys
	var styleInstructions []string
	for _, style := range cfg.ResponseStyles {
		switch style {
		case "thank":
			styleInstructions = append(styleInstructions, "Express gratitude to the reviewer.")
		case "engage":
			styleInstructions = append(styleInstructions, "Show appreciation and warmth without being pushy.")
		case "offer_reward":
			styleInstructions = append(styleInstructions, "Offer an incentive or reward if applicable, ensuring it feels natural.")
		case "personalized_message":
			styleInstructions = append(styleInstructions, "Include a personalized message tailored to the tone that acknowledges their specific feedback.")
		case "apologize":
			styleInstructions = append(styleInstructions, "Apologize sincerely and offer a solution (e.g., 'We're sorry, let us make this right!').")
		case "request_details":
			styleInstructions = append(styleInstructions, "Request more details to understand the issue (e.g., 'Can you share more so we can improve?').")
		case "escalate":
			styleInstructions = append(styleInstructions, "Mention that the issue will be escalated to a human team member (e.g., 'Our team will reach out soon.').")
		}
	}
	stylesBlock := ""
	if len(styleInstructions) > 0 {
		stylesBlock = fmt.Sprintf("\n- %s", strings.Join(styleInstructions, "\n- "))
	}

	// Reward block — include exact reward text or a generic placeholder
	rewardBlock := ""
	if cfg.Reward != "" && cfg.RedemptionInstructions != "" {
		rewardBlock = fmt.Sprintf(
			"A reward is offered. Incorporate it naturally and sincerely.\nReward: %q\nRedemption instructions: %q\nYou MUST include the reward and redemption instructions in the response text.",
			cfg.Reward, cfg.RedemptionInstructions)
	} else if contains(cfg.ResponseStyles, "offer_reward") && cfg.RandomizeReward {
		rewardBlock = "A reward is offered. Incorporate a generic reward (e.g., 'a special discount') naturally and sincerely.\nYou MUST include the generic reward in the response text and mention it will be provided upon next visit or contact."
	}

	// Special instructions from the business owner + learned preferences from the training loop
	specialInstructionsBlock := ""
	if cfg.SpecialInstructions != "" {
		specialInstructionsBlock = fmt.Sprintf(
			"Follow these special instructions:\n%q",
			cfg.SpecialInstructions)
	}
	if cfg.LearnedPreferences != "" {
		specialInstructionsBlock += cfg.LearnedPreferences
	}

	// Name personalization — extract first name and let LLM decide if appropriate
	firstName := extractFirstName(cfg.Author)
	nameInstruction := ""
	if firstName != "" {
		nameInstruction = fmt.Sprintf("\n\nPERSONALIZATION - USE YOUR JUDGMENT:\n"+
			"The reviewer's name appears to be %q.\n"+
			"IMPORTANT: You must decide if this is a real, appropriate first name to use in a response.\n\n"+
			"USE THE NAME if it's:\n"+
			"- A common real first name (like John, Sarah, Michael, Emily, David, Maria, etc.)\n"+
			"- A legitimate name from any culture that sounds authentic\n\n"+
			"DO NOT USE THE NAME if it's:\n"+
			"- A cartoon/fictional character (Spongebob, Squilliam, Batman, etc.)\n"+
			"- An obvious alias or username (User123, Guest, Anonymous, etc.)\n"+
			"- A fake-sounding or made-up name\n"+
			"- A business name, title, or generic term\n"+
			"- Something that just doesn't sound like a real person's name\n\n"+
			"If you're unsure or it seems fake, DO NOT use the name - just respond without it.\n"+
			"If you do use it, make it natural and friendly (e.g., 'Thanks %s!' or 'Hi %s, ...').",
			firstName, firstName, firstName)
	}

	// Rating-only reviews (no text, just stars) need special handling
	isRatingOnly := strings.TrimSpace(cfg.ReviewText) == ""
	ratingOnlyInstruction := ""
	if isRatingOnly {
		ratingOnlyInstruction = fmt.Sprintf("\n\nIMPORTANT - RATING-ONLY REVIEW:\nThis reviewer only left a %d-star rating with NO written feedback or comments.\n"+
			"- DO NOT reference specific feedback, comments, or details they mentioned (because they didn't provide any)\n"+
			"- DO NOT say things like 'thanks for your feedback' or 'we appreciate your comments' (they didn't leave any)\n"+
			"- Instead, acknowledge their %d-star rating directly (e.g., 'Thanks for the %d stars!' or 'We appreciate your %d-star rating!')\n"+
			"- Keep it simple and genuine - just thank them for taking the time to rate\n"+
			"- For positive ratings (4-5 stars): express gratitude for their support\n"+
			"- For negative ratings (1-3 stars): acknowledge their rating and invite them to share more details if they'd like\n",
			cfg.Rating, cfg.Rating, cfg.Rating, cfg.Rating)
	}

	// 5-star reviews get special acknowledgment
	fiveStarInstruction := ""
	if cfg.Rating == 5 {
		fiveStarInstruction = "\n\nIMPORTANT - 5-STAR REVIEW ACKNOWLEDGMENT:\n" +
			"This is a 5-star review! Make sure to acknowledge the 5-star rating in your responses.\n" +
			"Use natural, varied phrases like:\n" +
			"- 'Thanks for the 5 stars!'\n" +
			"- 'Appreciate the 5-star shoutout!'\n" +
			"- '5 stars! Thank you so much!'\n" +
			"- 'Thanks for the 5-star love!'\n" +
			"- 'So grateful for the 5 stars!'\n" +
			"- 'Love the 5-star review!'\n" +
			"- 'Thanks for taking the time to leave 5 stars!'\n" +
			"- '5 stars means the world to us!'\n" +
			"IMPORTANT: Not every response needs to mention '5 stars' explicitly, but at least 4-5 of the 8 responses should acknowledge it naturally.\n" +
			"Keep it casual and authentic - don't force it if it doesn't flow naturally in the response."
	}

	// Tone descriptions — how the LLM should "sound"
	toneDescription := ""
	switch tone {
	case "professional":
		toneDescription = "warm and genuine, like a friendly business owner talking to a valued customer"
	case "casual":
		toneDescription = "casual and friendly, like chatting with a neighbor"
	case "fun":
		toneDescription = "fun and upbeat with playful energy"
	case "witty":
		toneDescription = "witty and clever with subtle humor or wordplay (avoid being sarcastic)"
	case "humorous":
		toneDescription = "humorous and funny with light jokes, puns, or playful humor that makes people smile (keep it appropriate and friendly)"
	case "wild":
		toneDescription = "wild and enthusiastic with bold, energetic language and lots of excitement"
	default:
		toneDescription = tone
	}

	// Build the JSON output format dynamically based on response count
	jsonFields := ""
	for i := 1; i <= responseCount; i++ {
		jsonFields += fmt.Sprintf("  \"response%d\": \"<your response>\",\n", i)
	}
	// Remove trailing comma+newline and add just newline
	if len(jsonFields) > 2 {
		jsonFields = jsonFields[:len(jsonFields)-2] + "\n"
	}

	countWord := "EIGHT"
	switch responseCount {
	case 1:
		countWord = "ONE"
	case 2:
		countWord = "TWO"
	case 3:
		countWord = "THREE"
	case 4:
		countWord = "FOUR"
	}

	// Build example JSON output
	exampleResponses := []string{
		"Thank you so much! Really appreciate you taking the time to share this.",
		"Thanks for the kind words! So glad you enjoyed your visit.",
		"Appreciate you!",
		"This made our day. Thanks for coming in!",
		"So happy you had a great experience with us.",
		"Love hearing this!",
		"Thanks for stopping by and sharing your thoughts.",
		"Grateful for your support!",
	}
	exampleJSON := "{\n"
	for i := 0; i < responseCount && i < len(exampleResponses); i++ {
		comma := ","
		if i == responseCount-1 || i == len(exampleResponses)-1 {
			comma = ""
		}
		exampleJSON += fmt.Sprintf("  \"response%d\": \"%s\"%s\n", i+1, exampleResponses[i], comma)
	}
	exampleJSON += "}"

	return fmt.Sprintf(
		"You are a customer service AI assistant for a restaurant, tasked with crafting responses to %s customer reviews.\n\n"+
			"Customer Review:\n%q\n\n"+
			"Generate exactly %s unique, polite, and helpful replies based on the review. Each response must be RADICALLY DIFFERENT from the others. Each response must:\n"+
			buildLengthInstruction(cfg.Length)+

			"- Use a %s tone to convey genuine human empathy and sound natural.\n"+
			"- Use COMPLETELY DISTINCT word choices, approaches, and structures (e.g., warm and inviting, brief and direct, enthusiastic, understated, playful, sincere, upbeat, thoughtful).%s\n"+
			"%s\n"+
			"%s\n"+
			"%s%s%s\n\n"+
			"GOOGLE REVIEW RESPONSE REQUIREMENTS:\n"+
			"- These responses are for GOOGLE REVIEWS - keep them authentic and conversational\n"+
			"- Write like you're genuinely responding to a real customer, not writing corporate marketing copy\n"+
			"- Keep responses SHORT and NATURAL - Google review responses should be brief and genuine\n"+
			"- Avoid overly formal or sales-focused language that doesn't sound like a real person\n\n"+
			"NATURAL HUMAN LANGUAGE REQUIREMENTS:\n"+
			"- Write like a REAL PERSON texting or talking, NOT like a corporate PR statement\n"+
			"- AVOID formal/stuffy language like 'genuinely delighted', 'memorable feast of flavors', 'truly appreciate your praise'\n"+
			"- AVOID sales-pitchy phrases like 'explore even more', 'creative delights', 'attentive team'\n"+
			"- DO NOT use pushy return language like 'come back', 'hurry back', 'visit again', 'see you again', 'welcome you back'\n"+
			"- Just thank them or acknowledge their feedback - don't pressure them to return\n"+
			"- Use simple, everyday words that normal people actually say in conversation\n"+
			"- Keep it SHORT and NATURAL - don't try to pack everything into one long sentence\n"+
			nameInstruction+"\n\n"+
			"KEEP IT SIMPLE AND GENERIC:\n"+
			"- DO NOT mention specific people by name or role (manager, owner, staff names, etc.)\n"+
			"- DO NOT reference specific menu items or dishes they mentioned\n"+
			"- DO NOT call out specific actions or details from their review\n"+
			"- Keep responses GENERIC and focused on overall gratitude or apology\n"+
			"- LESS IS MORE - the shorter and simpler, the better\n"+
			"- For POSITIVE reviews: Just thank them warmly and express happiness\n"+
			"- For NEGATIVE reviews: Apologize briefly and offer to help\n"+
			"- Think: What would you text if you only had 10 seconds?\n\n"+
			"EXAMPLES - WHAT NOT TO DO:\n"+
			"❌ BAD: 'Awesome feedback, thanks for highlighting the manager's help and Jonathan's vibe.'\n"+
			"❌ BAD: 'Thanks so much for sharing that, it means a lot to hear how the manager guided you through the menu and Jonathan made everything feel just right.'\n"+
			"❌ BAD: 'Appreciate you calling out the manager's menu rundown and those spot-on mac bites we comped you'\n"+
			"❌ BAD: 'Glad the mac bites were a hit!'\n\n"+
			"EXAMPLES - WHAT TO DO:\n"+
			"✅ GOOD: 'Thanks so much! Really glad you enjoyed everything.'\n"+
			"✅ GOOD: 'So happy you had a great time with us!'\n"+
			"✅ GOOD: 'Love hearing this! Thanks for coming in.'\n"+
			"✅ GOOD: 'Thanks for the kind words! Really appreciate it.'\n"+
			"✅ GOOD: 'Appreciate you! So glad you enjoyed your visit.'\n\n"+
			"RADICAL VARIETY REQUIREMENTS - CRITICAL:\n"+
			"- Each of the %d responses MUST be COMPLETELY DIFFERENT in structure, word choice, and approach\n"+
			"- NEVER start two responses with the same word or phrase\n"+
			"- NEVER end two responses with the same sentiment or structure\n"+
			"- Use different sentence lengths: some very short (3-5 words), some medium, some longer\n"+
			"- Vary emotional intensity: some enthusiastic, some calm, some playful, some sincere, some warm, some brief\n"+
			"- AVOID repetitive phrases like 'over the moon', 'epic bites', 'cheesy magic', 'blew you away', etc.\n"+
			"- Use FRESH, VARIED language for each response - never repeat the same opening or closing phrases\n"+
			"- Be creative and authentic - imagine you're %d different people each writing their own unique response\n"+
			"- If you've used a phrase in one response, find a completely different way to express it in others\n"+
			buildPreviousResponsesBlock(cfg.PreviousResponses)+"\n"+
			"Output the responses in this EXACT JSON format, with no additional text, markdown, backticks, or comments:\n"+
			"{\n"+
			jsonFields+
			"}\n\n"+
			"CRITICAL INSTRUCTIONS:\n"+
			"- Output ONLY the JSON object. Do NOT include any extra text, explanations, markdown, backticks, or comments.\n"+
			"- Ensure the JSON is valid: use double quotes for strings, escape any quotes within the responses, and avoid trailing commas.\n"+
			"- All %d responses must be non-empty strings adhering to the specified tone, style, and instructions.\n"+
			"- NEVER use em dashes (—) or semicolons (;) in responses. Write naturally like a real person would.\n"+
			"- Use simple punctuation: periods, commas, exclamation points, and question marks only.\n"+
			"- Keep the writing conversational and authentic, as if a friendly human is responding.\n"+
			"- The output will be parsed by a strict JSON validator, and any deviation from the format will cause an error.\n\n"+
			"Example of correct output:\n"+
			exampleJSON,
		cfg.Sentiment, cfg.ReviewText, countWord, toneDescription, stylesBlock, rewardBlock, specialInstructionsBlock, ratingOnlyInstruction, fiveStarInstruction,
		responseCount, responseCount, responseCount)
}

// buildLengthInstruction returns the length constraint for each response.
func buildLengthInstruction(length string) string {
	switch length {
	case "long":
		return "- Be engaging and thoughtful, limited to 2-3 short sentences.\n"
	case "medium":
		return "- Be concise and engaging, limited to 1 sentence only.\n"
	default:
		// "short" or unspecified — super brief, a quick phrase
		return "- Be EXTREMELY brief. Use a SHORT phrase or fragment, roughly 5-10 words max. Think quick text-message style.\n"
	}
}

// buildPreviousResponsesBlock creates the "avoid repetition" section from recent responses.
func buildPreviousResponsesBlock(previousResponses []string) string {
	if len(previousResponses) == 0 {
		return ""
	}

	showCount := 10
	if len(previousResponses) < showCount {
		showCount = len(previousResponses)
	}

	block := "\n\nPREVIOUS RESPONSES TO AVOID REPETITION:\n"
	block += "Here are your last " + fmt.Sprintf("%d", showCount) + " responses. DO NOT reuse the same words, phrases, or sentence structures:\n"
	for i := 0; i < showCount; i++ {
		block += fmt.Sprintf("%d. %q\n", i+1, previousResponses[i])
	}
	block += "\nIMPORTANT: Analyze the language patterns above and create responses that are completely different in wording and structure."

	return block
}

// extractFirstName pulls the first word from an author name for personalization.
// Returns empty string if the name is too short or contains non-letter characters.
func extractFirstName(author string) string {
	parts := strings.Fields(strings.TrimSpace(author))
	if len(parts) == 0 {
		return ""
	}
	firstName := parts[0]

	if len(firstName) < 2 {
		return ""
	}

	for _, char := range firstName {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')) {
			return ""
		}
	}

	return firstName
}

// contains checks if a string slice contains a given value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
