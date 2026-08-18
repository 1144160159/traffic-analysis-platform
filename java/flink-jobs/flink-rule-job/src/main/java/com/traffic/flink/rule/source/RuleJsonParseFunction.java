package com.traffic.flink.rule.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleAction;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Parses versioned rule updates and keeps the previous broadcast state on rejection. */
public final class RuleJsonParseFunction extends ProcessFunction<RawKafkaRecord, Rule> {

    private static final long serialVersionUID = 1L;
    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private transient ObjectMapper mapper;

    public RuleJsonParseFunction(OutputTag<CanonicalDlqMessage> dlqTag) {
        this.dlqTag = dlqTag;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        mapper = parserMapper();
    }

    @Override
    public void processElement(RawKafkaRecord source, Context context, Collector<Rule> out) {
        ParseResult result = parse(source, mapper);
        if (result.rule != null) out.collect(result.rule);
        else context.output(dlqTag, result.failure);
    }

    static ParseResult parse(RawKafkaRecord source) {
        return parse(source, parserMapper());
    }

    private static ParseResult parse(RawKafkaRecord source, ObjectMapper mapper) {
        byte[] payload = source.getValue();
        if (payload == null || payload.length == 0) {
            return failure(source, null, "BAD_SCHEMA", "parse_error", "empty rule update payload");
        }
        Rule rule;
        JsonNode root;
        try {
            root = mapper.readTree(payload);
            if (root == null || !root.isObject()) {
                return failure(source, null, "BAD_SCHEMA", "parse_error",
                        "rule update payload must be a JSON object");
            }
            if (root.has("rule")) {
                JsonNode ruleNode = root.get("rule");
                if (ruleNode == null || !ruleNode.isObject()) {
                    return failure(source, null, "BAD_SCHEMA", "parse_error",
                            "RuleCommandV1Json rule must be an object");
                }
                rule = mapper.treeToValue(ruleNode, Rule.class);
                applyCommandMetadata(root, rule);
            } else {
                // Compatibility reader for the historical flattened rule
                // payload. New producers must emit RuleCommandV1Json.
                rule = mapper.treeToValue(root, Rule.class);
                rule.setCommandEventId(source.header("event_id"));
                rule.setCommandChecksum(source.header("checksum"));
                rule.setCommandSchemaVersion(source.header("schema_version"));
                rule.setCommandTimestamp(parsePositiveLong(source.header("event_ts")));
                rule.setCanonicalCommandEnvelope(false);
            }
        } catch (Exception error) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "invalid rule update JSON: " + error.getMessage());
        }
        String validationError = root.has("rule")
                ? validateCommand(source, root, rule)
                : validate(rule);
        if (validationError != null) {
            return failure(source, rule, "VALIDATION_ERROR", "validation_error", validationError);
        }
        return new ParseResult(rule, null);
    }

    private static void applyCommandMetadata(JsonNode root, Rule rule) {
        String action = text(root, "action");
        rule.setActionStr(action);
        if ((rule.getUpdatedBy() == null || rule.getUpdatedBy().isBlank())
                && !text(root, "operator_id").isBlank()) {
            rule.setUpdatedBy(text(root, "operator_id"));
        }
        rule.setCommandEventId(text(root, "event_id"));
        rule.setCommandTimestamp(root.path("timestamp").asLong(0));
        rule.setCommandChecksum(text(root, "checksum"));
        rule.setCommandSchemaVersion(text(root, "schema_version"));
        rule.setCommandChecksumAlgorithm(text(root, "checksum_algorithm"));
        rule.setCanonicalCommandEnvelope("1.1".equals(rule.getCommandSchemaVersion()));
    }

    private static String validateCommand(RawKafkaRecord source, JsonNode root, Rule rule) {
        String baseError = validate(rule);
        if (baseError != null) return baseError;

        String eventId = text(root, "event_id");
        String action = text(root, "action");
        String operatorId = text(root, "operator_id");
        String schemaVersion = text(root, "schema_version");
        String checksumAlgorithm = text(root, "checksum_algorithm");
        String ruleVersion = text(root, "rule_version");
        String checksum = text(root, "checksum");
        long timestamp = root.path("timestamp").asLong(0);
        long version = root.path("version").asLong(0);

        if (eventId.isBlank()) return "missing command event_id";
        if (!"1.0".equals(schemaVersion) && !"1.1".equals(schemaVersion)) {
            return "unsupported command schema_version";
        }
        if (operatorId.isBlank()) return "missing command operator_id";
        if (timestamp <= 0) return "command timestamp must be positive";
        if (version != rule.getVersion()) return "command version does not match nested rule version";
        if (!ruleVersion.equals("v" + rule.getVersion())) {
            return "command rule_version does not match nested rule version";
        }
        if (!checksum.matches("[0-9a-f]{32}")) return "invalid command checksum";
        if ("1.1".equals(schemaVersion)) {
            if (!RuleCommandChecksum.ALGORITHM.equals(checksumAlgorithm)) {
                return "unsupported command checksum_algorithm";
            }
            try {
                if (!checksum.equals(RuleCommandChecksum.calculate(root.get("rule")))) {
                    return "command checksum does not match nested rule";
                }
            } catch (IllegalArgumentException error) {
                return "invalid nested rule checksum input";
            }
        }

        String key = source.keyAsString();
        if (!key.isBlank() && !key.equals(rule.getRuleId())) return "Kafka key does not match rule_id";
        if (!matchesIfPresent(source.header("event_id"), eventId)) return "Kafka event_id header mismatch";
        if (!matchesIfPresent(source.header("action"), action)) return "Kafka action header mismatch";
        if (!matchesIfPresent(source.header("tenant_id"), rule.getTenantId())) return "Kafka tenant_id header mismatch";
        if (!matchesIfPresent(source.header("rule_id"), rule.getRuleId())) return "Kafka rule_id header mismatch";
        if (!matchesIfPresent(source.header("rule_version"), ruleVersion)) return "Kafka rule_version header mismatch";
        if (!matchesIfPresent(source.header("schema_version"), schemaVersion)) return "Kafka schema_version header mismatch";
        if (!matchesIfPresent(source.header("checksum_algorithm"), checksumAlgorithm)) return "Kafka checksum_algorithm header mismatch";
        if (!matchesIfPresent(source.header("checksum"), checksum)) return "Kafka checksum header mismatch";
        return null;
    }

    private static ObjectMapper parserMapper() {
        return new ObjectMapper()
                .enable(DeserializationFeature.USE_BIG_DECIMAL_FOR_FLOATS)
                .enable(DeserializationFeature.USE_BIG_INTEGER_FOR_INTS);
    }

    static String validate(Rule rule) {
        if (rule.getRuleId() == null || rule.getRuleId().trim().isEmpty()) return "missing rule_id";
        if (rule.getTenantId() == null || rule.getTenantId().trim().isEmpty()) return "missing tenant_id";
        if (rule.getVersion() <= 0) return "rule version must be positive";
        if (rule.getActionStr() == null || rule.getActionStr().trim().isEmpty()) return "missing rule action";
        try {
            RuleAction.valueOf(rule.getActionStr().trim().toUpperCase());
        } catch (IllegalArgumentException error) {
            return "unsupported rule action";
        }
        RuleAction action = RuleAction.valueOf(rule.getActionStr().trim().toUpperCase());
        if (action == RuleAction.CREATE
                || action == RuleAction.UPDATE
                || action == RuleAction.ENABLE
                || action == RuleAction.SYNC) {
            if (rule.getRuleTypeStr() == null || rule.getRuleTypeStr().trim().isEmpty()) {
                return "missing rule type for upsert action";
            }
            boolean supportedType = false;
            for (com.traffic.flink.rule.model.RuleType type
                    : com.traffic.flink.rule.model.RuleType.values()) {
                if (type.getValue().equalsIgnoreCase(rule.getRuleTypeStr())) {
                    supportedType = true;
                    break;
                }
            }
            if (!supportedType) return "unsupported rule type";
        }
        return null;
    }

    private static boolean matchesIfPresent(String actual, String expected) {
        return actual == null || actual.isBlank() || actual.equals(expected);
    }

    private static String text(JsonNode node, String field) {
        JsonNode value = node.get(field);
        return value == null || value.isNull() ? "" : value.asText("").trim();
    }

    private static long parsePositiveLong(String value) {
        try {
            long parsed = Long.parseLong(value);
            return parsed > 0 ? parsed : 0;
        } catch (Exception ignored) {
            return 0;
        }
    }

    private static ParseResult failure(
            RawKafkaRecord source, Rule rule,
            String code, String type, String message) {
        String tenantId = rule != null && rule.getTenantId() != null
                ? rule.getTenantId() : source.header("tenant_id");
        String eventId = rule != null && rule.getCommandEventId() != null
                && !rule.getCommandEventId().isBlank()
                ? rule.getCommandEventId()
                : rule != null && rule.getRuleId() != null ? rule.getRuleId() : "";
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, code, type, message,
                tenantId, eventId, source.header("trace_id"), source.header("run_id"), "",
                "flink-rule-job", "application/json", "traffic.rule.update", "v1");
        return new ParseResult(null, failure);
    }

    static final class ParseResult {
        final Rule rule;
        final CanonicalDlqMessage failure;

        ParseResult(Rule rule, CanonicalDlqMessage failure) {
            this.rule = rule;
            this.failure = failure;
        }
    }
}
