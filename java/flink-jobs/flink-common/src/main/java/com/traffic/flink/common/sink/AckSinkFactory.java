package com.traffic.flink.common.sink;

import org.apache.flink.connector.kafka.sink.KafkaSink;

import java.io.Serializable;

/**
 * 规则更新 ACK sink 工厂契约(GoF Abstract Factory 产品族成员之一)。
 * 与 DlqSinkFactory 同属"同源产品族":ack 与 dlq 必须由同一装配逻辑产出。
 *
 * @param <T> ACK 消息类型
 */
public interface AckSinkFactory<T> extends Serializable {

    KafkaSink<T> createSink(String brokers, String topic);
}
