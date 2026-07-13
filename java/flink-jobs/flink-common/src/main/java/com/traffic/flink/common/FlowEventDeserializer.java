// java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/FlowEventDeserializer.java
package com.traffic.flink.common;

import com.traffic.proto.traffic.v1.FlowEvent;
import org.apache.flink.api.common.serialization.DeserializationSchema;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * FlowEvent Protobuf 反序列化器 (修复版)
 *
 * 修复内容:
 *   - 添加 null/空消息保护
 *   - 添加反序列化错误计数器
 *   - 使用 SLF4J 日志替代 RuntimeException 静默吞错
 */
public class FlowEventDeserializer implements DeserializationSchema<FlowEvent> {

    private static final Logger LOG = LoggerFactory.getLogger(FlowEventDeserializer.class);
    private static final long serialVersionUID = 1L;

    // 反序列化错误计数（用于监控告警）
    private long deserializeErrorCount = 0;
    private long totalMessageCount = 0;

    @Override
    public FlowEvent deserialize(byte[] message) {
        totalMessageCount++;

        // 修复: null/空消息保护
        if (message == null || message.length == 0) {
            deserializeErrorCount++;
            if (deserializeErrorCount % 1000 == 1) {
                LOG.warn("Null or empty message received (total errors: {})", deserializeErrorCount);
            }
            return null;
        }

        try {
            FlowEvent event = FlowEvent.parseFrom(message);

            // 修复: 验证必要字段
            if (event.getHeader().getTenantId().isEmpty()) {
                deserializeErrorCount++;
                LOG.debug("FlowEvent missing tenant_id, skipping");
                return null;
            }

            return event;
        } catch (Exception e) {
            deserializeErrorCount++;
            if (deserializeErrorCount % 1000 == 1) {
                LOG.error("Failed to deserialize FlowEvent ({} bytes, total errors: {})",
                        message.length, deserializeErrorCount, e);
            }
            return null;
        }
    }

    @Override
    public boolean isEndOfStream(FlowEvent nextElement) {
        return false;
    }

    @Override
    public TypeInformation<FlowEvent> getProducedType() {
        return TypeInformation.of(FlowEvent.class);
    }

    /** 获取反序列化错误总数 */
    public long getDeserializeErrorCount() {
        return deserializeErrorCount;
    }

    /** 获取已处理消息总数 */
    public long getTotalMessageCount() {
        return totalMessageCount;
    }

    /** 获取错误率 */
    public double getErrorRate() {
        return totalMessageCount > 0
                ? (double) deserializeErrorCount / totalMessageCount
                : 0.0;
    }
}