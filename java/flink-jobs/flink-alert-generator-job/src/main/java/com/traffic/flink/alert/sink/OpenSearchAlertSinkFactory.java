package com.traffic.flink.alert.sink;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.traffic.proto.traffic.v1.Alert;

import org.apache.flink.api.common.functions.RuntimeContext;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;
import org.apache.flink.streaming.connectors.elasticsearch.ElasticsearchSinkFunction;
import org.apache.flink.streaming.connectors.elasticsearch.RequestIndexer;
import org.apache.flink.streaming.connectors.elasticsearch7.ElasticsearchSink;
import org.apache.http.HttpHost;
import org.apache.http.auth.AuthScope;
import org.apache.http.auth.UsernamePasswordCredentials;
import org.apache.http.client.CredentialsProvider;
import org.apache.http.impl.client.BasicCredentialsProvider;
import org.apache.http.impl.nio.client.HttpAsyncClientBuilder;
import org.elasticsearch.action.index.IndexRequest;
import org.elasticsearch.client.Requests;
import org.elasticsearch.common.xcontent.XContentType;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.net.URL;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * OpenSearch Alert Sink 工厂 (修复版)
 * 
 * 修复内容：
 * - 补充缺失字段 (evidence_ids, state_version)
 * - 优化时间戳格式 (ISO 8601)
 * - 与 ClickHouse 字段保持一致
 * - 添加错误处理和重试配置
 */
