package com.traffic.flink.common;

import com.google.protobuf.Message;
import com.google.protobuf.Parser;
import org.apache.flink.api.common.typeutils.TypeSerializer;
import org.apache.flink.api.common.typeutils.TypeSerializerSchemaCompatibility;
import org.apache.flink.api.common.typeutils.TypeSerializerSnapshot;
import org.apache.flink.core.memory.DataInputView;
import org.apache.flink.core.memory.DataOutputView;

import java.io.IOException;
import java.lang.reflect.Method;
import java.util.Objects;

/** Length-delimited Protobuf serializer with a stable Flink snapshot contract. */
public final class ProtoTypeSerializer<T extends Message> extends TypeSerializer<T> {
    private static final long serialVersionUID = 1L;
    private static final int MAX_MESSAGE_BYTES = 64 * 1024 * 1024;

    private final String messageClassName;
    private transient Parser<T> parser;
    private transient T defaultInstance;

    public ProtoTypeSerializer(Class<T> messageClass) {
        this(messageClass.getName(), messageClass.getClassLoader());
    }

    private ProtoTypeSerializer(String messageClassName, ClassLoader classLoader) {
        this.messageClassName = Objects.requireNonNull(messageClassName, "messageClassName");
        initialize(classLoader);
    }

    @SuppressWarnings("unchecked")
    private void initialize(ClassLoader classLoader) {
        try {
            Class<?> raw = Class.forName(messageClassName, true, classLoader);
            if (!Message.class.isAssignableFrom(raw)) {
                throw new IllegalArgumentException(messageClassName + " is not a Protobuf Message");
            }
            Method getDefaultInstance = raw.getMethod("getDefaultInstance");
            defaultInstance = (T) getDefaultInstance.invoke(null);
            parser = (Parser<T>) defaultInstance.getParserForType();
        } catch (ReflectiveOperationException error) {
            throw new IllegalArgumentException(
                    "cannot initialize Protobuf serializer for " + messageClassName, error);
        }
    }

    private Parser<T> parser() {
        if (parser == null) initialize(Thread.currentThread().getContextClassLoader());
        return parser;
    }

    @Override public boolean isImmutableType() { return true; }
    @Override public TypeSerializer<T> duplicate() { return this; }
    @Override public T createInstance() {
        if (defaultInstance == null) initialize(Thread.currentThread().getContextClassLoader());
        return defaultInstance;
    }
    @Override public T copy(T from) { return from; }
    @Override public T copy(T from, T reuse) { return from; }
    @Override public int getLength() { return -1; }

    @Override
    public void serialize(T record, DataOutputView target) throws IOException {
        if (record == null) throw new IllegalArgumentException("Protobuf record is null");
        byte[] bytes = record.toByteArray();
        validateLength(bytes.length);
        target.writeInt(bytes.length);
        target.write(bytes);
    }

    @Override
    public T deserialize(DataInputView source) throws IOException {
        int length = source.readInt();
        validateLength(length);
        byte[] bytes = new byte[length];
        source.readFully(bytes);
        return parser().parseFrom(bytes);
    }

    @Override public T deserialize(T reuse, DataInputView source) throws IOException {
        return deserialize(source);
    }

    @Override
    public void copy(DataInputView source, DataOutputView target) throws IOException {
        int length = source.readInt();
        validateLength(length);
        target.writeInt(length);
        target.write(source, length);
    }

    private static void validateLength(int length) throws IOException {
        if (length < 0 || length > MAX_MESSAGE_BYTES) {
            throw new IOException("invalid Protobuf record length: " + length);
        }
    }

    @Override
    public TypeSerializerSnapshot<T> snapshotConfiguration() {
        return new Snapshot<>(messageClassName);
    }

    @Override
    public boolean equals(Object value) {
        return value == this || value instanceof ProtoTypeSerializer
                && messageClassName.equals(((ProtoTypeSerializer<?>) value).messageClassName);
    }

    @Override public int hashCode() { return messageClassName.hashCode(); }

    public static final class Snapshot<T extends Message> implements TypeSerializerSnapshot<T> {
        private String messageClassName;
        private transient ClassLoader classLoader;

        public Snapshot() {}
        private Snapshot(String messageClassName) { this.messageClassName = messageClassName; }

        @Override public int getCurrentVersion() { return 1; }
        @Override public void writeSnapshot(DataOutputView out) throws IOException {
            out.writeUTF(messageClassName);
        }

        @Override
        public void readSnapshot(int version, DataInputView in, ClassLoader userCodeClassLoader)
                throws IOException {
            if (version != 1) throw new IOException("unsupported Protobuf serializer version " + version);
            messageClassName = in.readUTF();
            classLoader = userCodeClassLoader;
        }

        @Override
        public TypeSerializer<T> restoreSerializer() {
            ClassLoader loader = classLoader == null
                    ? Thread.currentThread().getContextClassLoader() : classLoader;
            return new ProtoTypeSerializer<>(messageClassName, loader);
        }

        @Override
        public TypeSerializerSchemaCompatibility<T> resolveSchemaCompatibility(
                TypeSerializer<T> newSerializer) {
            if (newSerializer instanceof ProtoTypeSerializer
                    && messageClassName.equals(
                            ((ProtoTypeSerializer<?>) newSerializer).messageClassName)) {
                return TypeSerializerSchemaCompatibility.compatibleAsIs();
            }
            return TypeSerializerSchemaCompatibility.incompatible();
        }
    }
}
