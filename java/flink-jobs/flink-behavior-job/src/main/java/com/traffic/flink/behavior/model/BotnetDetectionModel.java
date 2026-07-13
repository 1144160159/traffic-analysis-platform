////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/BotnetDetectionModel.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.HashMap;
import java.util.Map;

/**
 * 僵尸网络检测模型
 * 
 * 检测类型：
 * 1. C2 信标 (c2_beacon) - 周期性命令控制通信
 * 2. 僵尸主机 (zombie_host) - 被控制的僵尸主机行为
 * 3. DDoS 参与 (ddos_participant) - 参与 DDoS 攻击
 * 4. 垃圾邮件发送 (spam_sender) - 发送垃圾邮件
 * 5. 挖矿行为 (crypto_mining) - 加密货币挖矿通信
 * 
 * 特征分析：
 * - 周期性心跳（固定时间间隔）
 * - 低数据量（心跳包很小）
 * - 长持续时间（持久连接）
 * - 规律性通信模式
 * - 异常端口使用
 */
public class BotnetDetectionModel extends AbstractBehaviorModel {

    private static final long serialVersionUID = 1L;

    // 协议号
    private static final int PROTOCOL_TCP = 6;
    private static final int PROTOCOL_UDP = 17;

    // 僵尸网络检测阈值
    private static final long MIN_BEACON_DURATION_MS = 300000; // 5分钟
    private static final float MAX_BEACON_PKT_SIZE = 200.0f; // 小包
    private static final float MAX_IAT_CV = 0.3f; // IAT 变异系数阈值（规律性）
    private static final float LOW_BPS_THRESHOLD = 10000.0f; // 10 Kbps
    private static final float HIGH_PPS_THRESHOLD = 100.0f; // DDoS 特征

    public BotnetDetectionModel(BehaviorJobConfig config) {
        super(config, "botnet", config.getModelVersion(), config.getBotnetThreshold(),
              "c2_beacon", "zombie_host", "ddos_participant", "spam_sender", "crypto_mining", "normal");
    }

