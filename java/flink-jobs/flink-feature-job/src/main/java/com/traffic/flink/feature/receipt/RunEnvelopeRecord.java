package com.traffic.flink.feature.receipt;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * run envelope(analysis.envelopes.v1,JSON)输入视图。字段加法式演进。
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public final class RunEnvelopeRecord implements java.io.Serializable {

    private static final long serialVersionUID = 1L;

    @JsonProperty("schema_version")
    private String schemaVersion;

    @JsonProperty("tenant_id")
    private String tenantId;

    @JsonProperty("run_id")
    private String runId;

    @JsonProperty("execution_spec_sha256")
    private String executionSpecSha256;

    @JsonProperty("fencing_token")
    private String fencingToken;

    @JsonProperty("window_end_ms")
    private long windowEndMs;

    @JsonProperty("event")
    private Event event = new Event();

    public String tenantId() { return tenantId; }
    public String runId() { return runId; }
    public String executionSpecSha256() { return executionSpecSha256; }
    public String fencingToken() { return fencingToken; }
    public long windowEndMs() { return windowEndMs; }
    public Event event() { return event; }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public static final class Event implements java.io.Serializable {
        private static final long serialVersionUID = 1L;

        @JsonProperty("community_id")
        private String communityId;

        @JsonProperty("flow_id")
        private String flowId;

        @JsonProperty("ts_start")
        private long tsStart;

        @JsonProperty("ts_end")
        private long tsEnd;

        @JsonProperty("packets_fwd")
        private long packetsFwd;

        @JsonProperty("packets_bwd")
        private long packetsBwd;

        @JsonProperty("bytes_fwd")
        private long bytesFwd;

        @JsonProperty("bytes_bwd")
        private long bytesBwd;

        @JsonProperty("duration_ms")
        private long durationMs;

        // 完整流特征(路由器透传 FlowEvent;特征/识别/检测组件消费)
        @JsonProperty("direction")
        private String direction;

        @JsonProperty("pps")
        private double pps;

        @JsonProperty("bps")
        private double bps;

        @JsonProperty("tcp_flags_fwd")
        private long tcpFlagsFwd;

        @JsonProperty("tcp_flags_bwd")
        private long tcpFlagsBwd;

        @JsonProperty("tos")
        private long tos;

        @JsonProperty("subflow_count")
        private long subflowCount;

        @JsonProperty("tuple")
        private Tuple tuple;

        @JsonProperty("pktlen")
        private Stat pktlen;

        @JsonProperty("iat")
        private Stat iat;

        @JsonProperty("active")
        private Stat active;

        @JsonProperty("idle")
        private Stat idle;

        public String communityId() { return communityId; }
        public String flowId() { return flowId; }
        public long tsStart() { return tsStart; }
        public long tsEnd() { return tsEnd; }
        public long packetsFwd() { return packetsFwd; }
        public long packetsBwd() { return packetsBwd; }
        public long bytesFwd() { return bytesFwd; }
        public long bytesBwd() { return bytesBwd; }
        public long durationMs() { return durationMs; }
        public String direction() { return direction; }
        public double pps() { return pps; }
        public double bps() { return bps; }
        public long tcpFlagsFwd() { return tcpFlagsFwd; }
        public long tcpFlagsBwd() { return tcpFlagsBwd; }
        public long tos() { return tos; }
        public long subflowCount() { return subflowCount; }
        public Tuple tuple() { return tuple; }
        public Stat pktlen() { return pktlen; }
        public Stat iat() { return iat; }
        public Stat active() { return active; }
        public Stat idle() { return idle; }

        public long totalPackets() { return packetsFwd + packetsBwd; }
        public long totalBytes() { return bytesFwd + bytesBwd; }
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public static final class Tuple implements java.io.Serializable {
        private static final long serialVersionUID = 1L;
        @JsonProperty("src_ip") private String srcIp;
        @JsonProperty("dst_ip") private String dstIp;
        @JsonProperty("src_port") private long srcPort;
        @JsonProperty("dst_port") private long dstPort;
        @JsonProperty("protocol") private long protocol;
        public String srcIp() { return srcIp; }
        public String dstIp() { return dstIp; }
        public long srcPort() { return srcPort; }
        public long dstPort() { return dstPort; }
        public long protocol() { return protocol; }
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public static final class Stat implements java.io.Serializable {
        private static final long serialVersionUID = 1L;
        @JsonProperty("min") private double min;
        @JsonProperty("max") private double max;
        @JsonProperty("mean") private double mean;
        @JsonProperty("std") private double std;
        public double min() { return min; }
        public double max() { return max; }
        public double mean() { return mean; }
        public double std() { return std; }
    }
}
