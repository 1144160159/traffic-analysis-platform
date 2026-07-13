package com.traffic.flink.session.sink;

import com.traffic.flink.session.SessionJobConfig;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.streaming.api.datastream.AsyncDataStream;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.concurrent.TimeUnit;

/**
 * ClickHouse 异步 Sink 工厂类
 * 
 * 提供带降级策略的 ClickHouse 写入能力：
 * 1. 异步写入，不阻塞主流
 * 2. 写入失败的数据返回到主流，由调用方写入 DLQ
 * 3. 支持超时控制
 */
public class ClickHouseAsyncSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseAsyncSinkFactory.class);

    private ClickHouseAsyncSinkFactory() {}

    /**
     * 创建异步 ClickHouse Sink 并返回失败数据流
     * 
     * @param inputStream 输入的 SessionEvent 流
     * @param config 作业配置
     * @return 写入失败的 SessionEvent 流（用于写入 DLQ）
     */
    public static DataStream<SessionEvent> addAsyncSink(
            SingleOutputStreamOperator<SessionEvent> inputStream,
            SessionJobConfig config) {

        LOG.info("Creating ClickHouse Async Sink: url={}, table={}, batchSize={}, timeout={}ms",
                config.getClickhouseUrl(),
                config.getClickhouseTable(),
                config.getClickhouseBatchSize(),
                config.getClickhouseTimeoutMs());

        ClickHouseAsyncSinkFunction asyncFunction = new ClickHouseAsyncSinkFunction(
                config.getClickhouseUrl(),
                config.getClickhouseTable(),
                config.getClickhouseUser(),
                config.getClickhousePassword(),
                config.getClickhouseBatchSize(),
                config.getClickhouseBatchIntervalMs(),
                config.getClickhouseMaxRetries(),
                config.getClickhouseThreadPoolSize()
        );

        // 使用 AsyncDataStream 包装
        // 返回的是写入失败的 SessionEvent（成功的会被过滤掉）
        return AsyncDataStream.unorderedWait(
                inputStream,
                asyncFunction,
                config.getClickhouseTimeoutMs(),
                TimeUnit.MILLISECONDS,
                config.getClickhouseAsyncCapacity()
        ).filter(session -> session != null) // 过滤掉成功写入的空结果
         .name("ClickHouse Async Sink (with failover)")
         .uid("clickhouse-async-sink");
    }

    /**
     * 创建简化版异步 Sink（直接使用默认参数）
     */
    public static DataStream<SessionEvent> addAsyncSink(
            SingleOutputStreamOperator<SessionEvent> inputStream,
            String jdbcUrl,
            String table,
            String user,
            String password) {

        ClickHouseAsyncSinkFunction asyncFunction = new ClickHouseAsyncSinkFunction(
                jdbcUrl,
                table,
                user,
                password,
                10000,      // batchSize
                5000,       // batchIntervalMs
                3,          // maxRetries
                4           // threadPoolSize
        );

        return AsyncDataStream.unorderedWait(
                inputStream,
                asyncFunction,
                30000,      // timeout 30s
                TimeUnit.MILLISECONDS,
                100         // capacity
        ).filter(session -> session != null)
         .name("ClickHouse Async Sink")
         .uid("clickhouse-async-sink");
    }
}