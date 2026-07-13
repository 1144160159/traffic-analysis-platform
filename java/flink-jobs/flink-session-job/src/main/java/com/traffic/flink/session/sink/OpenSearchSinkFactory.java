package com.traffic.flink.session.sink;

import com.traffic.flink.session.SessionJobConfig;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * OpenSearch Sink 工厂类
 * 
 * 提供 Session 索引写入能力：
 * 1. 批量写入 OpenSearch
 * 2. 幂等写入（event_id 作为文档 ID）
 * 3. 精简字段（仅索引检索所需字段）
 */
public class OpenSearchSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(OpenSearchSinkFactory.class);

    private OpenSearchSinkFactory() {}

    /**
     * 创建 OpenSearch Sink
     * 
     * @param config 作业配置
     * @return OpenSearch SinkFunction
     */
    public static SinkFunction<SessionEvent> createSink(SessionJobConfig config) {
        LOG.info("Creating OpenSearch Sink: hosts={}, index={}, batchSize={}",
                String.join(",", config.getOpenSearchHosts()),
                config.getOpenSearchIndex(),
                config.getOpenSearchBatchSize());

        return new OpenSearchSinkFunction(
                config.getOpenSearchHosts(),
                config.getOpenSearchPort(),
                config.getOpenSearchScheme(),
                config.getOpenSearchIndex(),
                config.getOpenSearchUser(),
                config.getOpenSearchPassword(),
                config.getOpenSearchBatchSize(),
                config.getOpenSearchBatchIntervalMs()
        );
    }

    /**
     * 创建 OpenSearch Sink（简化版）
     */
    public static SinkFunction<SessionEvent> createSink(
            String[] hosts,
            int port,
            String scheme,
            String indexName,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs) {

        LOG.info("Creating OpenSearch Sink: hosts={}, index={}",
                String.join(",", hosts), indexName);

        return new OpenSearchSinkFunction(
                hosts,
                port,
                scheme,
                indexName,
                user,
                password,
                batchSize,
                batchIntervalMs
        );
    }

    /**
     * 创建 OpenSearch Sink（使用默认参数）
     */
    public static SinkFunction<SessionEvent> createSink(
            String host,
            String indexName) {

        return new OpenSearchSinkFunction(
                new String[]{host},
                9200,
                "http",
                indexName,
                null,
                null,
                1000,
                5000
        );
    }
}