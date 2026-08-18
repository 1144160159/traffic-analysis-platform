package com.traffic.flink.session.sink;

import com.traffic.flink.common.DeploymentActivation;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.configuration.Configuration;
import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.runtime.state.FunctionInitializationContext;
import org.apache.flink.runtime.state.FunctionSnapshotContext;
import org.apache.flink.streaming.api.checkpoint.CheckpointedFunction;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;

import org.apache.http.HttpHost;
import org.apache.http.auth.AuthScope;
import org.apache.http.auth.UsernamePasswordCredentials;
import org.apache.http.client.CredentialsProvider;
import org.apache.http.impl.client.BasicCredentialsProvider;
import org.opensearch.action.bulk.BulkItemResponse;
import org.opensearch.action.bulk.BulkRequest;
import org.opensearch.action.bulk.BulkResponse;
import org.opensearch.action.index.IndexRequest;
import org.opensearch.client.RequestOptions;
import org.opensearch.client.RestClient;
import org.opensearch.client.RestClientBuilder;
import org.opensearch.client.RestHighLevelClient;
import org.opensearch.common.xcontent.XContentType;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * OpenSearch Sink Function
 * 
 * 核心功能：
 * 1. 批量写入 OpenSearch（Bulk API）
 * 2. 幂等写入（使用 event_id 作为文档 ID）
 * 3. 任一 bulk item 失败时使 task 失败，由 checkpoint 重放；event_id 保证幂等
 * 4. 索引字段精简（只索引用于检索的关键字段）
 */
