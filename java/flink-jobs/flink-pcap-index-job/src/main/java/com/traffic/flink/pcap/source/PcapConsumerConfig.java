package com.traffic.flink.pcap.source;

import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;

import java.io.Serializable;
import java.util.Properties;

/** Immutable startup contract for the dormant N009 consumer-first topology. */
public final class PcapConsumerConfig implements Serializable {
    private static final long serialVersionUID = 1L;
    public static final String INPUT_TOPIC = "pcap.index.v1";
    public static final String DLQ_TOPIC = "dlq.v1";
    public static final String SOURCE_UID = "pcap-raw-kafka-source-v1";
    public static final String PARSE_UID = "pcap-raw-parse-v1";
    public static final String DLQ_UID = "pcap-raw-canonical-dlq-v1";

    private final String brokers;
    private final String inputTopic;
    private final String groupId;
    private final String dlqTopic;
    private final Properties kafkaProperties;
    private final OffsetsInitializer startingOffsets;
    private final boolean dlqWriteDescribeAuthorized;
    private final boolean idempotentWriteAuthorized;
    private final String downstreamCapability;

    public PcapConsumerConfig(String brokers, String inputTopic, String groupId, String dlqTopic,
                              Properties kafkaProperties, OffsetsInitializer startingOffsets,
                              boolean dlqWriteDescribeAuthorized, boolean idempotentWriteAuthorized,
                              String downstreamCapability) {
        this.brokers = brokers; this.inputTopic = inputTopic; this.groupId = groupId; this.dlqTopic = dlqTopic;
        this.kafkaProperties = copy(kafkaProperties); this.startingOffsets = startingOffsets;
        this.dlqWriteDescribeAuthorized = dlqWriteDescribeAuthorized;
        this.idempotentWriteAuthorized = idempotentWriteAuthorized;
        this.downstreamCapability = downstreamCapability;
        validate();
    }

    public void validate() {
        if (blank(brokers) || blank(groupId) || startingOffsets == null) {
            throw new IllegalArgumentException("PCAP consumer brokers group and offsets are required");
        }
        if (!INPUT_TOPIC.equals(inputTopic) || !DLQ_TOPIC.equals(dlqTopic)) {
            throw new IllegalArgumentException("PCAP consumer topics must match the canonical contract");
        }
        if (!dlqWriteDescribeAuthorized || !idempotentWriteAuthorized) {
            throw new IllegalArgumentException("PCAP consumer canonical DLQ ACL capability is absent");
        }
        if (!"N010_N011_REVIEWED_CARRIER_SINK".equals(downstreamCapability)) {
            throw new IllegalArgumentException("PCAP live source requires reviewed downstream carrier capability");
        }
        String acks = kafkaProperties.getProperty("acks", "all");
        if (!"all".equalsIgnoreCase(acks) && !"-1".equals(acks)) {
            throw new IllegalArgumentException("PCAP canonical DLQ requires acks=all");
        }
        if (!"false".equalsIgnoreCase(kafkaProperties.getProperty("enable.auto.commit", "false"))
                || !"true".equalsIgnoreCase(
                kafkaProperties.getProperty("commit.offsets.on.checkpoint", "true"))) {
            throw new IllegalArgumentException(
                    "PCAP source offsets must commit only on successful checkpoints");
        }
    }

    public String getBrokers() { return brokers; }
    public String getInputTopic() { return inputTopic; }
    public String getGroupId() { return groupId; }
    public String getDlqTopic() { return dlqTopic; }
    public Properties getKafkaProperties() { return copy(kafkaProperties); }
    public Properties getCheckpointBoundSourceProperties() {
        Properties source = copy(kafkaProperties);
        source.setProperty("enable.auto.commit", "false");
        source.setProperty("commit.offsets.on.checkpoint", "true");
        source.setProperty("isolation.level", "read_committed");
        return source;
    }
    public OffsetsInitializer getStartingOffsets() { return startingOffsets; }
    public String getDownstreamCapability() { return downstreamCapability; }

    private static Properties copy(Properties source) {
        Properties copy = new Properties();
        if (source != null) copy.putAll(source);
        return copy;
    }
    private static boolean blank(String value) { return value == null || value.trim().isEmpty(); }
}