public class OpenSearchAlertSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(OpenSearchAlertSinkFactory.class);
    
    private static final ObjectMapper MAPPER = new ObjectMapper()
            .configure(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS, false);

    // ISO 8601 时间格式
    private static final DateTimeFormatter ISO_FORMATTER = DateTimeFormatter
            .ofPattern("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'")
            .withZone(ZoneOffset.UTC);

    /**
     * 创建 Alert OpenSearch Sink
     * 
     * @param urlString OpenSearch URL
     * @param index 索引名称
     * @param user 用户名
     * @param password 密码
     * @return SinkFunction 实例
     */
    public static SinkFunction<Alert> createAlertSink(
            String urlString,
            String index,
            String user,
            String password
    ) {
        LOG.info("Creating OpenSearch alert sink: {} -> {}", urlString, index);

        try {
            URL url = new URL(urlString);
            HttpHost host = new HttpHost(
                    url.getHost(),
                    url.getPort(),
                    url.getProtocol()
            );

            List<HttpHost> httpHosts = new ArrayList<>();
            httpHosts.add(host);

            ElasticsearchSink.Builder<Alert> builder = new ElasticsearchSink.Builder<>(
                    httpHosts,
                    new AlertElasticsearchSinkFunction(index)
            );

            // ==================== 批量配置 ====================
            
            // 最大批量操作数
            builder.setBulkFlushMaxActions(1000);
            
            // 最大批量大小 (MB)
            builder.setBulkFlushMaxSizeMb(5);
            
            // 批量刷新间隔 (ms)
            builder.setBulkFlushInterval(5000);
            
            // 背压刷新（当缓冲区满时阻塞）
            builder.setBulkFlushBackoff(true);
            builder.setBulkFlushBackoffType(ElasticsearchSink.FlushBackoffType.EXPONENTIAL);
            builder.setBulkFlushBackoffRetries(3);
            builder.setBulkFlushBackoffDelay(1000);

            // ==================== 认证配置 ====================
            
            final String finalUser = user;
            final String finalPassword = password;
            
            builder.setRestClientFactory(restClientBuilder -> {
                restClientBuilder.setHttpClientConfigCallback(
                        new org.elasticsearch.client.RestClientBuilder.HttpClientConfigCallback() {
                            @Override
                            public HttpAsyncClientBuilder customizeHttpClient(
                                    HttpAsyncClientBuilder httpClientBuilder
                            ) {
                                CredentialsProvider credentialsProvider = new BasicCredentialsProvider();
                                credentialsProvider.setCredentials(
                                        AuthScope.ANY,
                                        new UsernamePasswordCredentials(finalUser, finalPassword)
                                );
                                
                                return httpClientBuilder
                                        .setDefaultCredentialsProvider(credentialsProvider)
                                        .setMaxConnTotal(100)
                                        .setMaxConnPerRoute(50);
                            }
                        }
                );
                
                // 连接超时
                restClientBuilder.setRequestConfigCallback(requestConfigBuilder ->
                        requestConfigBuilder
                                .setConnectTimeout(5000)
                                .setSocketTimeout(60000)
                                .setConnectionRequestTimeout(5000)
                );
            });

            // ==================== 失败处理 ====================
            
            builder.setFailureHandler((action, failure, restStatusCode, indexer) -> {
                LOG.error("OpenSearch bulk action failed: action={}, status={}, error={}",
                        action.getClass().getSimpleName(),
                        restStatusCode,
                        failure.getMessage());
                
                // 可以在这里实现重试逻辑或 DLQ
                if (restStatusCode == -1) {
                    // 网络错误，可重试
                    LOG.warn("Network error, consider retrying: {}", failure.getMessage());
                }
            });

            return builder.build();

        } catch (Exception e) {
            LOG.error("Failed to create OpenSearch sink", e);
            throw new RuntimeException("Failed to create OpenSearch sink", e);
        }
    }

    /**
     * Alert 到 Elasticsearch 的转换函数
     */
    private static class AlertElasticsearchSinkFunction implements ElasticsearchSinkFunction<Alert> {

        private static final long serialVersionUID = 1L;
        private final String index;

        public AlertElasticsearchSinkFunction(String index) {
            this.index = index;
        }

        @Override
        public void process(Alert alert, RuntimeContext ctx, RequestIndexer indexer) {
            try {
                Map<String, Object> doc = convertAlertToMap(alert);
                String json = MAPPER.writeValueAsString(doc);

                // 使用 alert_id 作为文档 ID（幂等写入）
                IndexRequest request = Requests.indexRequest()
                        .index(index)
                        .id(alert.getAlertId())
                        .source(json, XContentType.JSON);

                indexer.add(request);

            } catch (Exception e) {
                LOG.error("Failed to process alert for OpenSearch: alertId={}, error={}",
                        alert.getAlertId(), e.getMessage(), e);
            }
        }

        /**
         * 将 Alert 转换为 Map（用于 JSON 序列化）
         * 
         * 与 ClickHouse 字段保持一致
         */
        private Map<String, Object> convertAlertToMap(Alert alert) {
            Map<String, Object> doc = new HashMap<>();

            // ==================== 租户与标识 ====================
            
            doc.put("tenant_id", alert.getTenantId());
            doc.put("alert_id", alert.getAlertId());
            doc.put("event_id", alert.getEventId());

            // ==================== 网络五元组 ====================
            
            doc.put("src_ip", alert.getSrcIp());
            doc.put("dst_ip", alert.getDstIp());
            doc.put("src_port", alert.getSrcPort());
            doc.put("dst_port", alert.getDstPort());
            doc.put("protocol", alert.getProtocol());
            doc.put("protocol_name", alert.getProtocolName());

            // ==================== 告警分类 ====================
            
            doc.put("alert_type", alert.getAlertType());
            doc.put("severity", alert.getSeverity().name());
            doc.put("score", alert.getScore());
            doc.put("labels", alert.getLabelsList());

            // ==================== 时间信息（ISO 8601 格式）====================
            
            doc.put("first_seen", formatTimestamp(alert.getFirstSeen()));
            doc.put("last_seen", formatTimestamp(alert.getLastSeen()));
            doc.put("updated_ts", formatTimestamp(alert.getUpdatedTs()));
            doc.put("ingest_ts", formatTimestamp(alert.getIngestTs()));
            
            // 同时保留毫秒时间戳（便于数值范围查询）
            doc.put("first_seen_ms", alert.getFirstSeen());
            doc.put("last_seen_ms", alert.getLastSeen());

            // ==================== 状态信息 ====================
            
            doc.put("status", alert.getStatus().name());
            doc.put("assignee", alert.getAssignee());
            doc.put("count", alert.getCount());

            // ==================== 关联信息 ====================
            
            doc.put("community_id", alert.getCommunityId());
            doc.put("session_id", alert.getSessionId());
            doc.put("campaign_id", alert.getCampaignId());

            // ==================== 版本信息 ====================
            
            doc.put("model_version", alert.getModelVersion());
            doc.put("rule_version", alert.getRuleVersion());
            doc.put("feature_set_id", alert.getFeatureSetId());

            // ==================== 证据与去重 ====================
            
            // 修复：添加 evidence_ids 字段
            doc.put("evidence_ids", alert.getEvidenceIdsList());
            doc.put("dedup_fingerprint", alert.getDedupFingerprint());

            // ==================== Arkime 与反馈 ====================
            
            doc.put("arkime_session_link", alert.getArkimeSessionLink());
            doc.put("feedback_label", alert.getFeedbackLabel());
            doc.put("feedback_count", alert.getFeedbackCount());

            // ==================== 状态版本（乐观锁）====================
            
            // 修复：添加 state_version 字段
            doc.put("state_version", alert.getStateVersion());

            return doc;
        }

        /**
         * 将毫秒时间戳格式化为 ISO 8601 字符串
         */
        private String formatTimestamp(long epochMillis) {
            if (epochMillis <= 0) {
                return null;
            }
            return ISO_FORMATTER.format(Instant.ofEpochMilli(epochMillis));
        }
    }
}