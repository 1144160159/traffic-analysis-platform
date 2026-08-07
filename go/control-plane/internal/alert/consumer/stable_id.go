package consumer

import (
	"strings"

	"github.com/google/uuid"
)

var alertIdentityNamespace = uuid.MustParse("7cc7776f-3c85-5a4f-99ab-d9ce9aece198")

// stableAlertID makes Kafka replay idempotent even when an upstream producer
// omits alert_id. Source event identity is preferred; the deterministic
// fingerprint is the compatibility fallback.
func stableAlertID(tenantID, eventID, fingerprint string) string {
	identity := strings.TrimSpace(eventID)
	if identity == "" {
		identity = "fingerprint:" + strings.TrimSpace(fingerprint)
	} else {
		identity = "event:" + identity
	}
	return uuid.NewSHA1(alertIdentityNamespace, []byte(strings.TrimSpace(tenantID)+"|"+identity)).String()
}
