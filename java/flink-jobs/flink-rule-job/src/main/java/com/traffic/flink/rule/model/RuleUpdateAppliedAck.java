package com.traffic.flink.rule.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.Serializable;
import java.time.Instant;

/** Per-subtask receipt for a canonical rule broadcast-state transition. */
public final class RuleUpdateAppliedAck implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @JsonProperty("schema_version") public int schemaVersion = 1;
    @JsonProperty("event_id") public String eventId;
    @JsonProperty("tenant_id") public String tenantId;
    @JsonProperty("rule_id") public String ruleId;
    @JsonProperty("version") public long version;
    @JsonProperty("current_version") public long currentVersion;
    @JsonProperty("action") public String action;
    @JsonProperty("checksum") public String checksum;
    @JsonProperty("subtask_index") public int subtaskIndex;
    @JsonProperty("parallelism") public int parallelism;
    @JsonProperty("status") public String status;
    @JsonProperty("error") public String error = "";
    @JsonProperty("timestamp") public String timestamp = Instant.now().toString();

    public byte[] toJson() {
        try {
            return MAPPER.writeValueAsBytes(this);
        } catch (Exception error) {
            throw new IllegalStateException("cannot serialize rule update acknowledgement", error);
        }
    }

    public static RuleUpdateAppliedAck from(
            Rule incoming,
            long currentVersion,
            String status,
            String error,
            int subtaskIndex,
            int parallelism) {
        RuleUpdateAppliedAck ack = new RuleUpdateAppliedAck();
        ack.eventId = incoming.getCommandEventId();
        ack.tenantId = incoming.getTenantId();
        ack.ruleId = incoming.getRuleId();
        ack.version = incoming.getVersion();
        ack.currentVersion = currentVersion;
        ack.action = incoming.getAction().getValue();
        ack.checksum = incoming.getCommandChecksum();
        ack.subtaskIndex = subtaskIndex;
        ack.parallelism = parallelism;
        ack.status = status;
        ack.error = error == null ? "" : error;
        return ack;
    }
}
