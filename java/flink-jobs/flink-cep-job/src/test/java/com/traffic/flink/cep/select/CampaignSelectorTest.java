package com.traffic.flink.cep.select;

import com.traffic.flink.cep.model.CampaignType;
import com.traffic.flink.cep.patterns.PatternConfig;
import com.traffic.flink.cep.patterns.ScanExploitPattern;
import com.traffic.flink.cep.patterns.BruteForcePattern;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.Campaign;
import com.traffic.proto.traffic.v1.Severity;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.cep.CEP;
import org.apache.flink.cep.PatternStream;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.KeyedStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.functions.source.RichParallelSourceFunction;
import org.apache.flink.streaming.api.watermark.Watermark;

import org.junit.jupiter.api.*;
import static org.assertj.core.api.Assertions.assertThat;

import java.util.*;

/**
 * Campaign Selector 测试 — MiniCluster + SourceFunction + Watermark.
 */
class CampaignSelectorTest {

    private PatternConfig patternConfig;

    @BeforeEach
    void setUp() {
        patternConfig = new PatternConfig();
        patternConfig.setScanExploitWindowMinutes(30);
        patternConfig.setBruteForceWindowMinutes(10);
        patternConfig.setMinFailedAttempts(3);
    }

    @Test @DisplayName("ScanExploitSelector")
    void testScanExploitSelector() throws Exception {
        long baseTime = System.currentTimeMillis();
        // Need >=3 scans (MIN_SCAN_TARGETS=3) + 1 exploit targeting one of the scanned IPs
        List<Alert> alerts = Arrays.asList(
                createAlert("s1", "tenant-1", "192.168.1.100", "10.0.0.1", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 120000),
                createAlert("s2", "tenant-1", "192.168.1.100", "10.0.0.2", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 90000),
                createAlert("s3", "tenant-1", "192.168.1.100", "10.0.0.3", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 60000),
                createAlert("e1", "tenant-1", "192.168.1.100", "10.0.0.1", "EXPLOIT", Severity.SEVERITY_CRITICAL, baseTime));

        List<Campaign> results = runCepPipeline(alerts, ScanExploitPattern.create(patternConfig), new ScanExploitSelector());
        assertThat(results).hasSize(1);
        Campaign c = results.get(0);
        assertThat(c.getTenantId()).isEqualTo("tenant-1");
        assertThat(c.getCampaignType()).isEqualTo(CampaignType.SCAN_EXPLOIT.getCode());
    }

    @Test @DisplayName("BruteForceSelector")
    void testBruteForceSelector() throws Exception {
        long baseTime = System.currentTimeMillis();
        List<Alert> alerts = new ArrayList<>();
        // Same timestamps as CepJobIntegrationTest (which works)
        for (int i = 0; i < 5; i++)
            alerts.add(createAlert("af-" + i, "tenant-1", "192.168.1.100", "10.0.0.1", "AUTH_FAILED", Severity.SEVERITY_MEDIUM, baseTime - 300000 + i * 30000));
        alerts.add(createAlert("as", "tenant-1", "192.168.1.100", "10.0.0.1", "AUTH_SUCCESS", Severity.SEVERITY_HIGH, baseTime));

        List<Campaign> results = runCepPipeline(alerts, BruteForcePattern.create(patternConfig), new BruteForceSelector());
        // MiniCluster CEP may produce multiple overlapping matches
        assertThat(results).isNotEmpty();
        Campaign c = results.get(0);
        assertThat(c.getTenantId()).isEqualTo("tenant-1");
        assertThat(c.getCampaignType()).isEqualTo(CampaignType.BRUTE_FORCE.getCode());
    }

    @Test @DisplayName("无匹配不生成 Campaign")
    void testNoMatchingPattern() throws Exception {
        long baseTime = System.currentTimeMillis();
        List<Alert> alerts = Arrays.asList(
                createAlert("a1", "tenant-1", "192.168.1.100", "10.0.0.1", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 60000),
                createAlert("a2", "tenant-1", "192.168.1.100", "10.0.0.2", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime));
        List<Campaign> results = runCepPipeline(alerts, ScanExploitPattern.create(patternConfig), new ScanExploitSelector());
        assertThat(results).isEmpty();
    }

    // Helper: create scan-exploit test data with >=3 scans (MIN_SCAN_TARGETS)
    private List<Alert> scanExploitAlerts(long baseTime, String exploitDst) {
        return Arrays.asList(
                createAlert("s1", "tenant-1", "192.168.1.100", "10.0.0.1", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 120000),
                createAlert("s2", "tenant-1", "192.168.1.100", "10.0.0.2", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 90000),
                createAlert("s3", "tenant-1", "192.168.1.100", "10.0.0.3", "PORT_SCAN", Severity.SEVERITY_MEDIUM, baseTime - 60000),
                createAlert("e1", "tenant-1", "192.168.1.100", exploitDst, "EXPLOIT", Severity.SEVERITY_CRITICAL, baseTime));
    }

