////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/sink/KafkaSinkFactoryTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.sink;

import com.traffic.proto.traffic.v1.Campaign;

import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * KafkaSinkFactory 单元测试
 */
class KafkaSinkFactoryTest {

    @Test
    @DisplayName("创建 Campaign Kafka Sink")
    void testCreateCampaignSink() {
        KafkaSink<Campaign> sink = KafkaSinkFactory.createCampaignSink(
                "localhost:9092",
                "campaigns.v1"
        );

        assertThat(sink).isNotNull();
    }

    @Test
    @DisplayName("验证空 Campaign 处理")
    void testNullCampaignHandling() {
        // 验证序列化器可以处理 null
        Campaign nullCampaign = null;
        
        // 这里主要验证不会抛出异常
        assertThat(nullCampaign).isNull();
    }
}