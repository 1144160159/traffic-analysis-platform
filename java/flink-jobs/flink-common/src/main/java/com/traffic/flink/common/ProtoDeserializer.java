package com.traffic.flink.common;

import com.google.protobuf.InvalidProtocolBufferException;
import com.google.protobuf.Descriptors.FieldDescriptor;
import com.google.protobuf.Message;
import com.google.protobuf.Parser;
import com.google.protobuf.UnknownFieldSet;
import org.apache.flink.api.common.serialization.DeserializationSchema;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Flink Protobuf 反序列化器
 *
 * @param <T> Protobuf 消息类型
 */
public class ProtoDeserializer<T extends Message> implements DeserializationSchema<T> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ProtoDeserializer.class);

    private final Class<T> messageClass;
    private final boolean skipOnError;
    private final boolean stripUnknownFields;

    private transient Parser<T> parser;
    private transient long successCount = 0;
    private transient long errorCount = 0;

    /**
     * 创建反序列化器
     *
     * @param messageClass Protobuf 消息类
     */
    public ProtoDeserializer(Class<T> messageClass) {
        this(messageClass, true, false);
    }

    /**
     * 创建反序列化器
     *
     * @param messageClass Protobuf 消息类
     * @param skipOnError  是否跳过解析错误
     */
    public ProtoDeserializer(Class<T> messageClass, boolean skipOnError) {
        this(messageClass, skipOnError, false);
    }

    /**
     * 创建反序列化器
     *
     * @param messageClass        Protobuf 消息类
     * @param skipOnError         是否跳过解析错误
     * @param stripUnknownFields  是否递归清理 unknown fields，避免 Flink Kryo 复制 Protobuf 内部结构时失败
     */
    public ProtoDeserializer(Class<T> messageClass, boolean skipOnError, boolean stripUnknownFields) {
        this.messageClass = messageClass;
        this.skipOnError = skipOnError;
        this.stripUnknownFields = stripUnknownFields;
    }

    @SuppressWarnings("unchecked")
    private Parser<T> getParser() {
        if (parser == null) {
            try {
                Method parserMethod = messageClass.getMethod("parser");
                parser = (Parser<T>) parserMethod.invoke(null);
            } catch (NoSuchMethodException | IllegalAccessException | InvocationTargetException e) {
                throw new RuntimeException("Failed to get parser for " + messageClass.getName(), e);
            }
        }
        return parser;
    }

    @Override
    public T deserialize(byte[] message) throws IOException {
        if (message == null || message.length == 0) {
            if (skipOnError) {
                errorCount++;
                LOG.warn("Empty message received, skipping");
                return null;
            }
            throw new IOException("Empty message received");
        }

        try {
            T result = getParser().parseFrom(message);
            successCount++;
            return stripUnknownFields ? stripUnknownFields(result) : result;
        } catch (InvalidProtocolBufferException e) {
            errorCount++;
            if (skipOnError) {
                LOG.warn("Failed to parse protobuf message: {}", e.getMessage());
                return null;
            }
            throw new IOException("Failed to parse protobuf message", e);
        }
    }

    @Override
    public boolean isEndOfStream(T nextElement) {
        return false;
    }

    @Override
    public TypeInformation<T> getProducedType() {
        return TypeInformation.of(messageClass);
    }

    public long getSuccessCount() {
        return successCount;
    }

    public long getErrorCount() {
        return errorCount;
    }

    @SuppressWarnings("unchecked")
    private T stripUnknownFields(T message) {
        return (T) stripUnknownFieldsRecursive(message);
    }

    private Message stripUnknownFieldsRecursive(Message message) {
        boolean changed = !message.getUnknownFields().asMap().isEmpty();
        Message.Builder builder = null;

        for (Map.Entry<FieldDescriptor, Object> entry : message.getAllFields().entrySet()) {
            FieldDescriptor field = entry.getKey();
            if (field.getJavaType() != FieldDescriptor.JavaType.MESSAGE) {
                continue;
            }

            if (field.isRepeated()) {
                List<?> values = (List<?>) entry.getValue();
                List<Message> strippedValues = new ArrayList<>(values.size());
                boolean fieldChanged = false;

                for (Object value : values) {
                    Message child = (Message) value;
                    Message strippedChild = stripUnknownFieldsRecursive(child);
                    strippedValues.add(strippedChild);
                    fieldChanged = fieldChanged || strippedChild != child;
                }

                if (fieldChanged) {
                    if (builder == null) {
                        builder = message.toBuilder();
                    }
                    builder.clearField(field);
                    for (Message strippedValue : strippedValues) {
                        builder.addRepeatedField(field, strippedValue);
                    }
                    changed = true;
                }
            } else {
                Message child = (Message) entry.getValue();
                Message strippedChild = stripUnknownFieldsRecursive(child);
                if (strippedChild != child) {
                    if (builder == null) {
                        builder = message.toBuilder();
                    }
                    builder.setField(field, strippedChild);
                    changed = true;
                }
            }
        }

        if (!changed) {
            return message;
        }

        if (builder == null) {
            builder = message.toBuilder();
        }
        builder.setUnknownFields(UnknownFieldSet.getDefaultInstance());
        return builder.build();
    }
}
