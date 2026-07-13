package com.traffic.flink.behavior.model;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;
import java.util.HashMap;
import java.util.Map;
/**
 * 数据外泄检测模型
 * 
 * 检测类型：
 * 1. 大量上传 (large_upload) - 异常大量数据上传
 * 2. 快速外泄 (rapid_exfil) - 短时间内快速数据传输
 * 3. 隐蔽外泄 (covert_exfil) - 低速持续数据外泄
 * 4. DNS 外泄 (dns_exfil) - 通过 DNS 通道外泄数据
 * 5. ICMP 外泄 (icmp_exfil) - 通过 ICMP 通道外泄数据
 * 
 * 特征分析：
 * - 上下行比
 * - 传输速率
 * - 持续时间
 * - 协议特征
 */
public class DataExfilDetectionModel extends AbstractBehaviorModel {
    private static final long serialVersionUID = 1L;
    // 协议号
    private static final int PROTOCOL_ICMP = 1;
    private static final int PROTOCOL_TCP = 6;
    private static final int PROTOCOL_UDP = 17;
    // 阈值
    private static final float HIGH_UP_DOWN_RATIO = 5.0f;
    private static final float VERY_HIGH_UP_DOWN_RATIO = 10.0f;
    private static final float HIGH_BPS = 10_000_000.0f; // 10 Mbps
    private static final float VERY_HIGH_BPS = 100_000_000.0f; // 100 Mbps
    private static final long SHORT_DURATION_MS = 60_000; // 1 分钟
    private static final long LONG_DURATION_MS = 300_000; // 5 分钟
    public DataExfilDetectionModel(BehaviorJobConfig config) {
        super(config, "data_exfil", config.getModelVersion(), config.getDataExfilThreshold(),
              "large_upload", "rapid_exfil", "covert_exfil", "dns_exfil", "icmp_exfil", "normal");
    }
    @Override
    public String getDescription() {
        return "Data exfiltration detection model using traffic pattern analysis";
    }
    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Data exfiltration detection model initialized with threshold: {}", threshold);
    }
    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();
        Map<String, Float> scores = new HashMap<>();
        // 1. 大量上传检测
        scores.put("large_upload", calculateLargeUploadScore(feature));
        // 2. 快速外泄检测
        scores.put("rapid_exfil", calculateRapidExfilScore(feature));
        // 3. 隐蔽外泄检测
        scores.put("covert_exfil", calculateCovertExfilScore(feature));
        // 4. DNS 外泄检测
        scores.put("dns_exfil", calculateDnsExfilScore(feature));
        // 5. ICMP 外泄检测
        scores.put("icmp_exfil", calculateIcmpExfilScore(feature));
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
        builder.addFeatureImportance("up_down_ratio", feature.getUpDownRatio());
        builder.addFeatureImportance("bps", feature.getBps());
        builder.addFeatureImportance("duration_ms", (float) feature.getDurationMs());
        builder.addFeatureImportance("protocol", (float) feature.getProtocol());
        return builder.build();
    }
    /**
     * 计算大量上传分数
     * 特征：极高上下行比 + 高吞吐
     */
    private float calculateLargeUploadScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 极高上下行比（上传远大于下载）
        if (feature.getUpDownRatio() > VERY_HIGH_UP_DOWN_RATIO) {
            float ratioScore = Math.min(feature.getUpDownRatio() / (VERY_HIGH_UP_DOWN_RATIO * 5), 1.0f);
            score += ratioScore;
            factors++;
        } else if (feature.getUpDownRatio() > HIGH_UP_DOWN_RATIO) {
            float ratioScore = (feature.getUpDownRatio() - HIGH_UP_DOWN_RATIO) / 
                               (VERY_HIGH_UP_DOWN_RATIO - HIGH_UP_DOWN_RATIO) * 0.5f;
            score += ratioScore;
            factors++;
        }
        // 高吞吐量
        if (feature.getBps() > VERY_HIGH_BPS) {
            score += 1.0f;
            factors++;
        } else if (feature.getBps() > HIGH_BPS) {
            float bpsScore = (feature.getBps() - HIGH_BPS) / (VERY_HIGH_BPS - HIGH_BPS);
            score += bpsScore;
            factors++;
        }
        // 持续时间适中（不是瞬时也不是太长）
        if (feature.getDurationMs() > 10000 && feature.getDurationMs() < LONG_DURATION_MS) {
            score += 0.3f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算快速外泄分数
     * 特征：短时间 + 高速 + 高上下行比
     */
    private float calculateRapidExfilScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 短持续时间
        if (feature.getDurationMs() < SHORT_DURATION_MS && feature.getDurationMs() > 1000) {
            float durationScore = 1.0f - (float) feature.getDurationMs() / SHORT_DURATION_MS;
            score += durationScore;
            factors++;
        }
        // 高吞吐量
        if (feature.getBps() > HIGH_BPS) {
            float bpsScore = Math.min(feature.getBps() / VERY_HIGH_BPS, 1.0f);
            score += bpsScore;
            factors++;
        }
        // 高上下行比
        if (feature.getUpDownRatio() > HIGH_UP_DOWN_RATIO) {
            float ratioScore = Math.min(feature.getUpDownRatio() / VERY_HIGH_UP_DOWN_RATIO, 1.0f);
            score += ratioScore;
            factors++;
        }
        // 低 IAT（快速发送）
        if (feature.getIatMeanMs() > 0 && feature.getIatMeanMs() < 10) {
            score += 0.5f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算隐蔽外泄分数
     * 特征：长时间 + 低速 + 规律性
     */
    private float calculateCovertExfilScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;
        // 长持续时间
        if (feature.getDurationMs() > LONG_DURATION_MS) {
            float durationScore = Math.min((float) feature.getDurationMs() / (LONG_DURATION_MS * 5), 1.0f);
            score += durationScore * 0.4f;
            factors++;
        }
        // 中等上下行比（不太明显但持续上传）
        if (feature.getUpDownRatio() > 2.0f && feature.getUpDownRatio() < HIGH_UP_DOWN_RATIO) {
            score += 0.3f;
            factors++;
        }
        // 规律性通信（低 IAT 标准差）
        if (feature.getIatMeanMs() > 0) {
            float cv = feature.getIatStdMs() / feature.getIatMeanMs();
            if (cv < 0.3f && cv > 0) {
                score += 0.5f;
                factors++;
            }
        }
        // 中等吞吐量（避免触发告警）
        if (feature.getBps() > 100000 && feature.getBps() < HIGH_BPS) {
            score += 0.2f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算 DNS 外泄分数
     * 特征：UDP 协议 + 大包 + 高上下行比
     */
    private float calculateDnsExfilScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_UDP) {
            return 0.0f;
        }
        float score = 0.0f;
        int factors = 0;
        // 大包（DNS 查询通常较小，大包可能是隧道）
        if (feature.getPktlenMean() > 200) {
            float pktScore = Math.min((feature.getPktlenMean() - 200) / 300.0f, 1.0f);
            score += pktScore;
            factors++;
        }
        // 高上下行比（上传数据）
        if (feature.getUpDownRatio() > 2.0f) {
            float ratioScore = Math.min(feature.getUpDownRatio() / 10.0f, 1.0f);
            score += ratioScore;
            factors++;
        }
        // 高频率请求
        if (feature.getPps() > 10) {
            float ppsScore = Math.min(feature.getPps() / 100.0f, 1.0f);
            score += ppsScore;
            factors++;
        }
        // 长持续时间
        if (feature.getDurationMs() > 60000) {
            score += 0.3f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算 ICMP 外泄分数
     * 特征：ICMP 协议 + 大载荷 + 规律性
     */
    private float calculateIcmpExfilScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_ICMP) {
            return 0.0f;
        }
        float score = 0.0f;
        int factors = 0;
        // 大包（正常 ICMP 很小）
        if (feature.getPktlenMean() > 100) {
            float pktScore = Math.min((feature.getPktlenMean() - 100) / 400.0f, 1.0f);
            score += pktScore;
            factors++;
        }
        // 高频率
        if (feature.getPps() > 5) {
            float ppsScore = Math.min(feature.getPps() / 50.0f, 1.0f);
            score += ppsScore;
            factors++;
        }
        // 长持续时间
        if (feature.getDurationMs() > 60000) {
            float durationScore = Math.min(feature.getDurationMs() / 300000.0f, 1.0f);
            score += durationScore;
            factors++;
        }
        // 规律性（低 IAT 标准差）
        if (feature.getIatMeanMs() > 0 && feature.getIatStdMs() / feature.getIatMeanMs() < 0.5f) {
            score += 0.4f;
            factors++;
        }
        // 高上下行比
        if (feature.getUpDownRatio() > 3.0f) {
            score += 0.3f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}