    @Override
    public String getDescription() {
        return "Botnet detection model for identifying command-and-control and zombie host behaviors";
    }

    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Botnet detection model initialized with threshold: {}", threshold);
    }

    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();

        Map<String, Float> scores = new HashMap<>();

        // 1. C2 信标检测
        scores.put("c2_beacon", calculateC2BeaconScore(feature));

        // 2. 僵尸主机检测
        scores.put("zombie_host", calculateZombieHostScore(feature));

        // 3. DDoS 参与检测
        scores.put("ddos_participant", calculateDdosParticipantScore(feature));

        // 4. 垃圾邮件发送检测
        scores.put("spam_sender", calculateSpamSenderScore(feature));

        // 5. 挖矿行为检测
        scores.put("crypto_mining", calculateCryptoMiningScore(feature));

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
        builder.addFeatureImportance("duration_ms", (float) feature.getDurationMs());
        builder.addFeatureImportance("pktlen_mean", feature.getPktlenMean());
        builder.addFeatureImportance("iat_mean_ms", feature.getIatMeanMs());
        builder.addFeatureImportance("iat_std_ms", feature.getIatStdMs());
        builder.addFeatureImportance("up_down_ratio", feature.getUpDownRatio());

        // 计算并添加 IAT 变异系数
        if (feature.getIatMeanMs() > 0) {
            float iatCv = feature.getIatStdMs() / feature.getIatMeanMs();
            builder.addFeatureImportance("iat_cv", iatCv);
        }

        return builder.build();
    }

    /**
     * 计算 C2 信标分数
     * 特征：周期性心跳、小包、长连接、规律性
     */
    private float calculateC2BeaconScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 长持续时间（持久连接）
        if (feature.getDurationMs() >= MIN_BEACON_DURATION_MS) {
            float durationScore = Math.min(feature.getDurationMs() / (MIN_BEACON_DURATION_MS * 4.0f), 1.0f);
            score += durationScore * 0.3f;
            factors++;
        }

        // 小包（心跳包特征）
        if (feature.getPktlenMean() > 0 && feature.getPktlenMean() < MAX_BEACON_PKT_SIZE) {
            float pktScore = (MAX_BEACON_PKT_SIZE - feature.getPktlenMean()) / MAX_BEACON_PKT_SIZE;
            score += pktScore * 0.2f;
            factors++;
        }

        // 规律性通信（低 IAT 变异系数）
        if (feature.getIatMeanMs() > 0) {
            float iatCv = feature.getIatStdMs() / feature.getIatMeanMs();
            if (iatCv < MAX_IAT_CV && iatCv >= 0) {
                float regularityScore = (MAX_IAT_CV - iatCv) / MAX_IAT_CV;
                score += regularityScore * 0.3f;
                factors++;
            }
        }

        // 低流量（心跳流量小）
        if (feature.getBps() > 0 && feature.getBps() < LOW_BPS_THRESHOLD) {
            score += 0.2f;
            factors++;
        }

        // 合理的心跳间隔（1秒到5分钟）
        if (feature.getIatMeanMs() > 1000 && feature.getIatMeanMs() < 300000) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score, 1.0f) : 0.0f;
    }

    /**
     * 计算僵尸主机分数
     * 特征：被动响应、低上下行比、持续在线
     */
    private float calculateZombieHostScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 低上下行比（接收命令多于发送）
        if (feature.getUpDownRatio() > 0 && feature.getUpDownRatio() < 0.5f) {
            float ratioScore = (0.5f - feature.getUpDownRatio()) / 0.5f;
            score += ratioScore * 0.3f;
            factors++;
        }

        // 长持续时间
        if (feature.getDurationMs() > 60000) { // > 1分钟
            float durationScore = Math.min(feature.getDurationMs() / 600000.0f, 1.0f);
            score += durationScore * 0.3f;
            factors++;
        }

        // 低 PPS（等待命令状态）
        if (feature.getPps() > 0 && feature.getPps() < 10) {
            score += 0.2f;
            factors++;
        }

        // TCP 协议
        if (feature.getProtocol() == PROTOCOL_TCP) {
            score += 0.1f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors * 1.5f, 1.0f) : 0.0f;
    }

    /**
     * 计算 DDoS 参与分数
     * 特征：高 PPS、高上下行比、短连接、大量发送
     */
    private float calculateDdosParticipantScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 高 PPS（攻击流量）
        if (feature.getPps() > HIGH_PPS_THRESHOLD) {
            float ppsScore = Math.min(feature.getPps() / (HIGH_PPS_THRESHOLD * 10), 1.0f);
            score += ppsScore * 0.4f;
            factors++;
        }

        // 高上下行比（发送远多于接收）
        if (feature.getUpDownRatio() > 10.0f) {
            float ratioScore = Math.min(feature.getUpDownRatio() / 100.0f, 1.0f);
            score += ratioScore * 0.3f;
            factors++;
        }

        // 高 BPS
        if (feature.getBps() > 1000000) { // > 1 Mbps
            float bpsScore = Math.min(feature.getBps() / 100000000.0f, 1.0f);
            score += bpsScore * 0.2f;
            factors++;
        }

        // 小包（SYN Flood 等）
        if (feature.getPktlenMean() < 100) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score, 1.0f) : 0.0f;
    }

    /**
     * 计算垃圾邮件发送分数
     * 特征：SMTP 端口特征、高频短连接、高上下行比
     */
    private float calculateSpamSenderScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // TCP 协议（SMTP）
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        // 短连接
        if (feature.getDurationMs() > 0 && feature.getDurationMs() < 30000) {
            score += 0.3f;
            factors++;
        }

        // 高上下行比（发送邮件内容）
        if (feature.getUpDownRatio() > 5.0f) {
            float ratioScore = Math.min(feature.getUpDownRatio() / 20.0f, 1.0f);
            score += ratioScore * 0.3f;
            factors++;
        }

        // 中等包大小（邮件内容）
        if (feature.getPktlenMean() > 200 && feature.getPktlenMean() < 1000) {
            score += 0.2f;
            factors++;
        }

        // 中等 PPS
        if (feature.getPps() > 5 && feature.getPps() < 50) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors * 1.5f, 1.0f) : 0.0f;
    }

    /**
     * 计算加密货币挖矿分数
     * 特征：长连接、规律性通信、特定协议模式
     */
    private float calculateCryptoMiningScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // TCP 协议
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        // 长持续时间（挖矿是持续性的）
        if (feature.getDurationMs() > 600000) { // > 10分钟
            float durationScore = Math.min(feature.getDurationMs() / 3600000.0f, 1.0f);
            score += durationScore * 0.3f;
            factors++;
        }

        // 规律性通信（Stratum 协议特征）
        if (feature.getIatMeanMs() > 0) {
            float iatCv = feature.getIatStdMs() / feature.getIatMeanMs();
            if (iatCv < 0.5f) {
                score += 0.3f;
                factors++;
            }
        }

        // 双向通信（获取任务、提交结果）
        if (feature.getUpDownRatio() > 0.3f && feature.getUpDownRatio() < 3.0f) {
            score += 0.2f;
            factors++;
        }

        // 低到中等流量
        if (feature.getBps() > 1000 && feature.getBps() < 100000) {
            score += 0.2f;
            factors++;
        }

        // 小包（JSON-RPC 协议）
        if (feature.getPktlenMean() > 50 && feature.getPktlenMean() < 500) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors * 1.5f, 1.0f) : 0.0f;
    }
}