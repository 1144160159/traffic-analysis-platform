package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.HashMap;
import java.util.Map;

/**
 * DGA (Domain Generation Algorithm) 检测模型
 * 
 * 检测类型：
 * 1. 随机域名 (random_dga) - 纯随机字符组成的域名
 * 2. 字典 DGA (dictionary_dga) - 基于字典词组合的域名
 * 3. 高频 NXDOMAIN (nxdomain_flood) - 大量不存在域名查询
 * 4. C2 域名 (c2_domain) - 命令与控制服务器域名
 * 
 * 注意：完整的 DGA 检测需要域名字符串分析，
 * 这里基于流量特征进行辅助检测
 * 
 * 流量特征分析：
 * - 高频 DNS 查询
 * - 大量失败响应（NXDOMAIN）
 * - 短间隔批量查询
 */
public class DGADetectionModel extends AbstractBehaviorModel {

    private static final long serialVersionUID = 1L;

    // 协议号
    private static final int PROTOCOL_UDP = 17;

    // DGA 检测阈值
    private static final float HIGH_DNS_PPS_THRESHOLD = 20.0f;
    private static final float HIGH_UP_DOWN_RATIO_THRESHOLD = 3.0f;
    private static final float LOW_IAT_THRESHOLD = 50.0f; // 50ms
    private static final float SHORT_DURATION_THRESHOLD = 5000.0f; // 5秒

    public DGADetectionModel(BehaviorJobConfig config) {
        super(config, "dga", config.getModelVersion(), config.getDgaThreshold(),
              "random_dga", "dictionary_dga", "nxdomain_flood", "c2_domain", "normal");
    }

    @Override
    public String getDescription() {
        return "DGA detection model using DNS traffic pattern analysis";
    }

    @Override
    protected void doInitialize() throws Exception {
        LOG.info("DGA detection model initialized with threshold: {}", threshold);
    }

    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();

        // DGA 检测主要针对 DNS 流量（UDP 53）
        // 这里基于流量特征进行辅助检测

        Map<String, Float> dgaScores = new HashMap<>();

        // 1. 随机 DGA 检测
        float randomDgaScore = calculateRandomDgaScore(feature);
        dgaScores.put("random_dga", randomDgaScore);

        // 2. 字典 DGA 检测
        float dictDgaScore = calculateDictDgaScore(feature);
        dgaScores.put("dictionary_dga", dictDgaScore);

        // 3. NXDOMAIN 洪泛检测
        float nxdomainScore = calculateNxdomainFloodScore(feature);
        dgaScores.put("nxdomain_flood", nxdomainScore);

        // 4. C2 域名检测
        float c2DomainScore = calculateC2DomainScore(feature);
        dgaScores.put("c2_domain", c2DomainScore);

        // 找到最高分和对应标签
        String topLabel = "normal";
        float topScore = 0.0f;
        boolean detected = false;

        for (Map.Entry<String, Float> entry : dgaScores.entrySet()) {
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
        for (Map.Entry<String, Float> entry : dgaScores.entrySet()) {
            builder.addLabel(entry.getKey(), entry.getValue());
        }

        // 添加特征重要性
        builder.addFeatureImportance("protocol", (float) feature.getProtocol());
        builder.addFeatureImportance("pps", feature.getPps());
        builder.addFeatureImportance("up_down_ratio", feature.getUpDownRatio());
        builder.addFeatureImportance("iat_mean_ms", feature.getIatMeanMs());

        return builder.build();
    }

    /**
     * 计算随机 DGA 分数
     * 特征：高频 DNS 查询、短间隔、高失败率
     */
    private float calculateRandomDgaScore(FeatureStat feature) {
        // 主要针对 UDP 流量
        if (feature.getProtocol() != PROTOCOL_UDP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 高频查询
        if (feature.getPps() > HIGH_DNS_PPS_THRESHOLD) {
            float ppsScore = Math.min(feature.getPps() / (HIGH_DNS_PPS_THRESHOLD * 5), 1.0f);
            score += ppsScore;
            factors++;
        }

        // 极低 IAT（批量快速查询）
        if (feature.getIatMeanMs() > 0 && feature.getIatMeanMs() < LOW_IAT_THRESHOLD) {
            float iatScore = (LOW_IAT_THRESHOLD - feature.getIatMeanMs()) / LOW_IAT_THRESHOLD;
            score += iatScore;
            factors++;
        }

        // 高上下行比（发送多于接收，可能是 NXDOMAIN）
        if (feature.getUpDownRatio() > HIGH_UP_DOWN_RATIO_THRESHOLD) {
            float ratioScore = Math.min(feature.getUpDownRatio() / 10.0f, 1.0f);
            score += ratioScore;
            factors++;
        }

        // 短持续时间内大量查询
        if (feature.getDurationMs() < SHORT_DURATION_THRESHOLD && feature.getPps() > 10) {
            score += 0.5f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算字典 DGA 分数
     * 特征：中频查询、规律性间隔
     */
    private float calculateDictDgaScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_UDP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 中等频率查询
        if (feature.getPps() > 5.0f && feature.getPps() < HIGH_DNS_PPS_THRESHOLD) {
            score += 0.4f;
            factors++;
        }

        // 规律性间隔（低 IAT 标准差）
        if (feature.getIatMeanMs() > 0 && feature.getIatStdMs() / feature.getIatMeanMs() < 0.3f) {
            score += 0.5f;
            factors++;
        }

        // 中等上下行比
        if (feature.getUpDownRatio() > 1.5f && feature.getUpDownRatio() < HIGH_UP_DOWN_RATIO_THRESHOLD) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算 NXDOMAIN 洪泛分数
     * 特征：极高上下行比、高频查询
     */
    private float calculateNxdomainFloodScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_UDP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 极高上下行比（发送远多于接收）
        if (feature.getUpDownRatio() > 5.0f) {
            float ratioScore = Math.min(feature.getUpDownRatio() / 20.0f, 1.0f);
            score += ratioScore;
            factors++;
        }

        // 高频查询
        if (feature.getPps() > HIGH_DNS_PPS_THRESHOLD * 2) {
            float ppsScore = Math.min(feature.getPps() / (HIGH_DNS_PPS_THRESHOLD * 10), 1.0f);
            score += ppsScore;
            factors++;
        }

        // 短持续时间
        if (feature.getDurationMs() < SHORT_DURATION_THRESHOLD * 2) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算 C2 域名分数
     * 特征：周期性查询、规律性通信
     */
    private float calculateC2DomainScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_UDP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 周期性查询（规律性间隔）
        if (feature.getIatMeanMs() > 1000 && feature.getIatMeanMs() < 60000) {
            // 1秒到1分钟的间隔，典型的 C2 beacon
            if (feature.getIatStdMs() / feature.getIatMeanMs() < 0.2f) {
                score += 0.7f;
                factors++;
            }
        }

        // 长持续时间
        if (feature.getDurationMs() > 60000) { // 1分钟以上
            score += 0.4f;
            factors++;
        }

        // 低 PPS（周期性而非高频）
        if (feature.getPps() > 0.1f && feature.getPps() < 5.0f) {
            score += 0.3f;
            factors++;
        }

        // 近似平衡的上下行比
        if (feature.getUpDownRatio() > 0.5f && feature.getUpDownRatio() < 2.0f) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}