    @Test @DisplayName("实体格式一致性")
    void testEntityFormatConsistency() throws Exception {
        long baseTime = System.currentTimeMillis();
        List<Alert> alerts = scanExploitAlerts(baseTime, "10.0.0.1");
        List<Campaign> results = runCepPipeline(alerts, ScanExploitPattern.create(patternConfig), new ScanExploitSelector());
        assertThat(results).isNotEmpty();
        for (String entity : results.get(0).getEntitiesList())
            assertThat(entity).startsWith("ip:");
    }

    @Test @DisplayName("Campaign ID 格式")
    void testCampaignIdFormat() throws Exception {
        long baseTime = System.currentTimeMillis();
        List<Alert> alerts = scanExploitAlerts(baseTime, "10.0.0.1");
        List<Campaign> results = runCepPipeline(alerts, ScanExploitPattern.create(patternConfig), new ScanExploitSelector());
        assertThat(results).isNotEmpty();
        assertThat(results.get(0).getCampaignId()).startsWith("campaign-").contains("tenant-1");
    }

    @Test @DisplayName("EventHeader 正确设置")
    void testEventHeaderCorrectlySet() throws Exception {
        long baseTime = System.currentTimeMillis();
        List<Alert> alerts = scanExploitAlerts(baseTime, "10.0.0.1");
        List<Campaign> results = runCepPipeline(alerts, ScanExploitPattern.create(patternConfig), new ScanExploitSelector());
        assertThat(results).isNotEmpty();
        Campaign c = results.get(0);
        assertThat(c.getHeader()).isNotNull();
        assertThat(c.getHeader().getTenantId()).isEqualTo("tenant-1");
        assertThat(c.getHeader().getEventId()).isNotEmpty();
    }

    // ==================== MiniCluster Pipeline ====================

    @SuppressWarnings("unchecked")
    private <T> List<Campaign> runCepPipeline(
            List<Alert> alerts,
            org.apache.flink.cep.pattern.Pattern<Alert, ?> pattern,
            org.apache.flink.cep.functions.PatternProcessFunction<Alert, Campaign> selector) throws Exception {

        // Sort and get max timestamp for final watermark
        List<Alert> sorted = new ArrayList<>(alerts);
        sorted.sort(Comparator.comparingLong(Alert::getFirstSeen));
        long maxTs = sorted.stream().mapToLong(Alert::getLastSeen).max().orElse(0);
        long finalWm = maxTs + 60 * 60 * 1000; // 1 hour past last event

        StreamExecutionEnvironment env = StreamExecutionEnvironment.createLocalEnvironment();
        env.setParallelism(1);
        env.getConfig().setAutoWatermarkInterval(100);

        // Add sentinel with far-future timestamp to advance watermark past within() window
        sorted.add(createAlert("_s", "_s_", "255.255.255.255", "0.0.0.0", "PING", Severity.SEVERITY_INFO, finalWm));
        sorted.sort(Comparator.comparingLong(Alert::getFirstSeen));

        DataStream<Alert> input = env.addSource(new AlertSource(sorted, finalWm + 60000))
                .returns(TypeInformation.of(Alert.class))
                .assignTimestampsAndWatermarks(
                        WatermarkStrategy.<Alert>forMonotonousTimestamps()
                                .withTimestampAssigner((a, ts) -> a.getLastSeen()));
        KeyedStream<Alert, String> keyed = input.keyBy(Alert::getSrcIp);
        PatternStream<Alert> ps = CEP.pattern(keyed, pattern);
        DataStream<Campaign> campaigns = ps.process(selector);

        List<Campaign> results = new ArrayList<>();
        campaigns.executeAndCollect().forEachRemaining(results::add);
        return results;
    }

    private static Alert createAlert(String id, String tenant, String src, String dst,
            String type, Severity sev, long ts) {
        return Alert.newBuilder().setAlertId(id).setTenantId(tenant).setSrcIp(src).setDstIp(dst)
                .setDstPort(22).setProtocol(6).setAlertType(type).setSeverity(sev)
                .setFirstSeen(ts).setLastSeen(ts).setScore(0.8f)
                .setRuleVersion("r1").setModelVersion("m1").build();
    }

    /** Serializable SourceFunction: emits Alerts then final Watermark. */
    public static class AlertSource extends RichParallelSourceFunction<Alert> {
        private static final long serialVersionUID = 1L;
        private final List<Alert> events;
        private final long finalWatermarkTs;

        public AlertSource(List<Alert> events, long finalWatermarkTs) {
            this.events = events; this.finalWatermarkTs = finalWatermarkTs;
        }

        @Override
        public void run(SourceContext<Alert> ctx) throws Exception {
            long maxTs = 0;
            for (Alert e : events) {
                synchronized (ctx.getCheckpointLock()) {
                    ctx.collectWithTimestamp(e, e.getLastSeen());
                }
                maxTs = Math.max(maxTs, e.getLastSeen());
            }
            synchronized (ctx.getCheckpointLock()) {
                ctx.emitWatermark(new Watermark(Math.max(finalWatermarkTs, maxTs + 1)));
            }
            Thread.sleep(200);
        }

        @Override public void cancel() {}
    }
}
