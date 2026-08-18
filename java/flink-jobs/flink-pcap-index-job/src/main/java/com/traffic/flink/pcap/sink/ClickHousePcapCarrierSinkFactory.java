package com.traffic.flink.pcap.sink;

import com.traffic.flink.pcap.process.PcapManifestPolicy;
import com.traffic.flink.pcap.process.PcapManifestValidation;
import com.traffic.flink.pcap.process.PcapManifestValidator;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.connector.jdbc.JdbcStatementBuilder;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.util.List;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

/** Manifest-v2 PCAP carrier sink; the old payload-only sink remains available for rollback. */
public final class ClickHousePcapCarrierSinkFactory {
    private ClickHousePcapCarrierSinkFactory() { }

    public static String buildInsertSql(String table, PcapProjectionColumns columns) {
        if (!("pcap_index_v2".equals(table) || "traffic.pcap_index_v2".equals(table))) {
            throw new IllegalArgumentException("unapproved PCAP table identifier");
        }
        if (columns == null) throw new IllegalArgumentException("PCAP projection columns are required");
        columns.validate();
        String placeholders = IntStream.range(0, columns.size()).mapToObj(ignored -> "?")
                .collect(Collectors.joining(", "));
        return "INSERT INTO " + table + " (" + String.join(", ", columns.ordered()) +
                ") VALUES (" + placeholders + ")";
    }

    public static SinkFunction<PcapIndexedRecord> createPcapIndexSink(
            PcapClickHouseConfig config, PcapProjectionColumns columns) {
        if (config == null || columns == null) {
            throw new IllegalArgumentException("ClickHouse PCAP config and columns are required");
        }
        config.validate(columns);
        return JdbcSink.sink(
                buildInsertSql(config.getTable(), columns),
                new PcapIndexedRecordStatementBuilder(columns),
                JdbcExecutionOptions.builder()
                        .withBatchSize(config.getBatchSize())
                        .withBatchIntervalMs(config.getBatchIntervalMs())
                        .withMaxRetries(config.getMaxRetries())
                        .build(),
                new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                        .withUrl(config.getJdbcUrl())
                        .withDriverName("com.clickhouse.jdbc.ClickHouseDriver")
                        .withUsername(config.getUsername())
                        .withPassword(config.getPassword())
                        .withConnectionCheckTimeoutSeconds(30)
                        .build());
    }

    public static final class PcapIndexedRecordStatementBuilder
            implements JdbcStatementBuilder<PcapIndexedRecord> {
        private static final long serialVersionUID = 1L;
        private final PcapProjectionColumns columns;

        public PcapIndexedRecordStatementBuilder(PcapProjectionColumns columns) {
            if (columns == null) throw new IllegalArgumentException("PCAP columns are required");
            columns.validate();
            this.columns = columns;
        }

        @Override
        public void accept(PreparedStatement ps, PcapIndexedRecord record) throws SQLException {
            if (ps == null || record == null) throw new SQLException("PCAP statement and carrier are required");
            PcapManifestValidation validation = PcapManifestValidator.validate(
                    record, PcapManifestPolicy.strictV2());
            if (!validation.isAccepted()) {
                throw new SQLException("PCAP manifest rejected: " + String.join(",", validation.getReasons()));
            }
            PcapIndexMeta meta = record.getMeta();
            int index = 1;
            ps.setString(index++, meta.getTenantId());
            ps.setString(index++, meta.getProbeId());
            ps.setString(index++, meta.getFileKey());
            ps.setString(index++, meta.getBucket());
            ps.setString(index++, meta.getObjectVersion());
            ps.setString(index++, meta.getEtag());
            ps.setLong(index++, meta.getOriginalSize());
            ps.setLong(index++, meta.getStoredSize());
            ps.setString(index++, meta.getCompression());
            ps.setInt(index++, meta.getManifestVersion());
            ps.setString(index++, record.getTopic());
            ps.setInt(index++, record.getPartition());
            ps.setLong(index++, record.getOffset());
            ps.setString(index++, record.getKafkaKeySha256());
            ps.setString(index++, record.getKafkaHeadersSha256());
            ps.setString(index++, record.getRawSha256());
            ps.setString(index++, record.getProjectionIdentity());
            ps.setLong(index++, meta.getTsStart());
            ps.setLong(index++, meta.getTsEnd());
            ps.setLong(index++, meta.getByteSize());
            ps.setInt(index++, meta.getZstdLevel());
            ps.setString(index++, meta.getSha256());
            ps.setString(index++, meta.getCommunityId());
            ps.setString(index++, meta.getFlowId());
            ps.setLong(index++, meta.getOffsetStart());
            ps.setLong(index++, meta.getOffsetEnd());
            ps.setString(index++, meta.getBloomFilterB64());
            List<String> communityIds = meta.getCommunityIdsList();
            ps.setObject(index++, communityIds.toArray(new String[0]));
            ps.setLong(index++, meta.getCreatedTs());
            if (index - 1 != columns.size()) {
                throw new SQLException("PCAP binder parameter count differs from ordered column contract");
            }
        }
    }
}
