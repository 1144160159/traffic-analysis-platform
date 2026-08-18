package contract

import (
	"errors"
	"testing"
)

func TestParseErrorKnownCodes(t *testing.T) {
	cases := []struct {
		err  error
		code string
		msg  string
	}{
		{errors.New("ANALYSIS_PLAN_NOT_APPROVED: plan binding mismatch"), string(ErrCodePlanNotApproved), "plan binding mismatch"},
		{errors.New("REVISION_CONFLICT: governance revision conflict"), string(ErrCodeRevisionConflict), "governance revision conflict"},
		{errors.New("MISSING_IDEMPOTENCY_KEY: idempotency key is required"), string(ErrCodeMissingIdempotencyKey), "idempotency key is required"},
	}
	for _, c := range cases {
		code, msg, ok := ParseError(c.err)
		if !ok || code != c.code || msg != c.msg {
			t.Fatalf("ParseError(%q) = %q,%q,%v want %q,%q,true", c.err, code, msg, ok, c.code, c.msg)
		}
	}
}

func TestParseErrorUnknownAndNil(t *testing.T) {
	if _, _, ok := ParseError(errors.New("pq: connection refused")); ok {
		t.Fatalf("unexpected parse for raw driver error")
	}
	if _, _, ok := ParseError(errors.New("ANALYSIS_PLAN_NOT_APPROVED_WITHOUT_SEPARATOR")); ok {
		t.Fatalf("prefix without separator must not parse")
	}
	if _, _, ok := ParseError(nil); ok {
		t.Fatalf("nil must not parse")
	}
}
