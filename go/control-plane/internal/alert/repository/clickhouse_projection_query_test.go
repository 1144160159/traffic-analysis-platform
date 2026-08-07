package repository

import (
	"strings"
	"testing"
)

func TestAlertSelectColumnsDoNotShadowMillisecondFilterColumns(t *testing.T) {
	t.Parallel()

	for name, columns := range map[string]string{
		"detail": alertSelectColumns,
		"list":   alertListSelectColumns,
	} {
		t.Run(name, func(t *testing.T) {
			for _, collidingAlias := range []string{
				"AS first_seen,",
				"AS last_seen,",
			} {
				if strings.Contains(columns, collidingAlias) {
					t.Fatalf("projection alias %q shadows the raw millisecond column used by WHERE", collidingAlias)
				}
			}
			for _, safeAlias := range []string{
				"AS first_seen_time,",
				"AS last_seen_time,",
			} {
				if !strings.Contains(columns, safeAlias) {
					t.Fatalf("projection is missing safe timestamp alias %q", safeAlias)
				}
			}
		})
	}
}
