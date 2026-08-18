package com.traffic.flink.alert.router;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * RunSubscription 订阅记录(analysis.run.events.v1 信封,JSON;与 Go contract.RunSubscription 对齐)。
 * 字段加法式演进;未知字段忽略。
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public final class RunSubscriptionRecord implements java.io.Serializable {

    private static final long serialVersionUID = 1L;

    @JsonProperty("schema_version")
    private String schemaVersion;

    @JsonProperty("tenant_id")
    private String tenantId;

    @JsonProperty("run_id")
    private String runId;

    @JsonProperty("revision")
    private long revision;

    @JsonProperty("state")
    private String state;

    @JsonProperty("execution_spec_sha256")
    private String executionSpecSha256;

    @JsonProperty("window_start_ms")
    private long windowStartMs;

    @JsonProperty("window_end_ms")
    private long windowEndMs;

    @JsonProperty("fence")
    private String fence;

    public String schemaVersion() { return schemaVersion; }
    public String tenantId() { return tenantId; }
    public String runId() { return runId; }
    public long revision() { return revision; }
    public String state() { return state; }
    public String executionSpecSha256() { return executionSpecSha256; }
    public long windowStartMs() { return windowStartMs; }
    public long windowEndMs() { return windowEndMs; }
    public String fence() { return fence; }

    /** 广播状态键:tenant|run_id(租户内 run_id 唯一)。 */
    public String stateKey() {
        return tenantId + "|" + runId;
    }

    /** 测试/装配工厂(生产路径经 Jackson 反序列化)。 */
    public static RunSubscriptionRecord of(
            String tenantId, String runId, long revision, String state,
            String executionSpecSha256, long windowStartMs, long windowEndMs, String fence) {
        RunSubscriptionRecord r = new RunSubscriptionRecord();
        r.schemaVersion = "1";
        r.tenantId = tenantId;
        r.runId = runId;
        r.revision = revision;
        r.state = state;
        r.executionSpecSha256 = executionSpecSha256;
        r.windowStartMs = windowStartMs;
        r.windowEndMs = windowEndMs;
        r.fence = fence;
        return r;
    }
}
