package com.traffic.flink.rule.broadcast;

import com.traffic.flink.rule.model.Rule;

import java.util.Objects;

/** Pure version transition policy for the rule broadcast state. */
final class RuleUpdateStateMachine {

    enum Status {
        APPLIED,
        DUPLICATE,
        STALE,
        CONFLICT
    }

    static final class Decision {
        private final Status status;
        private final Rule state;
        private final String reason;

        Decision(Status status, Rule state, String reason) {
            this.status = status;
            this.state = state;
            this.reason = reason;
        }

        Status getStatus() { return status; }
        Rule getState() { return state; }
        String getReason() { return reason; }
        boolean changesState() { return status == Status.APPLIED; }
    }

    private RuleUpdateStateMachine() {
    }

    static Decision decide(Rule existing, Rule incoming) {
        return decideForRuntime(existing, incoming, true);
    }

    static Decision decideForRuntime(
            Rule existing, Rule incoming, boolean matcherAvailable) {
        if (incoming == null) {
            throw new IllegalArgumentException("incoming rule command is required");
        }
        normalizeTombstone(incoming);
        if (incoming.isEnabled() && requiresMatcher(incoming) && !matcherAvailable) {
            return new Decision(Status.CONFLICT, existing,
                    "rule type has no executable matcher in flink-rule-job");
        }
        if (existing == null) {
            return new Decision(Status.APPLIED, incoming, "first observed version");
        }
        if (!Objects.equals(existing.getTenantId(), incoming.getTenantId())
                || !Objects.equals(existing.getRuleId(), incoming.getRuleId())) {
            return new Decision(Status.CONFLICT, existing, "rule identity changed within one state key");
        }
        if (incoming.getVersion() < existing.getVersion()) {
            return new Decision(Status.STALE, existing, "incoming version is older than state");
        }
        if (incoming.getVersion() > existing.getVersion()) {
            return new Decision(Status.APPLIED, incoming, "incoming version advances state");
        }

        if (sameCommand(existing, incoming) || sameBusinessPayload(existing, incoming)) {
            return new Decision(Status.DUPLICATE, existing, "same version and payload replayed");
        }
        return new Decision(Status.CONFLICT, existing,
                "same rule version carries a different action or payload");
    }

    private static boolean requiresMatcher(Rule incoming) {
        switch (incoming.getAction()) {
            case DELETE:
            case DISABLE:
                return false;
            default:
                return true;
        }
    }

    private static void normalizeTombstone(Rule rule) {
        switch (rule.getAction()) {
            case DELETE:
            case DISABLE:
                rule.setEnabled(false);
                break;
            default:
                break;
        }
    }

    private static boolean sameCommand(Rule left, Rule right) {
        String leftEvent = left.getCommandEventId();
        String rightEvent = right.getCommandEventId();
        if (leftEvent != null && !leftEvent.isBlank() && leftEvent.equals(rightEvent)) {
            return true;
        }
        String leftChecksum = left.getCommandChecksum();
        String rightChecksum = right.getCommandChecksum();
        return leftChecksum != null && !leftChecksum.isBlank()
                && leftChecksum.equals(rightChecksum)
                && Objects.equals(left.getActionStr(), right.getActionStr());
    }

    private static boolean sameBusinessPayload(Rule left, Rule right) {
        return Objects.equals(left.getActionStr(), right.getActionStr())
                && Objects.equals(left.getName(), right.getName())
                && Objects.equals(left.getRuleTypeStr(), right.getRuleTypeStr())
                && Objects.equals(left.getEngine(), right.getEngine())
                && Objects.equals(left.getDescription(), right.getDescription())
                && Objects.equals(left.getConditions(), right.getConditions())
                && Objects.equals(left.getLabels(), right.getLabels())
                && Objects.equals(left.getSeverityStr(), right.getSeverityStr())
                && left.isEnabled() == right.isEnabled()
                && left.getPriority() == right.getPriority()
                && Objects.equals(left.getStatus(), right.getStatus());
    }
}
