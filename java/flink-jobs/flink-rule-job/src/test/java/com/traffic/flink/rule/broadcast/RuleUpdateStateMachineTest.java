package com.traffic.flink.rule.broadcast;

import com.traffic.flink.rule.model.Rule;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class RuleUpdateStateMachineTest {

    @Test
    void newerDisablePersistsTombstoneAndBlocksOlderReplay() {
        Rule active = rule(3, "update", true, "event-3", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        Rule disabled = rule(4, "disable", true, "event-4", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");

        RuleUpdateStateMachine.Decision disableDecision =
                RuleUpdateStateMachine.decide(active, disabled);
        RuleUpdateStateMachine.Decision staleReplay =
                RuleUpdateStateMachine.decide(disableDecision.getState(), active);

        assertThat(disableDecision.getStatus())
                .isEqualTo(RuleUpdateStateMachine.Status.APPLIED);
        assertThat(disableDecision.getState().isEnabled()).isFalse();
        assertThat(staleReplay.getStatus())
                .isEqualTo(RuleUpdateStateMachine.Status.STALE);
        assertThat(staleReplay.getState().isEnabled()).isFalse();
        assertThat(staleReplay.getState().getVersion()).isEqualTo(4);
    }

    @Test
    void exactReplayIsDuplicateButSameVersionDifferentPayloadConflicts() {
        Rule existing = rule(7, "update", true, "event-7", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        Rule replay = rule(7, "update", true, "event-7", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        Rule conflict = rule(7, "update", true, "event-other", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
        conflict.setPriority(99);

        assertThat(RuleUpdateStateMachine.decide(existing, replay).getStatus())
                .isEqualTo(RuleUpdateStateMachine.Status.DUPLICATE);
        assertThat(RuleUpdateStateMachine.decide(existing, conflict).getStatus())
                .isEqualTo(RuleUpdateStateMachine.Status.CONFLICT);
    }

    @Test
    void rollbackMustUseANewerVersion() {
        Rule current = rule(9, "update", true, "event-9", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        Rule staleRollback = rule(8, "update", true, "rollback-8", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
        Rule versionedRollback = rule(10, "update", true, "rollback-10", "cccccccccccccccccccccccccccccccc");
        versionedRollback.setName("restored-known-good-content");

        assertThat(RuleUpdateStateMachine.decide(current, staleRollback).getStatus())
                .isEqualTo(RuleUpdateStateMachine.Status.STALE);
        assertThat(RuleUpdateStateMachine.decide(current, versionedRollback).getStatus())
                .isEqualTo(RuleUpdateStateMachine.Status.APPLIED);
    }

    @Test
    void enabledRuleWithoutExecutableMatcherFailsTruthfully() {
        Rule unsupported = rule(
                1, "create", true, "event-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        unsupported.setRuleTypeStr("signature");

        RuleUpdateStateMachine.Decision decision =
                RuleUpdateStateMachine.decideForRuntime(null, unsupported, false);

        assertThat(decision.getStatus()).isEqualTo(RuleUpdateStateMachine.Status.CONFLICT);
        assertThat(decision.getState()).isNull();
        assertThat(decision.getReason()).contains("no executable matcher");
    }

    private static Rule rule(
            long version, String action, boolean enabled, String eventId, String checksum) {
        Rule rule = new Rule();
        rule.setRuleId("rule-1");
        rule.setTenantId("tenant-1");
        rule.setName("known rule");
        rule.setRuleTypeStr("port_scan");
        rule.setActionStr(action);
        rule.setEnabled(enabled);
        rule.setVersion(version);
        rule.setPriority(50);
        rule.setCommandEventId(eventId);
        rule.setCommandChecksum(checksum);
        return rule;
    }
}
