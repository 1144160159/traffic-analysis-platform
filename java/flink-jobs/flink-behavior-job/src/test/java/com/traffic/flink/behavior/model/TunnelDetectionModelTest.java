////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/model/TunnelDetectionModelTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Nested;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 隧道检测模型单元测试
 */
@DisplayName("TunnelDetectionModel Tests")
class TunnelDetectionModelTest {

    private TunnelDetectionModel model;
    private BehaviorJobConfig config;

    @BeforeEach
    void setUp() throws Exception {
        config = new BehaviorJobConfig.Builder()
                .tunnelThreshold(0.75f)
                .modelVersion("v1.0-test")
                .build();
        model = new TunnelDetectionModel(config);
        model.initialize();
    }

    @Test
    @DisplayName("模型初始化成功")
    void testInitialization() {
        assertTrue(model.isReady());
        assertEquals("tunnel", model.getName());
        assertTrue(model.getSupportedLabels().contains("dns_tunnel"));
        assertTrue(model.getSupportedLabels().contains("icmp_tunnel"));
        assertTrue(model.getSupportedLabels().contains("http_tunnel"));
    }

    @Nested
    @DisplayName("DNS 隧道检测")
    class DnsTunnelTests {

        @Test
        @DisplayName("检测 DNS 隧道 - 大包 + 高频")
        void testDnsTunnelLargePackets() {
            FeatureStat feature = createFeature()
                    .setProtocol(17)            // UDP
                    .setPktlenMean(300.0f)      // 大 DNS 包
                    .setPps(20.0f)              // 高频
                    .setUpDownRatio(2.0f)       // 异常比例
                    .setDurationMs(60000)       // 长持续时间
                    .setIatStdMs(50.0f)         // 规律性
                    .setIatMeanMs(200.0f)
                    .build();

            ModelInferenceResult result = model.infer(feature);

            assertNotNull(result);
            float dnsScore = result.getScoreForLabel("dns_tunnel");
            assertTrue(dnsScore > 0.3f, "Should detect DNS tunnel characteristics");
        }

        @Test
        @DisplayName("正常 DNS 流量不触发检测")
        void testNormalDnsTraffic() {
            FeatureStat feature = createFeature()
                    .setProtocol(17)            // UDP
                    .setPktlenMean(80.0f)       // 正常 DNS 包大小
                    .setPps(2.0f)               // 正常频率
                    .setUpDownRatio(1.0f)
                    .setDurationMs(5000)
                    .build();

            ModelInferenceResult result = model.infer(feature);

            float dnsScore = result.getScoreForLabel("dns_tunnel");
            assertTrue(dnsScore < 0.5f, "Normal DNS should have low tunnel score");
        }
    }

    @Nested
    @DisplayName("ICMP 隧道检测")
    class IcmpTunnelTests {

        @Test
        @DisplayName("检测 ICMP 隧道 - 大载荷 + 规律性")
        void testIcmpTunnelDetection() {
            FeatureStat feature = createFeature()
                    .setProtocol(1)             // ICMP
                    .setPktlenMean(500.0f)      // 大载荷
                    .setPps(10.0f)              // 高频
                    .setDurationMs(120000)      // 长持续时间
                    .setIatMeanMs(1000.0f)
                    .setIatStdMs(100.0f)        // 低变异系数 = 规律性
                    .build();

            ModelInferenceResult result = model.infer(feature);

            float icmpScore = result.getScoreForLabel("icmp_tunnel");
            assertTrue(icmpScore > 0.3f, "Should detect ICMP tunnel");
        }

        @Test
        @DisplayName("正常 ICMP（ping）不触发检测")
        void testNormalIcmpPing() {
            FeatureStat feature = createFeature()
                    .setProtocol(1)             // ICMP
                    .setPktlenMean(64.0f)       // 正常 ping 包大小
                    .setPps(1.0f)               // 正常频率
                    .setDurationMs(5000)
                    .build();

            ModelInferenceResult result = model.infer(feature);

            float icmpScore = result.getScoreForLabel("icmp_tunnel");
            assertTrue(icmpScore < 0.5f, "Normal ICMP should have low tunnel score");
        }
    }

    @Nested
    @DisplayName("HTTP 隧道检测")
    class HttpTunnelTests {

        @Test
        @DisplayName("检测 HTTP 隧道 - 长连接 + 持续传输")
        void testHttpTunnelDetection() {
            FeatureStat feature = createFeature()
                    .setProtocol(6)             // TCP
                    .setDurationMs(600000)      // 10 分钟长连接
                    .setBps(500000.0f)          // 高吞吐
                    .setIatStdMs(5.0f)          // 低变异 = 规律性
                    .setIatMeanMs(100.0f)
                    .setUpDownRatio(1.2f)       // 双向通信
                    .build();

            ModelInferenceResult result = model.infer(feature);

            float httpScore = result.getScoreForLabel("http_tunnel");
            assertTrue(httpScore > 0.3f, "Should detect HTTP tunnel");
        }
    }

    @Test
    @DisplayName("非 TCP/UDP/ICMP 协议返回低分")
    void testUnsupportedProtocol() {
        FeatureStat feature = createFeature()
                .setProtocol(47)  // GRE
                .build();

        ModelInferenceResult result = model.infer(feature);

        // 应该检测到 covert_channel 或返回低分
        assertNotNull(result);
        assertFalse(result.hasError());
    }

    private FeatureStat.Builder createFeature() {
        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("test-tenant")
                        .setEventId("test-event-id")
                        .build())
                .setObjectType("flow")
                .setObjectId("test-object-id")
                .setCommunityId("1:abc123")
                .setTs(System.currentTimeMillis())
                .setProtocol(6)
                .setDurationMs(10000)
                .setPps(10.0f)
                .setBps(100000.0f)
                .setUpDownRatio(1.0f)
                .setPktlenMean(200.0f)
                .setPktlenStd(50.0f)
                .setIatMeanMs(100.0f)
                .setIatStdMs(50.0f)
                .setActiveMeanMs(100.0f)
                .setIdleMeanMs(50.0f);
    }
}