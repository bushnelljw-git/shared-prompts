package prompts

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

// deadDB is a handle whose every query fails, which is exactly the path
// GetLearnedPreferencesBlock already degrades over. It lets these tests build a
// real prompt without a database, and without a nil *sql.DB (which panics).
type deadConnector struct{}

func (deadConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("no database in unit tests")
}
func (deadConnector) Driver() driver.Driver { return nil }

func deadDB() *sql.DB { return sql.OpenDB(deadConnector{}) }

// The em-dash ban used to exist only on the review path, and only as item four
// of seven inside a section about JSON validity. Live drafts came back dashed
// ("Thanks for the 5 stars—delicious food..."), and comment replies were never
// told the rule at all. These tests hold the line on both: every builder that
// writes something a customer will read must state the rule, and state it as a
// rule.

func mustStateTheRule(t *testing.T, label, out string) {
	t.Helper()
	if !strings.Contains(out, "CRITICAL RULE - PUNCTUATION") {
		t.Errorf("%s: the punctuation rule is not stated as a critical rule", label)
	}
	if !strings.Contains(out, "NEVER use em dashes (—)") {
		t.Errorf("%s: the em-dash ban is missing", label)
	}
	// A rule the model must obey every time gets a re-read pass, not one mention.
	if !strings.Contains(out, "re-read") {
		t.Errorf("%s: the rule is stated once and never re-checked", label)
	}
}

func TestReviewRepliesStateTheEmDashRule(t *testing.T) {
	mustStateTheRule(t, "review response", BuildReviewResponsePrompt(base()))
}

// The sign-off is usually "— Dana, Owner". Banning its dash would mean quietly
// editing the owner's own signature, so the ban must carry the exception, and
// the exception must never leak into a business that set no sign-off.
func TestEmDashRuleExemptsTheSignoffOnlyWhenThereIsOne(t *testing.T) {
	with := base()
	with.Signoff = "— Dana, Owner"
	out := BuildReviewResponsePrompt(with)
	mustStateTheRule(t, "review response with sign-off", out)
	if !strings.Contains(out, "except the sign-off") {
		t.Error("with a sign-off set, the em-dash ban does not carry its exception")
	}

	out = BuildReviewResponsePrompt(base())
	if strings.Contains(out, "except the sign-off") {
		t.Error("with no sign-off set, the ban offers an exception for a line that does not exist")
	}
}

// Comment auto-replies carry the business's name in front of a customer exactly
// like a review reply does, so they answer to the same rule.
func TestCommentAutoRepliesStateTheEmDashRule(t *testing.T) {
	db := deadDB()
	defer db.Close()

	userDefined, err := BuildUserDefinedReplyPrompt(db, UserDefinedReplyInput{
		UserID:      1,
		BusinessID:  1,
		CommentText: "what time do you close?",
	})
	if err != nil {
		t.Fatalf("user-defined auto reply prompt: %v", err)
	}
	mustStateTheRule(t, "user-defined auto reply", userDefined)

	sentiment, err := BuildSentimentReplyPrompt(db, SentimentReplyInput{
		UserID:          1,
		BusinessID:      1,
		CommentText:     "loved it",
		IsPositiveReply: true,
	})
	if err != nil {
		t.Fatalf("sentiment auto reply prompt: %v", err)
	}
	mustStateTheRule(t, "sentiment auto reply", sentiment)
}
