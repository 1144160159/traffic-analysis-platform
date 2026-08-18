package com.traffic.flink.rule.sink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleUpdateAppliedAck;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class RuleUpdateAckKafkaSinkFactoryTest {

    @Test
    void receiptIsKeyedByStableCommandEventAndCarriesSubtaskCoordinates() throws Exception {
        Rule rule = new Rule();
        rule.setRuleId("rule-1");
        rule.setTenantId("tenant-1");
        rule.setVersion(7);
        rule.setActionStr("update");
        rule.setCommandEventId("event-7");
        rule.setCommandChecksum("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        RuleUpdateAppliedAck ack = RuleUpdateAppliedAck.from(
                rule, 7, "applied", "", 2, 4);

        ProducerRecord<byte[], byte[]> record =
                new RuleUpdateAckKafkaSinkFactory.AckSerializationSchema(
                        "rule-update-applied.v1").serialize(ack, null, 3_000L);
        JsonNode payload = new ObjectMapper().readTree(record.value());

        assertEquals("rule-update-applied.v1", record.topic());
        assertEquals("event-7", new String(record.key(), StandardCharsets.UTF_8));
        assertEquals("event-7", payload.get("event_id").asText());
        assertEquals(2, payload.get("subtask_index").asInt());
        assertEquals(4, payload.get("parallelism").asInt());
        assertEquals("applied", payload.get("status").asText());
    }

    @Test
    void nonCanonicalTopicIsRejected() {
        assertThrows(IllegalArgumentException.class,
                () -> RuleUpdateAckKafkaSinkFactory.create("broker:9092", "rule-ack-dev"));
    }
}
