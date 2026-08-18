package com.traffic.flink.alert.router;

/**
 * RunRouterJob 配置(env 注入;topic 按合同钉死,防止误配跨合同主题)。
 */
public final class RunRouterJobConfig {

    private final String kafkaBrokers;
    private final String flowTopic = "flow.events.v1";
    private final String subscriptionTopic = "analysis.run.events.v1";
    private final String envelopeTopic = "analysis.envelopes.v1";
    private final String flowConsumerGroup;
    private final String subscriptionConsumerGroup;
    private final String dlqTopic = "dlq.v1";

    public RunRouterJobConfig(
            String kafkaBrokers, String flowConsumerGroup, String subscriptionConsumerGroup) {
        if (kafkaBrokers == null || kafkaBrokers.isBlank()) {
            throw new IllegalArgumentException("kafka.brokers is required");
        }
        this.kafkaBrokers = kafkaBrokers;
        this.flowConsumerGroup = flowConsumerGroup == null || flowConsumerGroup.isBlank()
                ? "run-router-flow-v1" : flowConsumerGroup;
        this.subscriptionConsumerGroup = subscriptionConsumerGroup == null || subscriptionConsumerGroup.isBlank()
                ? "run-router-subscription-v1" : subscriptionConsumerGroup;
    }

    public String kafkaBrokers() { return kafkaBrokers; }
    public String flowTopic() { return flowTopic; }
    public String subscriptionTopic() { return subscriptionTopic; }
    public String envelopeTopic() { return envelopeTopic; }
    public String flowConsumerGroup() { return flowConsumerGroup; }
    public String subscriptionConsumerGroup() { return subscriptionConsumerGroup; }
    public String dlqTopic() { return dlqTopic; }

    public static RunRouterJobConfig fromEnv() {
        return new RunRouterJobConfig(
                System.getenv().getOrDefault("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092"),
                System.getenv("RUN_ROUTER_FLOW_GROUP"),
                System.getenv("RUN_ROUTER_SUBSCRIPTION_GROUP"));
    }
}
