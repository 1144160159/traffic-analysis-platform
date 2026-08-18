package com.traffic.flink.common;

import com.google.protobuf.Message;
import org.apache.flink.api.common.ExecutionConfig;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.api.common.typeutils.TypeSerializer;

import java.util.Objects;

/** Explicit Protobuf wire type information; generated messages must not use reflective Kryo. */
public final class ProtoTypeInformation<T extends Message> extends TypeInformation<T> {
    private static final long serialVersionUID = 1L;
    private final Class<T> typeClass;

    private ProtoTypeInformation(Class<T> typeClass) {
        this.typeClass = Objects.requireNonNull(typeClass, "typeClass");
    }

    public static <T extends Message> ProtoTypeInformation<T> forMessage(Class<T> typeClass) {
        return new ProtoTypeInformation<>(typeClass);
    }

    @Override public boolean isBasicType() { return false; }
    @Override public boolean isTupleType() { return false; }
    @Override public int getArity() { return 1; }
    @Override public int getTotalFields() { return 1; }
    @Override public Class<T> getTypeClass() { return typeClass; }
    @Override public boolean isKeyType() { return false; }

    @Override
    public TypeSerializer<T> createSerializer(ExecutionConfig config) {
        return new ProtoTypeSerializer<>(typeClass);
    }

    @Override public String toString() { return "Protobuf<" + typeClass.getName() + ">"; }
    @Override public boolean canEqual(Object value) { return value instanceof ProtoTypeInformation; }

    @Override
    public boolean equals(Object value) {
        return value == this || value instanceof ProtoTypeInformation
                && typeClass.equals(((ProtoTypeInformation<?>) value).typeClass);
    }

    @Override public int hashCode() { return typeClass.hashCode(); }
}
