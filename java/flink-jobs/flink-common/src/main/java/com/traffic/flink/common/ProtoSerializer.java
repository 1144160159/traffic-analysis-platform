package com.traffic.flink.common;

import com.google.protobuf.Message;
import org.apache.flink.api.common.serialization.SerializationSchema;

/**
 * Flink Protobuf 序列化器
 *
 * @param <T> Protobuf 消息类型
 */
public class ProtoSerializer<T extends Message> implements SerializationSchema<T> {

    private static final long serialVersionUID = 1L;

    @Override
    public byte[] serialize(T element) {
        if (element == null) {
            return new byte[0];
        }
        return element.toByteArray();
    }
}