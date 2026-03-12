# Shared Prompts

Single source of truth for AI-powered review response generation across the Tenably AI ecosystem.

## Overview

Both `reviewprocessor` (interactive response generation) and `fetcher` (auto-reply worker) need to build LLM prompts for generating review responses. Previously, each service maintained its own copy of the prompt-building logic, which led to **prompt drift** -- the reviewprocessor prompt evolved with richer instructions (name personalization, rating-only handling, 5-star acknowledgment) while the fetcher's copy fell behind.

This matters because of the **training loop**: when a business owner uses the Review Simulator to critique AI responses, those learned preferences must apply identically whether the response is generated interactively or by the auto-reply worker. A single shared module eliminates drift by design.

## Package API

### `prompts.BuildReviewResponsePrompt(cfg ReviewResponseConfig) string`

Assembles a complete LLM prompt for review response generation. The prompt incorporates:

- **Tone control** -- professional, casual, fun, witty, humorous, or wild
- **Response styles** -- thank, engage, offer_reward, personalized_message, apologize, request_details, escalate
- **Reward integration** -- specific or randomized reward offers with redemption instructions
- **Name personalization** -- extracts first name and instructs the LLM to judge whether it's a real name
- **Rating-only handling** -- special instructions when a review has stars but no text
- **5-star acknowledgment** -- natural phrases to celebrate top ratings
- **Repetition avoidance** -- includes recent responses so the LLM avoids reusing patterns
- **Learned preferences** -- owner feedback from the training loop, injected into the prompt
- **Length control** -- short (5-10 words), medium (1 sentence), or long (2-3 sentences)
- **Configurable count** -- generates N unique responses (default 8 for the picker UI, 2 for auto-reply)

### `prompts.GetLearnedPreferencesBlock(db *sql.DB, userID, businessID int) string`

Queries the `review_response_feedback` table for up to 10 recent owner critiques and formats them into a prompt section. Each example includes the original review, the AI's response, and the owner's feedback. Returns an empty string if no feedback exists, degrading gracefully.

### `prompts.ReviewResponseConfig`

Configuration struct that serves as the contract between calling services and the prompt builder. All inputs (review text, author, sentiment, tone, styles, reward details, special instructions, etc.) are passed through this struct rather than function parameters, so new fields can be added without breaking callers.

## Usage

```go
import "github.com/bushnelljw-git/shared-prompts/prompts"
```

Build a prompt for interactive response generation:

```go
// Fetch learned preferences from the database
learnedPrefs := prompts.GetLearnedPreferencesBlock(db, userID, businessID)

// Configure and build the prompt
prompt := prompts.BuildReviewResponsePrompt(prompts.ReviewResponseConfig{
    ReviewText:         "Great food and amazing service!",
    Author:             "Emily Smith",
    Sentiment:          "positive",
    Tone:               "professional",
    ResponseStyles:     []string{"thank", "engage"},
    Length:             "medium",
    Rating:             5,
    ResponseCount:      8,
    LearnedPreferences: learnedPrefs,
})

// Send prompt to your LLM of choice
```

For auto-reply with fewer responses:

```go
prompt := prompts.BuildReviewResponsePrompt(prompts.ReviewResponseConfig{
    ReviewText:    "Terrible experience.",
    Sentiment:     "negative",
    Tone:          "professional",
    ResponseStyles: []string{"apologize", "request_details"},
    Length:        "medium",
    Rating:        1,
    ResponseCount: 2,
})
```

## How It Works

`BuildReviewResponsePrompt` constructs a structured prompt that instructs the LLM to generate a set of unique, varied review responses in strict JSON format. The prompt is built in layers:

1. **Core instruction** -- sets the LLM's role and the number of responses to generate
2. **Tone and style** -- maps tone keys to natural language descriptions and style keys to behavioral instructions
3. **Contextual blocks** -- conditionally appended sections for rewards, special instructions, learned preferences, name personalization, rating-only reviews, and 5-star acknowledgment
4. **Quality guardrails** -- detailed rules for natural language, radical variety between responses, and things to avoid (em dashes, corporate jargon, pushy return language)
5. **Output format** -- strict JSON schema with examples, dynamically sized to the requested response count

The prompt enforces Google Review conventions: responses are kept authentic, conversational, and brief.

## Project Structure

```
shared-prompts/
  go.mod                            # github.com/bushnelljw-git/shared-prompts (Go 1.23.8)
  prompts/
    review_response.go              # BuildReviewResponsePrompt and supporting helpers
    learned_preferences.go          # GetLearnedPreferencesBlock (DB query + formatting)
```

No external dependencies -- stdlib only (`fmt`, `strings`, `database/sql`).

## Development

To modify prompt behavior, edit the files in `prompts/` and tag a new version:

```bash
git tag v0.X.0
git push origin v0.X.0
```

Then update the consuming services:

```bash
# In reviewprocessor or fetcher
go get github.com/bushnelljw-git/shared-prompts@v0.X.0
```

Since all prompt logic lives here, changes propagate to both services on their next dependency update. There are no tests to run in this module currently -- prompt correctness is validated end-to-end through the Review Simulator in the dashboard.
