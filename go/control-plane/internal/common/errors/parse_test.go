package errors

import (
	goerrors "errors"
	"testing"
)

func TestParseErrorCodeKnownCodes(t *testing.T) {
	cases := []struct {
		err  error
		code ErrorCode
		msg  string
	}{
		{goerrors.New("ANALYSIS_PLAN_NOT_APPROVED: plan binding mismatch"), ErrCodeAnalysisPlanNotApproved, "plan binding mismatch"},
		{goerrors.New("REVISION_CONFLICT: governance revision conflict"), ErrCodeAnalysisRevisionConflict, "governance revision conflict"},
		{goerrors.New("MISSING_IDEMPOTENCY_KEY: idempotency key is required"), ErrCodeAnalysisMissingIdempotencyKey, "idempotency key is required"},
	}
	for _, c := range cases {
		code, msg, ok := ParseErrorCode(c.err)
		if !ok || code != c.code || msg != c.msg {
			t.Fatalf("ParseErrorCode(%q) = %q,%q,%v want %q,%q,true", c.err, code, msg, ok, c.code, c.msg)
		}
	}
}

func TestParseErrorCodeBracketForm(t *testing.T) {
	err := New(ErrCodeAnalysisStaleFence, "old token")
	code, msg, ok := ParseErrorCode(err)
	if !ok || code != ErrCodeAnalysisStaleFence || msg != "old token" {
		t.Fatalf("bracket form parse = %q,%q,%v", code, msg, ok)
	}
}

func TestParseErrorCodeUnknownAndNil(t *testing.T) {
	if _, _, ok := ParseErrorCode(goerrors.New("pq: connection refused")); ok {
		t.Fatalf("unexpected parse for raw driver error")
	}
	if _, _, ok := ParseErrorCode(nil); ok {
		t.Fatalf("nil must not parse")
	}
}
