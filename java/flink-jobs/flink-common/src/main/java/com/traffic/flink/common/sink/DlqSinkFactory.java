package com.traffic.flink.common.sink;

import org.apache.flink.connector.kafka.sink.KafkaSink;

import java.io.Serializable;

/**
 * DLQ sink 工厂契约(GoF Abstract Factory 产品族成员之一)。
 *
 * 产品族一致性约束:同一作业的 serializer/DLQ/ack 三件套必须同源(同一
 * 工厂装配),避免"序列化格式与 DLQ 语义各自漂移"。flink-rule-job 的
 * RuleDlqSinkFactory 是首个实现者。
 *
 * @param <T> DLQ 消息类型(建议 CanonicalDlqMessage)
 */
public interface DlqSinkFactory<T> extends Serializable {

    KafkaSink<T> createSink(String brokers, String topic);
}
