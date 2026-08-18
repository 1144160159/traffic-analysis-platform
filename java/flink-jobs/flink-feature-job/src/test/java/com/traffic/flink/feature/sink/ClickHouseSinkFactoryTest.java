package com.traffic.flink.feature.sink;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureCategory;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FeatureFingerprint;
import com.traffic.proto.traffic.v1.FeatureSeq;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;
import org.junit.jupiter.api.Test;

import java.sql.PreparedStatement;
import java.sql.Types;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;

class ClickHouseSinkFactoryTest {

    @Test
    void insertSqlHasExactM03ColumnCount() {
        String sql = ClickHouseSinkFactory.buildInsertSql("feature_stat_local");
        assertEquals(41, sql.chars().filter(ch -> ch == '?').count());
        assertTrue(sql.contains("event_schema_version"));
        assertTrue(sql.contains("source_watermark_ms"));
        assertTrue(sql.contains("availability"));
        assertTrue(sql.contains("missing_reason"));
    }

    @Test
    void contractFieldsAreBoundAndUnknownWatermarkRemainsNull() throws Exception {
        PreparedStatement statement = mock(PreparedStatement.class);
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("feature-event-1")
                        .setSchemaVersion("v1.0")
                        .setAggregateVersion(3)
                        .setIngestTs(2000))
                .setSchemaVersion("v2.0")
                .setObjectType("session")
                .setObjectId("session-1")
                .setTs(1500)
                .setEventTimeStartMs(1000)
                .setEventTimeEndMs(1500)
                .addSourceEventIds("session-event-1")
                .addEvidenceIds("flow-1")
                .setFeatureCategory(FeatureCategory.FEATURE_CATEGORY_FLOW_METADATA)
                .setAvailability(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE)
                .setAlgorithmVersion("feature-stat-flow-metadata-v1")
                .setWindowId("session-1")
                .setValueUnit("mixed")
                .addMissingFields("active_mean_ms")
                .setMissingReason("source did not measure field")
                .build();

        new ClickHouseSinkFactory.FeatureStatementBuilder().accept(statement, feature);

        verify(statement).setString(26, "v1.0");
        verify(statement).setLong(27, 3L);
        verify(statement).setLong(28, 1000L);
        verify(statement).setLong(29, 1500L);
        verify(statement).setNull(30, Types.BIGINT);
        verify(statement).setString(33, "FEATURE_CATEGORY_FLOW_METADATA");
        verify(statement).setString(34, "FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE");
        verify(statement).setInt(38, 1);
    }

    @Test
    void sequenceSqlAndBinderHaveExactContractShape() throws Exception {
        String sql = ClickHouseSinkFactory.buildSequenceInsertSql("feature_seq");
        assertEquals(31, sql.chars().filter(ch -> ch == '?').count());
        assertTrue(sql.contains("algorithm_version"));
        assertTrue(sql.contains("missing_reason"));

        PreparedStatement statement = mock(PreparedStatement.class);
        FeatureSeq sequence = FeatureSeq.newBuilder()
                .setHeader(EventHeader.newBuilder().setTenantId("t").setIngestTs(5))
                .setTsStart(1).setTsEnd(2)
                .setAvailability(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE)
                .setAlgorithmVersion("seq/v1")
                .setMissingReason("partial")
                .build();
        new ClickHouseSinkFactory.FeatureSequenceStatementBuilder().accept(statement, sequence);
        verify(statement).setString(23, "FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE");
        verify(statement).setString(25, "seq/v1");
        verify(statement).setString(30, "partial");
    }

    @Test
    void fingerprintSqlAndBinderHaveExactContractShape() throws Exception {
        String sql = ClickHouseSinkFactory.buildFingerprintInsertSql("feature_fp");
        assertEquals(35, sql.chars().filter(ch -> ch == '?').count());
        assertTrue(sql.contains("ja4"));
        assertTrue(sql.contains("transport_security"));
        assertTrue(sql.contains("raw_traffic_ref"));

        PreparedStatement statement = mock(PreparedStatement.class);
        FeatureFingerprint fingerprint = FeatureFingerprint.newBuilder()
                .setHeader(EventHeader.newBuilder().setTenantId("t").setIngestTs(5))
                .setTs(2)
                .setJa4("ja4")
                .setSni("example.com")
                .setTransportSecurity(TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS)
                .setRawTrafficRef("pcap://object")
                .setAvailability(FeatureAvailability.FEATURE_AVAILABILITY_AVAILABLE)
                .build();
        new ClickHouseSinkFactory.FeatureFingerprintStatementBuilder().accept(statement, fingerprint);
        verify(statement).setString(11, "ja4");
        verify(statement).setString(12, "example.com");
        verify(statement).setString(18, "TRANSPORT_SECURITY_PROTOCOL_TLS");
        verify(statement).setString(19, "pcap://object");
        verify(statement).setString(25, "FEATURE_AVAILABILITY_AVAILABLE");
    }
}
