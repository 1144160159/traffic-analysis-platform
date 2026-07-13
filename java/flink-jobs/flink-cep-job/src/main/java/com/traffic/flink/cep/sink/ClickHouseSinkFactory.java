package com.traffic.flink.cep.sink;

import com.traffic.proto.traffic.v1.Campaign;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * ClickHouse Campaign Sink 工厂 — 集群模式
 */
public class ClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseSinkFactory.class);

    /**
     * 创建 Campaign Sink (集群模式, 写入 Distributed 表)
     *
     * @param hosts    ClickHouse 端点列表 (逗号分隔)
     * @param database 数据库名称
     * @param table    Distributed 表名 (campaigns)
     */
    public static SinkFunction<Campaign> createCampaignSink(
            String hosts,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = String.format("jdbc:clickhouse://%s/%s", hosts, database);

        LOG.info("Creating ClickHouse CLUSTER campaign sink: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = buildInsertSql(table);

        return JdbcSink.sink(
                insertSql,
                (ps, campaign) -> {
                    int idx = 1;
                    
                    // tenant_id
                    ps.setString(idx++, campaign.getTenantId());
                    // campaign_id
                    ps.setString(idx++, campaign.getCampaignId());
                    
                    // ts_start, ts_end (ClickHouse schema stores epoch millis as Int64)
                    ps.setLong(idx++, campaign.getTsStart());
                    ps.setLong(idx++, campaign.getTsEnd());
                    
                    // entities (Array<String>)
                    if (campaign.getEntitiesCount() > 0) {
                        ps.setObject(idx++, campaign.getEntitiesList().toArray(new String[0]));
                    } else {
                        ps.setObject(idx++, new String[0]);
                    }
                    
                    // alerts (Array<String>)
                    if (campaign.getAlertsCount() > 0) {
                        ps.setObject(idx++, campaign.getAlertsList().toArray(new String[0]));
                    } else {
                        ps.setObject(idx++, new String[0]);
                    }
                    
                    // score
                    ps.setFloat(idx++, campaign.getScore());
                    
                    // summary
                    ps.setString(idx++, campaign.getSummary());
                    
                    // event_id
                    ps.setString(idx++, campaign.getEventId());
                    
                    // ingest_ts (ClickHouse schema stores epoch millis as Int64)
                    ps.setLong(idx++, campaign.getIngestTs());
                    
                    // campaign_type
                    ps.setString(idx++, campaign.getCampaignType());
                    
                    // attack_phases (Array<String>)
                    if (campaign.getAttackPhasesCount() > 0) {
                        ps.setObject(idx++, campaign.getAttackPhasesList().toArray(new String[0]));
                    } else {
                        ps.setObject(idx++, new String[0]);
                    }
                    
                    // rule_ids (Array<String>)
                    if (campaign.getRuleIdsCount() > 0) {
                        ps.setObject(idx++, campaign.getRuleIdsList().toArray(new String[0]));
                    } else {
                        ps.setObject(idx++, new String[0]);
                    }
                    
                    // model_ids (Array<String>)
                    if (campaign.getModelIdsCount() > 0) {
                        ps.setObject(idx++, campaign.getModelIdsList().toArray(new String[0]));
                    } else {
                        ps.setObject(idx++, new String[0]);
                    }
                },
                JdbcExecutionOptions.builder()
                        .withBatchSize(1000)
                        .withBatchIntervalMs(5000)
                        .withMaxRetries(3)
                        .build(),
                new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                        .withUrl(jdbcUrl)
                        .withDriverName("com.clickhouse.jdbc.ClickHouseDriver")
                        .withUsername(user)
                        .withPassword(password)
                        .build()
        );
    }

    private static String buildInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        "tenant_id, campaign_id, " +
                        "ts_start, ts_end, " +
                        "entities, alerts, " +
                        "score, summary, " +
                        "event_id, ingest_ts, " +
                        "campaign_type, attack_phases, rule_ids, model_ids" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );
    }
}
