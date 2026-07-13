package com.traffic.flink.behavior.model;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;
import java.util.HashMap;
import java.util.Map;
/**
 * C2 (Command & Control) 通信检测模型
 * 
 * 检测类型：
 * 1. 信标通信 (beacon) - 周期性 C2 心跳
 * 2. 命令接收 (command_recv) - 接收控制命令
 * 3. 数据回传 (data_exfil) - 向 C2 回传数据
 * 4. 恶意软件回调 (malware_callback) - 恶意软件首次回调
 * 5. DGA 通信 (dga_comm) - 使用 DGA 域名的 C2
 * 
 * 特征分析：
 * - 通信间隔规律性（Beacon 特征）
 * - 流量方向性
 * - 连接时长
 * - 包大小分布
 */
public class C2DetectionModel extends AbstractBehaviorModel {
    private static final long serialVersionUID = 1L;
    // C2 通信典型特征阈值
    private static final float BEACON_REGULARITY_THRESHOLD = 0.3f; // IAT 变异系数阈值
    private static final long MIN_BEACON_DURATION = 60000; // 最小 Beacon 持续时间 (1分钟)
    private static final long MAX_BEACON_INTERVAL = 300000; // 最大 Beacon 间隔 (5分钟)
    private static final float LOW_VOLUME_THRESHOLD = 10.0f; // 低流量阈值 (KB/s)
    public C2DetectionModel(BehaviorJobConfig config) {
        super(config, "c2", config.getModelVersion(), config.getC2Threshold(),
              "beacon", "command_recv", "data_exfil", "malware_callback", "dga_comm", "normal");
    }
    @Override
    public String getDescription() {
        return "C2 communication detection model for identifying command and control traffic";
    }
    @Override
    protected void doInitialize() throws Exception {
        LOG.info("C2 detection model initialized with threshold: {}", threshold);
    }
    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();
        Map<String, Float> scores = new HashMap<>();
        // 1. 信标通信检测
        scores.put("beacon", calculateBeaconScore(feature));
        // 2. 命令接收检测
        scores.put("command_recv", calculateCommandRecvScore(feature));
        // 3. 数据回传检测
        scores.put("data_exfil", calculateDataExfilScore(feature));
        // 4. 恶意软件回调检测
        scores.put("malware_callback", calculateMalwareCallbackScore(feature));
        // 5. DGA 通信检测
        scores.put("dga_comm", calculateDgaCommScore(feature));
        // 找到最高分
        String topLabel = "normal";
        float topScore = 0.0f;
        boolean detected = false;
        for (Map.Entry<String, Float> entry : scores.entrySet()) {
            if (entry.getValue() > topScore) {
                topScore = entry.getValue();
                topLabel = entry.getKey();
            }
        }
        if (topScore >= threshold) {
            detected = true;
        } else {
            topLabel = "normal";
            topScore = 1.0f - topScore;
        }
        long inferenceTimeMs = (System.nanoTime() - startTime) / 1_000_000;
        ModelInferenceResult.Builder builder = ModelInferenceResult.success(name, version)
                .topLabel(topLabel)
                .topScore(topScore)
                .detected(detected)
                .inferenceTimeMs(inferenceTimeMs);
        for (Map.Entry<String, Float> entry : scores.entrySet()) {
            builder.addLabel(entry.getKey(), entry.getValue());
        }
        // 添加特征重要性
        builder.addFeatureImportance("iat_mean_ms", feature.getIatMeanMs());
        builder.addFeatureImportance("iat_std_ms", feature.getIatStdMs());
        builder.addFeatureImportance("duration_ms", (float) feature.getDurationMs());
        builder.addFeatureImportance("up_down_ratio", feature.getUpDownRatio());
        // 添加 Beacon 规律性指标
        if (feature.getIatMeanMs() > 0) {
            float regularity = feature.getIatStdMs() / feature.getIatMeanMs();
            builder.addFeatureImportance("beacon_regularity", regularity);
        }
        return builder.build();
    }
    /**
     * 计算信标通信分数
     * 
     * Beacon 特征：
     * - 周期性通信（IAT 变异系数低）
     * - 持续时间长
     * - 低流量
     */
    private float calculateBeaconScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 规律性检测（低 IAT 变异系数）
        if (feature.getIatMeanMs() > 0 && feature.getIatStdMs() >= 0) {
            float cv = feature.getIatStdMs() / feature.getIatMeanMs();
            if (cv < BEACON_REGULARITY_THRESHOLD) {
                score += (BEACON_REGULARITY_THRESHOLD - cv) / BEACON_REGULARITY_THRESHOLD;
                factors++;
            }
        }
        // 长持续时间
        if (feature.getDurationMs() > MIN_BEACON_DURATION) {
            float durationScore = Math.min(feature.getDurationMs() / 600000.0f, 1.0f); // 归一化到 10 分钟
            score += durationScore * 0.5f;
            factors++;
        }
        // 低流量
        float kbps = feature.getBps() / 8000.0f;
        if (kbps < LOW_VOLUME_THRESHOLD && kbps > 0) {
            score += 0.3f;
            factors++;
        }
        // Beacon 间隔在合理范围内
        if (feature.getIatMeanMs() > 1000 && feature.getIatMeanMs() < MAX_BEACON_INTERVAL) {
            score += 0.4f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算命令接收分数
     * 
     * 命令接收特征：
     * - 下行流量大于上行（接收多于发送）
     * - 响应时间短
     * - 小包传输
     */
    private float calculateCommandRecvScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 下行大于上行（upDownRatio < 1）
        if (feature.getUpDownRatio() < 0.5f && feature.getUpDownRatio() > 0) {
            score += (0.5f - feature.getUpDownRatio()) / 0.5f * 0.5f;
            factors++;
        }
        // 小包
        if (feature.getPktlenMean() < 200 && feature.getPktlenMean() > 0) {
            score += 0.3f;
            factors++;
        }
        // 低 PPS 但持续时间长
        if (feature.getPps() < 10 && feature.getDurationMs() > 30000) {
            score += 0.3f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算数据回传分数
     * 
     * 数据回传特征：
     * - 上行流量大于下行
     * - 较大的数据量
     * - 持续传输
     */
    private float calculateDataExfilScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 上行大于下行
        if (feature.getUpDownRatio() > 2.0f) {
            float ratioScore = Math.min((feature.getUpDownRatio() - 2.0f) / 10.0f, 1.0f);
            score += ratioScore * 0.5f;
            factors++;
        }
        // 较大的上行流量
        if (feature.getBps() > 100000) { // > 100 Kbps
            score += 0.3f;
            factors++;
        }
        // 持续传输
        if (feature.getDurationMs() > 10000 && feature.getIdleMeanMs() < feature.getActiveMeanMs()) {
            score += 0.3f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算恶意软件回调分数
     * 
     * 恶意软件回调特征：
     * - 短连接
     * - 特定端口（非标准）
     * - 首次连接后立即传输数据
     */
    private float calculateMalwareCallbackScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 短连接
        if (feature.getDurationMs() > 0 && feature.getDurationMs() < 10000) {
            score += 0.3f;
            factors++;
        }
        // 快速数据交换
        if (feature.getPps() > 5 && feature.getDurationMs() < 5000) {
            score += 0.3f;
            factors++;
        }
        // TCP 连接（有 SYN）
        if (feature.getTcpFlagSynCnt() > 0 && feature.getTcpFlagAckCnt() > 0) {
            score += 0.2f;
            factors++;
        }
        // 小数据量但有双向通信
        if (feature.getUpDownRatio() > 0.5f && feature.getUpDownRatio() < 2.0f) {
            if (feature.getBps() < 50000) { // < 50 Kbps
                score += 0.2f;
                factors++;
            }
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算 DGA 通信分数
     * 
     * DGA 通信特征：
     * - 基于 DNS 特征（这里通过协议判断）
     * - 短连接后长连接
     * - 失败重试模式
     */
    private float calculateDgaCommScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // UDP 协议（DNS）
        if (feature.getProtocol() == 17) {
            // 高频率 DNS 查询
            if (feature.getPps() > 5) {
                score += 0.4f;
                factors++;
            }
            // 短连接
            if (feature.getDurationMs() < 5000) {
                score += 0.2f;
                factors++;
            }
            // 上行大于下行（查询多于响应，可能是 NXDOMAIN）
            if (feature.getUpDownRatio() > 1.5f) {
                score += 0.3f;
                factors++;
            }
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}