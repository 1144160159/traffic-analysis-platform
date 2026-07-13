package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.HashMap;
import java.util.Map;

/**
 * 扫描检测模型
 * 
 * 检测类型：
 * 1. 端口扫描 (port_scan)
 * 2. 网络扫描 (network_scan)
 * 3. 服务探测 (service_probe)
 * 4. 垂直扫描 (vertical_scan)
 * 5. 水平扫描 (horizontal_scan)
 * 
 * 特征分析：
 * - 高 PPS + 短持续时间 = 扫描特征
 * - 低包长均值 = SYN 扫描特征
 * - 高 SYN 标志比例 = SYN 扫描
 * - 高 RST 比例 = 端口关闭/过滤
 */
public class ScanDetectionModel extends AbstractBehaviorModel {

    private static final long serialVersionUID = 1L;

    // 归一化参数（训练集统计值）
    private static final float[] FEATURE_MEANS = {
        6.0f,      // protocol
        5000.0f,   // duration_ms
        100.0f,    // pps
        1000000.0f, // bps
        1.0f,      // up_down_ratio
        100.0f,    // pktlen_mean
        50.0f,     // pktlen_std
        10.0f,     // iat_mean_ms
        5.0f,      // iat_std_ms
        100.0f,    // active_mean_ms
        100.0f,    // idle_mean_ms
        1.0f,      // tcp_flag_syn_cnt
        10.0f,     // tcp_flag_ack_cnt
        65535.0f,  // tcp_init_win_bytes_fwd
        65535.0f,  // tcp_init_win_bytes_bwd
        0.0f,      // extra[0]
        0.0f       // extra[1]
    };

    private static final float[] FEATURE_STDS = {
        3.0f,      // protocol
        10000.0f,  // duration_ms
        500.0f,    // pps
        5000000.0f, // bps
        2.0f,      // up_down_ratio
        200.0f,    // pktlen_mean
        100.0f,    // pktlen_std
        50.0f,     // iat_mean_ms
        25.0f,     // iat_std_ms
        500.0f,    // active_mean_ms
        500.0f,    // idle_mean_ms
        5.0f,      // tcp_flag_syn_cnt
        50.0f,     // tcp_flag_ack_cnt
        30000.0f,  // tcp_init_win_bytes_fwd
        30000.0f,  // tcp_init_win_bytes_bwd
        1.0f,      // extra[0]
        1.0f       // extra[1]
    };

    // 扫描检测阈值
    private static final float HIGH_PPS_THRESHOLD = 1000.0f;
    private static final float SHORT_DURATION_THRESHOLD = 1000.0f; // 1秒
    private static final float LOW_PKT_SIZE_THRESHOLD = 100.0f;
    private static final float HIGH_SYN_RATIO_THRESHOLD = 0.8f;
    private static final float LOW_IAT_THRESHOLD = 5.0f; // 5ms

    public ScanDetectionModel(BehaviorJobConfig config) {
        super(config, "scan", config.getModelVersion(), config.getScanThreshold(),
              "port_scan", "network_scan", "service_probe", "vertical_scan", "horizontal_scan", "normal");
    }

