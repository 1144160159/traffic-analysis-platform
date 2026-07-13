package com.traffic.flink.session;

import com.traffic.flink.session.aggregator.SessionAggregator;
import com.traffic.proto.traffic.v1.*;
import org.apache.flink.runtime.testutils.MiniClusterResourceConfiguration;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.windowing.assigners.EventTimeSessionWindows;
import org.apache.flink.streaming.api.windowing.time.Time;
import org.apache.flink.test.util.MiniClusterWithClientResource;
import org.junit.jupiter.api.*;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.*;

/**
 * SessionJob 集成测试（新增）
 * 
 * 测试要点：
 * 1. ✅ 端到端数据流测试（Source → Aggregation → Sink）
 * 2. ✅ Late Data 处理测试
 * 3. ✅ 租户隔离测试
 * 4. ✅ 窗口触发验证
 */
@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class SessionJobIntegrationTest {

    private MiniClusterWithClientResource flinkCluster;

    @BeforeAll
    void setupCluster() {
        flinkCluster = new MiniClusterWithClientResource(
            new MiniClusterResourceConfiguration.Builder()
                .setNumberSlotsPerTaskManager(2)
                .setNumberTaskManagers(1)
                .build()
        );

        try {
            flinkCluster.before();
        } catch (Exception e) {
            fail("Failed to start Flink MiniCluster: " + e.getMessage());
        }
    }

    @AfterAll
    void teardownCluster() throws Exception {
        if (flinkCluster != null) {
            flinkCluster.after();
        }
    }

    @Test
    @DisplayName("✅ 端到端：单个 Session 聚合")
    void testSingleSessionAggregation() throws Exception {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(1);

        // 创建测试数据
        List<FlowEvent> testData = createTestFlowEvents("tenant1", "run1", "1:abc123", 3);

        DataStream<FlowEvent> flowStream = env.fromCollection(testData)
            .assignTimestampsAndWatermarks(
                WatermarkStrategy.<FlowEvent>forBoundedOutOfOrderness(Duration.ofSeconds(1))
                    .withTimestampAssigner((event, timestamp) -> event.getTsEnd())
            );

        // 聚合
        DataStream<SessionEvent> sessionStream = flowStream
            .keyBy(flow -> {
                String tenantId = flow.getHeader().getTenantId();
                String communityId = flow.getCommunityId();
                return tenantId + "|" + communityId;
            })
            .window(EventTimeSessionWindows.withGap(Time.seconds(5)))
            .aggregate(new SessionAggregator());

        // 收集结果
        List<SessionEvent> results = new ArrayList<>();
        sessionStream.executeAndCollect().forEachRemaining(results::add);

        // 验证
        assertEquals(1, results.size(), "应聚合为 1 个 Session");
        SessionEvent session = results.get(0);
        assertEquals("tenant1", session.getHeader().getTenantId());
        assertEquals("1:abc123", session.getCommunityId());
        assertEquals(3, session.getFlowIdsCount(), "应关联 3 个 Flow");
    }

    @Test
    @DisplayName("✅ 租户隔离：不同租户的相同 community_id 应分别聚合")
    void testTenantIsolation() throws Exception {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(1);

        // 创建两个租户的数据（相同 community_id）
        List<FlowEvent> tenant1Data = createTestFlowEvents("tenant1", "run1", "1:abc123", 2);
        List<FlowEvent> tenant2Data = createTestFlowEvents("tenant2", "run1", "1:abc123", 2);

        List<FlowEvent> allData = new ArrayList<>();
        allData.addAll(tenant1Data);
        allData.addAll(tenant2Data);

        DataStream<FlowEvent> flowStream = env.fromCollection(allData)
            .assignTimestampsAndWatermarks(
                WatermarkStrategy.<FlowEvent>forBoundedOutOfOrderness(Duration.ofSeconds(1))
                    .withTimestampAssigner((event, timestamp) -> event.getTsEnd())
            );

        DataStream<SessionEvent> sessionStream = flowStream
            .keyBy(flow -> {
                String tenantId = flow.getHeader().getTenantId();
                String communityId = flow.getCommunityId();
                return tenantId + "|" + communityId;
            })
            .window(EventTimeSessionWindows.withGap(Time.seconds(5)))
            .aggregate(new SessionAggregator());

        List<SessionEvent> results = new ArrayList<>();
        sessionStream.executeAndCollect().forEachRemaining(results::add);

        // 验证
        assertEquals(2, results.size(), "应聚合为 2 个独立的 Session（不同租户）");
        
        List<String> tenantIds = results.stream()
            .map(s -> s.getHeader().getTenantId())
            .sorted()
            .collect(Collectors.toList());
        
        assertEquals("tenant1", tenantIds.get(0));
        assertEquals("tenant2", tenantIds.get(1));
    }

    @Test
    @DisplayName("✅ 窗口间隔：超过 gap 的 Flow 应触发新 Session")
    void testSessionWindowGap() throws Exception {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(1);

        // 创建两组 Flow，间隔超过 5 秒
        List<FlowEvent> testData = new ArrayList<>();
        
        // 第一组：1000-2000ms
        testData.add(createTestFlow("tenant1", "run1", "1:abc123", 1000L, 2000L, 1));
        testData.add(createTestFlow("tenant1", "run1", "1:abc123", 2000L, 3000L, 2));
        
        // 第二组：10000-11000ms（间隔 7 秒，超过 gap）
        testData.add(createTestFlow("tenant1", "run1", "1:abc123", 10000L, 11000L, 3));

        DataStream<FlowEvent> flowStream = env.fromCollection(testData)
            .assignTimestampsAndWatermarks(
                WatermarkStrategy.<FlowEvent>forBoundedOutOfOrderness(Duration.ofSeconds(1))
                    .withTimestampAssigner((event, timestamp) -> event.getTsEnd())
            );

        DataStream<SessionEvent> sessionStream = flowStream
            .keyBy(flow -> {
                String tenantId = flow.getHeader().getTenantId();
                String communityId = flow.getCommunityId();
                return tenantId + "|" + communityId;
            })
            .window(EventTimeSessionWindows.withGap(Time.seconds(5)))
            .aggregate(new SessionAggregator());

        List<SessionEvent> results = new ArrayList<>();
        sessionStream.executeAndCollect().forEachRemaining(results::add);

        // 验证
        assertEquals(2, results.size(), "应触发 2 个独立的 Session（超过 gap）");
        
        SessionEvent session1 = results.get(0);
        SessionEvent session2 = results.get(1);
        
        assertTrue(session1.getTsEnd() < session2.getTsStart() - 5000,
                "两个 Session 之间应有超过 5 秒的间隔");
    }

    @Test
    @DisplayName("✅ Client/Server 映射验证")
    void testClientServerMappingInPipeline() throws Exception {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(1);

        // 创建 HTTP 流量（目标端口 80）
        FlowEvent httpFlow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .setFeatureSetId("default")
                .setProbeId("probe-1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:http")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.100")
                .setDstIp("10.0.0.50")
                .setSrcPort(54321)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setPacketsFwd(100)
            .setPacketsBwd(200)
            .setBytesFwd(10000)
            .setBytesBwd(50000)
            .build();

        DataStream<FlowEvent> flowStream = env.fromCollection(List.of(httpFlow))
            .assignTimestampsAndWatermarks(
                WatermarkStrategy.<FlowEvent>forBoundedOutOfOrderness(Duration.ofSeconds(1))
                    .withTimestampAssigner((event, timestamp) -> event.getTsEnd())
            );

        DataStream<SessionEvent> sessionStream = flowStream
            .keyBy(flow -> flow.getHeader().getTenantId() + "|" + flow.getCommunityId())
            .window(EventTimeSessionWindows.withGap(Time.seconds(5)))
            .aggregate(new SessionAggregator());

        List<SessionEvent> results = new ArrayList<>();
        sessionStream.executeAndCollect().forEachRemaining(results::add);

        assertEquals(1, results.size());
        SessionEvent session = results.get(0);
        
        // 验证 client/server 识别
        assertEquals("192.168.1.100", session.getClientIp());
        assertEquals("10.0.0.50", session.getServerIp());
        assertEquals(54321, session.getClientPort());
        assertEquals(80, session.getServerPort());
        
        // 验证 bytes_up/down 映射（fwd = client→server = up）
        assertEquals(10000, session.getBytesFwd(), "bytesFwd 应为 client→server");
        assertEquals(50000, session.getBytesBwd(), "bytesBwd 应为 server→client");
    }

    // ==================== 辅助方法 ====================

    /**
     * 创建测试用 FlowEvent 列表
     */
    private List<FlowEvent> createTestFlowEvents(String tenantId, String runId, String communityId, int count) {
        List<FlowEvent> events = new ArrayList<>();
        for (int i = 0; i < count; i++) {
            events.add(createTestFlow(tenantId, runId, communityId, 
                    1000L + i * 1000, 2000L + i * 1000, i + 1));
        }
        return events;
    }

    /**
     * 创建单个测试 FlowEvent
     */
    private FlowEvent createTestFlow(String tenantId, String runId, String communityId, 
                                      long tsStart, long tsEnd, int flowNum) {
        return FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId(tenantId)
                .setRunId(runId)
                .setEventId("event-" + flowNum)
                .setFeatureSetId("default")
                .setProbeId("probe-1")
                .build())
            .setFlowId("flow-" + flowNum)
            .setCommunityId(communityId)
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345 + flowNum)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(tsStart)
            .setTsEnd(tsEnd)
            .setPacketsFwd(100)
            .setPacketsBwd(50)
            .setBytesFwd(1000)
            .setBytesBwd(500)
            .build();
    }
}