public class OpenSearchSinkFunction extends RichSinkFunction<SessionEvent>
        implements CheckpointedFunction {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(OpenSearchSinkFunction.class);

    // ==================== 配置 ====================
    private final String[] hosts;
    private final int port;
    private final String scheme;
    private final String indexName;
    private final String user;
    private final String password;
    private final int batchSize;
    private final long batchIntervalMs;
    private final DeploymentActivation activation;

    // ==================== 运行时资源 ====================
    private transient RestHighLevelClient client;
    private transient List<SessionEvent> buffer;
    private transient ListState<SessionEvent> pendingState;
    private transient long lastFlushTime;

    // ==================== Metrics ====================
    private transient Counter indexSuccessCounter;
    private transient Counter indexFailCounter;
    private transient Counter bulkFlushCounter;

    public OpenSearchSinkFunction(
            String[] hosts,
            int port,
            String scheme,
            String indexName,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs) {
        this(hosts, port, scheme, indexName, user, password, batchSize, batchIntervalMs,
                DeploymentActivation.legacy("flink-session-job"));
    }

    public OpenSearchSinkFunction(
            String[] hosts,
            int port,
            String scheme,
            String indexName,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs,
            DeploymentActivation activation) {
        this.hosts = hosts;
        this.port = port;
        this.scheme = scheme;
        this.indexName = indexName;
        this.user = user;
        this.password = password;
        this.batchSize = batchSize;
        this.batchIntervalMs = batchIntervalMs;
        this.activation = java.util.Objects.requireNonNull(activation, "activation");
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 创建 OpenSearch 客户端
        HttpHost[] httpHosts = new HttpHost[hosts.length];
        for (int i = 0; i < hosts.length; i++) {
            httpHosts[i] = new HttpHost(hosts[i], port, scheme);
        }

        RestClientBuilder builder = RestClient.builder(httpHosts);

        // 配置认证
        if (user != null && !user.isEmpty()) {
            CredentialsProvider credentialsProvider = new BasicCredentialsProvider();
            credentialsProvider.setCredentials(
                    AuthScope.ANY,
                    new UsernamePasswordCredentials(user, password)
            );

            builder.setHttpClientConfigCallback(httpClientBuilder ->
                    httpClientBuilder.setDefaultCredentialsProvider(credentialsProvider)
            );
        }

        // 配置超时
        builder.setRequestConfigCallback(requestConfigBuilder ->
                requestConfigBuilder
                        .setConnectTimeout(5000)
                        .setSocketTimeout(60000)
                        .setConnectionRequestTimeout(5000)
        );

        this.client = new RestHighLevelClient(builder);
        if (this.buffer == null) {
            this.buffer = new ArrayList<>();
        }
        this.lastFlushTime = System.currentTimeMillis();

        // 初始化 Metrics
        MetricGroup metricGroup = getRuntimeContext().getMetricGroup()
                .addGroup("opensearch_sink");

        this.indexSuccessCounter = metricGroup.counter("index_success_total");
        this.indexFailCounter = metricGroup.counter("index_fail_total");
        this.bulkFlushCounter = metricGroup.counter("bulk_flush_total");

        LOG.info("OpenSearchSinkFunction initialized: hosts={}, index={}, batchSize={}",
                String.join(",", hosts), indexName, batchSize);
    }

    @Override
    public void close() throws Exception {
        // Flush 剩余数据
        if (activation.externalWritesEnabled() && buffer != null && !buffer.isEmpty()) {
            flushBuffer();
        }

        // 关闭客户端
        if (client != null) {
            try {
                client.close();
            } catch (IOException e) {
                LOG.error("Error closing OpenSearch client: {}", e.getMessage());
            }
        }

        super.close();
    }

    @Override
    public void invoke(SessionEvent session, Context context) throws Exception {
        if (!activation.externalWritesEnabled()) return;
        buffer.add(session);

        // 检查是否需要 Flush
        long now = System.currentTimeMillis();
        boolean shouldFlush = buffer.size() >= batchSize ||
                (now - lastFlushTime) >= batchIntervalMs;

        if (shouldFlush) {
            flushBuffer();
        }
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        // A checkpoint cannot commit source offsets while OpenSearch still has
        // unacknowledged documents. Deterministic event_id makes replay safe.
        if (activation.externalWritesEnabled()) flushBuffer();
        pendingState.clear();
        for (SessionEvent session : buffer) {
            pendingState.add(session);
        }
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        pendingState = context.getOperatorStateStore().getListState(
                new ListStateDescriptor<>("opensearch-session-pending-v1", SessionEvent.class));
        buffer = new ArrayList<>();
        if (context.isRestored()) {
            for (SessionEvent session : pendingState.get()) {
                buffer.add(session);
            }
        }
    }

    /**
     * Flush 缓冲区到 OpenSearch
     */
    private void flushBuffer() throws Exception {
        if (!activation.externalWritesEnabled()) {
            return;
        }
        if (buffer.isEmpty()) {
            return;
        }

        List<SessionEvent> toFlush = new ArrayList<>(buffer);
        lastFlushTime = System.currentTimeMillis();

        BulkRequest bulkRequest = new BulkRequest();

        for (SessionEvent session : toFlush) {
            Map<String, Object> document = buildDocument(session);
            String docId = session.getHeader().getEventId();

            IndexRequest indexRequest = new IndexRequest(indexName)
                    .id(docId)
                    .source(document, XContentType.JSON);

            bulkRequest.add(indexRequest);
        }

        final BulkResponse bulkResponse;
        try {
            bulkResponse = client.bulk(bulkRequest, RequestOptions.DEFAULT);
        } catch (Exception e) {
            indexFailCounter.inc(toFlush.size());
            LOG.error("OpenSearch bulk request failed; retaining {} documents for checkpoint replay",
                    toFlush.size(), e);
            throw e;
        }
        bulkFlushCounter.inc();

        BulkItemResponse[] responseItems = bulkResponse.getItems();
        try {
            validateBulkReceiptCount(toFlush.size(), responseItems);
        } catch (IOException incompleteReceipt) {
            indexFailCounter.inc(toFlush.size());
            throw incompleteReceipt;
        }

        int successCount = 0;
        int failCount = 0;
        for (BulkItemResponse itemResponse : responseItems) {
            if (itemResponse.isFailed()) {
                failCount++;
                LOG.warn("Failed to index document {}: {}",
                        itemResponse.getId(),
                        itemResponse.getFailureMessage());
            } else {
                successCount++;
            }
        }

        indexSuccessCounter.inc(successCount);
        indexFailCounter.inc(failCount);
        String failureSummary = bulkFailureSummary(responseItems);
        if (!failureSummary.isEmpty()) {
            LOG.error("OpenSearch bulk partially failed ({}/{}); retaining the batch and failing the task: {}",
                    failCount, toFlush.size(), failureSummary);
            throw new IOException("OpenSearch bulk partial failure: " + failureSummary);
        }

        // Clear only after every item is acknowledged. A later checkpoint
        // replay is safe because event_id is the deterministic document ID.
        buffer.clear();
        LOG.debug("Bulk index completed: {} documents indexed successfully", successCount);
    }

    static String bulkFailureSummary(BulkItemResponse[] items) {
        StringBuilder summary = new StringBuilder();
        for (BulkItemResponse item : items) {
            if (!item.isFailed()) {
                continue;
            }
            if (summary.length() > 0) {
                summary.append("; ");
            }
            summary.append(item.getId()).append(": ").append(item.getFailureMessage());
        }
        return summary.toString();
    }

    static void validateBulkReceiptCount(int expected, BulkItemResponse[] items) throws IOException {
        if (items == null || items.length != expected) {
            throw new IOException("OpenSearch returned an incomplete bulk receipt: expected="
                    + expected + ", actual=" + (items == null ? "null" : items.length));
        }
    }

    /**
     * 构建 OpenSearch 文档
     * 只索引用于检索的关键字段，减少存储开销
     */
    private Map<String, Object> buildDocument(SessionEvent session) {
        Map<String, Object> doc = new HashMap<>();

        // 标识字段
        doc.put("tenant_id", session.getHeader().getTenantId());
        doc.put("run_id", session.getHeader().getRunId());
        doc.put("feature_set_id", session.getHeader().getFeatureSetId());
        doc.put("event_id", session.getHeader().getEventId());
        doc.put("session_id", session.getSessionId());
        doc.put("community_id", session.getCommunityId());

        // 时间字段
        doc.put("ts_start", session.getTsStart());
        doc.put("ts_end", session.getTsEnd());
        doc.put("duration_ms", session.getDurationMs());
        doc.put("ingest_ts", session.getHeader().getIngestTs());

        // 网络五元组
        doc.put("protocol", session.getProtocol());
        doc.put("client_ip", session.getClientIp());
        doc.put("server_ip", session.getServerIp());
        doc.put("client_port", session.getClientPort());
        doc.put("server_port", session.getServerPort());

        // 流量统计
        doc.put("packets_total", session.getPacketsTotal());
        doc.put("bytes_total", session.getBytesTotal());
        doc.put("bytes_up", session.getBytesFwd());
        doc.put("bytes_down", session.getBytesBwd());
        doc.put("up_down_ratio", session.getUpDownRatio());

        // 包长统计
        doc.put("avg_payload", session.getAvgPayload());
        doc.put("std_payload", session.getStdPayload());

        // IAT 统计
        doc.put("mean_iat_ms", session.getMeanIatMs());
        doc.put("std_iat_ms", session.getStdIatMs());

        // TCP 标志
        doc.put("has_syn", session.getHasSyn());
        doc.put("has_fin", session.getHasFin());
        doc.put("has_rst", session.getHasRst());
        doc.put("is_established", session.getIsEstablished());

        // 协议统计
        doc.put("dns_pkt_cnt", session.getDnsPktCnt());
        doc.put("tcp_pkt_cnt", session.getTcpPktCnt());
        doc.put("udp_pkt_cnt", session.getUdpPktCnt());
        doc.put("icmp_pkt_cnt", session.getIcmpPktCnt());

        // 其他
        doc.put("end_reason", session.getEndReason());
        doc.put("evidence_count", session.getEvidenceCount());
        doc.put("flow_count", session.getFlowIdsCount());

        return doc;
    }
}
