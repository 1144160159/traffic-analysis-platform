package com.traffic.flink.rule.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;

class RuleJsonParseFunctionTest {

    @Test
    void acceptsVersionedRuleUpdate() {
        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(source(
                canonicalCommand("update", 3, canonicalRuleChecksum())));

        assertNotNull(result.rule);
        assertNull(result.failure);
        assertEquals(3L, result.rule.getVersion());
        assertEquals("event-1", result.rule.getCommandEventId());
        assertEquals(canonicalRuleChecksum(), result.rule.getCommandChecksum());
        assertEquals("operator-1", result.rule.getUpdatedBy());
        assertEquals(true, result.rule.isCanonicalCommandEnvelope());
    }

    @Test
    void malformedJsonProducesCanonicalDlqWithJsonContentType() throws Exception {
        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(source("{"));

        assertNull(result.rule);
        assertEquals("BAD_SCHEMA", result.failure.errorCode());
        JsonNode json = new ObjectMapper().readTree(result.failure.toJson());
        assertEquals("rule.updates", json.get("original_topic").asText());
        assertEquals("application/json", json.get("content_type").asText());
        assertEquals("traffic.rule.update", json.get("proto_message_type").asText());
        assertEquals("flink-rule-job", json.get("service_name").asText());
    }

    @Test
    void unsupportedActionIsRejectedWithoutReplacingBroadcastState() {
        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(source(
                "{\"rule_id\":\"rule-1\",\"tenant_id\":\"tenant-1\","
                        + "\"version\":3,\"action\":\"overwrite\"}"));

        assertNull(result.rule);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    @Test
    void sourceHeaderMismatchIsRejectedBeforeBroadcastStateMutation() {
        RawKafkaRecord mismatched = new RawKafkaRecord(
                "rule.updates", 0, 11L, 2_500L,
                "rule-1".getBytes(StandardCharsets.UTF_8),
                canonicalCommand("update", 3, canonicalRuleChecksum())
                        .getBytes(StandardCharsets.UTF_8),
                Map.of(
                        "event_id", "different-event",
                        "action", "update",
                        "tenant_id", "tenant-1",
                        "rule_id", "rule-1",
                        "rule_version", "v3",
                        "schema_version", "1.1",
                        "checksum_algorithm", RuleCommandChecksum.ALGORITHM,
                        "checksum", canonicalRuleChecksum()));

        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(mismatched);

        assertNull(result.rule);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    @Test
    void nestedRuleTamperingIsRejectedBeforeBroadcastStateMutation() {
        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(source(
                canonicalCommand("update", 3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")));

        assertNull(result.rule);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    @Test
    void wireChecksumMatchesGoCanonicalFixture() throws Exception {
        JsonNode wireRule = new ObjectMapper().readTree("{"
                + "\"rule_id\":\"rule-1\",\"tenant_id\":\"tenant-1\","
                + "\"name\":\"known 攻击\",\"type\":\"port_scan\","
                + "\"engine\":\"internal\",\"description\":\"fixture <wire>\","
                + "\"conditions\":{\"ports\":[80,443],\"threshold\":1.25},"
                + "\"labels\":[\"known\",\"scan\"],\"severity\":\"high\","
                + "\"enabled\":true,\"version\":7,\"priority\":60,"
                + "\"created_by\":\"operator-1\",\"created_at\":1720000000000,"
                + "\"updated_at\":1720000001000}");

        assertEquals("b7b368d7a2b6544d5bf95e0b347ff9a5",
                RuleCommandChecksum.calculate(wireRule));
    }

    @Test
    void historicalFlattenedRuleRemainsReadable() {
        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(source(
                "{\"rule_id\":\"rule-1\",\"tenant_id\":\"tenant-1\","
                        + "\"type\":\"port_scan\",\"enabled\":true,\"version\":3,"
                        + "\"action\":\"update\"}"));

        assertNotNull(result.rule);
        assertEquals(3L, result.rule.getVersion());
        assertEquals(false, result.rule.isCanonicalCommandEnvelope());
    }

    @Test
    void versionOneCommandRemainsReadableButDoesNotClaimVerifiableReceiptIdentity() {
        String payload = canonicalCommand("update", 3, canonicalRuleChecksum())
                .replace("\"schema_version\":\"1.1\",", "\"schema_version\":\"1.0\",")
                .replace("\"checksum_algorithm\":\"" + RuleCommandChecksum.ALGORITHM + "\",", "");
        RawKafkaRecord legacy = new RawKafkaRecord(
                "rule.updates", 0, 11L, 2_500L,
                "rule-1".getBytes(StandardCharsets.UTF_8),
                payload.getBytes(StandardCharsets.UTF_8),
                Map.of(
                        "event_id", "event-1",
                        "action", "update",
                        "tenant_id", "tenant-1",
                        "rule_id", "rule-1",
                        "rule_version", "v3",
                        "schema_version", "1.0",
                        "checksum", canonicalRuleChecksum()));

        RuleJsonParseFunction.ParseResult result = RuleJsonParseFunction.parse(legacy);

        assertNotNull(result.rule);
        assertEquals(false, result.rule.isCanonicalCommandEnvelope());
    }

    private static String canonicalCommand(String action, long version, String checksum) {
        return "{"
                + "\"event_id\":\"event-1\","
                + "\"schema_version\":\"1.1\","
                + "\"checksum_algorithm\":\"" + RuleCommandChecksum.ALGORITHM + "\","
                + "\"action\":\"" + action + "\","
                + "\"timestamp\":2000,"
                + "\"operator_id\":\"operator-1\","
                + "\"version\":" + version + ","
                + "\"rule_version\":\"v" + version + "\","
                + "\"checksum\":\"" + checksum + "\","
                + "\"rule\":{"
                + "\"rule_id\":\"rule-1\",\"tenant_id\":\"tenant-1\","
                + "\"type\":\"port_scan\",\"enabled\":true,"
                + "\"version\":" + version
                + "}}";
    }

    private static String canonicalRuleChecksum() {
        try {
            return RuleCommandChecksum.calculate(new ObjectMapper().readTree("{"
                    + "\"rule_id\":\"rule-1\",\"tenant_id\":\"tenant-1\","
                    + "\"type\":\"port_scan\",\"enabled\":true,\"version\":3}"));
        } catch (Exception error) {
            throw new IllegalStateException(error);
        }
    }

    private static RawKafkaRecord source(String json) {
        return new RawKafkaRecord(
                "rule.updates", 0, 11L, 2_500L,
                "rule-1".getBytes(StandardCharsets.UTF_8),
                json.getBytes(StandardCharsets.UTF_8),
                Map.of(
                        "event_id", "event-1",
                        "action", "update",
                        "tenant_id", "tenant-1",
                        "rule_id", "rule-1",
                        "rule_version", "v3",
                        "schema_version", "1.1",
                        "checksum_algorithm", RuleCommandChecksum.ALGORITHM,
                        "checksum", canonicalRuleChecksum(),
                        "trace_id", "trace-1"));
    }
}