    @Override
    public String getDescription() {
        return "Port and network scan detection model using statistical features";
    }

    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Scan detection model initialized with threshold: {}", threshold);
    }

    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();

        // 归一化特征
        float[] normalized = normalizeFeatures(processedFeatures, FEATURE_MEANS, FEATURE_STDS);

        // 计算各类扫描的分数
        Map<String, Float> scanScores = new HashMap<>();

        // 1. 端口扫描检测
        float portScanScore = calculatePortScanScore(feature, normalized);
        scanScores.put("port_scan", portScanScore);

        // 2. 网络扫描检测
        float networkScanScore = calculateNetworkScanScore(feature, normalized);
        scanScores.put("network_scan", networkScanScore);

        // 3. 服务探测检测
        float serviceProbeScore = calculateServiceProbeScore(feature, normalized);
        scanScores.put("service_probe", serviceProbeScore);

        // 4. 垂直扫描检测
        float verticalScanScore = calculateVerticalScanScore(feature, normalized);
        scanScores.put("vertical_scan", verticalScanScore);

        // 5. 水平扫描检测
        float horizontalScanScore = calculateHorizontalScanScore(feature, normalized);
        scanScores.put("horizontal_scan", horizontalScanScore);

        // 找到最高分和对应标签
        String topLabel = "normal";
        float topScore = 0.0f;
        boolean detected = false;

        for (Map.Entry<String, Float> entry : scanScores.entrySet()) {
            if (entry.getValue() > topScore) {
                topScore = entry.getValue();
                topLabel = entry.getKey();
            }
        }

        // 检查是否超过阈值
        if (topScore >= threshold) {
            detected = true;
        } else {
            topLabel = "normal";
            topScore = 1.0f - topScore;
        }

        // 计算推理时间
        long inferenceTimeMs = (System.nanoTime() - startTime) / 1_000_000;

        // 构建结果
        ModelInferenceResult.Builder builder = ModelInferenceResult.success(name, version)
                .topLabel(topLabel)
                .topScore(topScore)
                .detected(detected)
                .inferenceTimeMs(inferenceTimeMs);

        // 添加所有标签分数
        for (Map.Entry<String, Float> entry : scanScores.entrySet()) {
            builder.addLabel(entry.getKey(), entry.getValue());
        }

        // 添加特征重要性（用于可解释性）
        builder.addFeatureImportance("pps", feature.getPps());
        builder.addFeatureImportance("duration_ms", (float) feature.getDurationMs());
        builder.addFeatureImportance("pktlen_mean", feature.getPktlenMean());
        builder.addFeatureImportance("tcp_flag_syn_cnt", (float) feature.getTcpFlagSynCnt());

        return builder.build();
    }

    /**
     * 计算端口扫描分数
     * 特征：高 PPS + 小包 + 多 SYN
     */
    private float calculatePortScanScore(FeatureStat feature, float[] normalized) {
        float score = 0.0f;
        int factors = 0;

        // 高 PPS
        if (feature.getPps() > HIGH_PPS_THRESHOLD) {
            score += Math.min(feature.getPps() / (HIGH_PPS_THRESHOLD * 10), 1.0f);
            factors++;
        }

        // 短持续时间
        if (feature.getDurationMs() < SHORT_DURATION_THRESHOLD) {
            score += (SHORT_DURATION_THRESHOLD - feature.getDurationMs()) / SHORT_DURATION_THRESHOLD;
            factors++;
        }

        // 小包
        if (feature.getPktlenMean() < LOW_PKT_SIZE_THRESHOLD) {
            score += (LOW_PKT_SIZE_THRESHOLD - feature.getPktlenMean()) / LOW_PKT_SIZE_THRESHOLD;
            factors++;
        }

        // 高 SYN 比例
        int totalFlags = feature.getTcpFlagSynCnt() + feature.getTcpFlagAckCnt();
        if (totalFlags > 0) {
            float synRatio = (float) feature.getTcpFlagSynCnt() / totalFlags;
            if (synRatio > HIGH_SYN_RATIO_THRESHOLD) {
                score += synRatio;
                factors++;
            }
        }

        // 低 IAT（快速发包）
        if (feature.getIatMeanMs() > 0 && feature.getIatMeanMs() < LOW_IAT_THRESHOLD) {
            score += (LOW_IAT_THRESHOLD - feature.getIatMeanMs()) / LOW_IAT_THRESHOLD;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算网络扫描分数
     * 特征：低上下行比 + 多 SYN
     */
    private float calculateNetworkScanScore(FeatureStat feature, float[] normalized) {
        float score = 0.0f;
        int factors = 0;

        // 高上下行比（发送多于接收）
        if (feature.getUpDownRatio() > 2.0f) {
            score += Math.min(feature.getUpDownRatio() / 10.0f, 1.0f);
            factors++;
        }

        // 高 PPS
        if (feature.getPps() > HIGH_PPS_THRESHOLD / 2) {
            score += Math.min(feature.getPps() / (HIGH_PPS_THRESHOLD * 5), 1.0f);
            factors++;
        }

        // 小包
        if (feature.getPktlenMean() < LOW_PKT_SIZE_THRESHOLD * 1.5f) {
            score += 0.5f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算服务探测分数
     * 特征：特定端口探测模式
     */
    private float calculateServiceProbeScore(FeatureStat feature, float[] normalized) {
        float score = 0.0f;
        int factors = 0;

        // 中等 PPS（比扫描慢，但比正常流量快）
        if (feature.getPps() > 10.0f && feature.getPps() < HIGH_PPS_THRESHOLD) {
            score += 0.5f;
            factors++;
        }

        // 中等包长（包含探测载荷）
        if (feature.getPktlenMean() > 50.0f && feature.getPktlenMean() < 500.0f) {
            score += 0.5f;
            factors++;
        }

        // 短持续时间
        if (feature.getDurationMs() < SHORT_DURATION_THRESHOLD * 5) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算垂直扫描分数
     * 特征：对单个目标的多端口扫描
     */
    private float calculateVerticalScanScore(FeatureStat feature, float[] normalized) {
        float score = 0.0f;
        int factors = 0;

        // 非常高的 PPS
        if (feature.getPps() > HIGH_PPS_THRESHOLD * 2) {
            score += Math.min(feature.getPps() / (HIGH_PPS_THRESHOLD * 20), 1.0f);
            factors++;
        }

        // 非常低的 IAT
        if (feature.getIatMeanMs() > 0 && feature.getIatMeanMs() < LOW_IAT_THRESHOLD / 2) {
            score += 1.0f;
            factors++;
        }

        // 小包 + 高 SYN
        if (feature.getPktlenMean() < LOW_PKT_SIZE_THRESHOLD && 
            feature.getTcpFlagSynCnt() > feature.getTcpFlagAckCnt()) {
            score += 0.8f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算水平扫描分数
     * 特征：对多个目标的同端口扫描
     */
    private float calculateHorizontalScanScore(FeatureStat feature, float[] normalized) {
        float score = 0.0f;
        int factors = 0;

        // 高 PPS
        if (feature.getPps() > HIGH_PPS_THRESHOLD) {
            score += Math.min(feature.getPps() / (HIGH_PPS_THRESHOLD * 10), 1.0f);
            factors++;
        }

        // 极高上下行比（几乎只发不收）
        if (feature.getUpDownRatio() > 5.0f) {
            score += Math.min(feature.getUpDownRatio() / 20.0f, 1.0f);
            factors++;
        }

        // TCP 协议
        if (feature.getProtocol() == 6) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}