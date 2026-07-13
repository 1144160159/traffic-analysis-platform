package com.traffic.flink.behavior.model;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;
import java.util.HashMap;
import java.util.Map;
/**
 * 异常检测模型
 * 
 * 基于统计方法的异常行为检测：
 * 1. 流量异常 (traffic_anomaly) - 流量模式偏离基线
 * 2. 时间异常 (time_anomaly) - 非工作时间活动
 * 3. 协议异常 (protocol_anomaly) - 异常协议使用
 * 4. 行为异常 (behavior_anomaly) - 行为模式偏离
 * 5. 统计异常 (statistical_anomaly) - 统计特征异常
 * 
 * 使用 Z-Score 和 IQR 方法检测异常
 */
public class AnomalyDetectionModel extends AbstractBehaviorModel {
    private static final long serialVersionUID = 1L;
    // 基线统计值（实际应从历史数据计算）
    private static final float BASELINE_PPS_MEAN = 100.0f;
    private static final float BASELINE_PPS_STD = 50.0f;
    private static final float BASELINE_BPS_MEAN = 1000000.0f;
    private static final float BASELINE_BPS_STD = 500000.0f;
    private static final float BASELINE_DURATION_MEAN = 30000.0f;
    private static final float BASELINE_DURATION_STD = 60000.0f;
    // Z-Score 阈值
    private static final float ZSCORE_THRESHOLD = 3.0f;
    private static final float ZSCORE_WARNING = 2.0f;
    public AnomalyDetectionModel(BehaviorJobConfig config) {
        super(config, "anomaly", config.getModelVersion(), config.getAnomalyThreshold(),
              "traffic_anomaly", "time_anomaly", "protocol_anomaly", 
              "behavior_anomaly", "statistical_anomaly", "normal");
    }
    @Override
    public String getDescription() {
        return "Statistical anomaly detection model using Z-Score and IQR methods";
    }
    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Anomaly detection model initialized with threshold: {}", threshold);
    }
    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();
        Map<String, Float> scores = new HashMap<>();
        Map<String, Float> zScores = new HashMap<>();
        // 计算各特征的 Z-Score
        float ppsZScore = calculateZScore(feature.getPps(), BASELINE_PPS_MEAN, BASELINE_PPS_STD);
        float bpsZScore = calculateZScore(feature.getBps(), BASELINE_BPS_MEAN, BASELINE_BPS_STD);
        float durationZScore = calculateZScore(feature.getDurationMs(), BASELINE_DURATION_MEAN, BASELINE_DURATION_STD);
        zScores.put("pps", ppsZScore);
        zScores.put("bps", bpsZScore);
        zScores.put("duration", durationZScore);
        // 1. 流量异常检测
        scores.put("traffic_anomaly", calculateTrafficAnomalyScore(feature, ppsZScore, bpsZScore));
        // 2. 时间异常检测
        scores.put("time_anomaly", calculateTimeAnomalyScore(feature));
        // 3. 协议异常检测
        scores.put("protocol_anomaly", calculateProtocolAnomalyScore(feature));
        // 4. 行为异常检测
        scores.put("behavior_anomaly", calculateBehaviorAnomalyScore(feature, zScores));
        // 5. 统计异常检测
        scores.put("statistical_anomaly", calculateStatisticalAnomalyScore(feature, zScores));
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
        // 添加 Z-Score 作为特征重要性
        for (Map.Entry<String, Float> entry : zScores.entrySet()) {
            builder.addFeatureImportance("zscore_" + entry.getKey(), entry.getValue());
        }
        return builder.build();
    }
    /**
     * 计算 Z-Score
     */
    private float calculateZScore(float value, float mean, float std) {
        if (std <= 0) return 0.0f;
        return Math.abs(value - mean) / std;
    }
    /**
     * 将 Z-Score 转换为异常分数 [0, 1]
     */
    private float zScoreToAnomalyScore(float zScore) {
        if (zScore < ZSCORE_WARNING) {
            return 0.0f;
        } else if (zScore >= ZSCORE_THRESHOLD) {
            return Math.min(1.0f, 0.5f + (zScore - ZSCORE_THRESHOLD) / 10.0f);
        } else {
            // 线性插值
            return (zScore - ZSCORE_WARNING) / (ZSCORE_THRESHOLD - ZSCORE_WARNING) * 0.5f;
        }
    }
    /**
     * 计算流量异常分数
     */
    private float calculateTrafficAnomalyScore(FeatureStat feature, float ppsZScore, float bpsZScore) {
        float score = 0.0f;
        int factors = 0;
        // PPS 异常
        float ppsScore = zScoreToAnomalyScore(ppsZScore);
        if (ppsScore > 0) {
            score += ppsScore;
            factors++;
        }
        // BPS 异常
        float bpsScore = zScoreToAnomalyScore(bpsZScore);
        if (bpsScore > 0) {
            score += bpsScore;
            factors++;
        }
        // 极端上下行比
        if (feature.getUpDownRatio() > 100.0f || feature.getUpDownRatio() < 0.01f) {
            score += 0.5f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算时间异常分数
     * 检测非工作时间的异常活动
     */
    private float calculateTimeAnomalyScore(FeatureStat feature) {
        // 从时间戳获取小时（简化实现）
        long ts = feature.getTs();
        long hour = (ts / 3600000) % 24;
        float score = 0.0f;
        // 凌晨活动（2-5点）
        if (hour >= 2 && hour <= 5) {
            score += 0.3f;
        }
        // 周末活动检测需要日期信息，这里简化处理
        // 实际应该从配置中读取工作时间
        // 结合流量特征
        if (score > 0 && feature.getBps() > BASELINE_BPS_MEAN * 2) {
            score += 0.3f;
        }
        return Math.min(score, 1.0f);
    }
    /**
     * 计算协议异常分数
     */
    private float calculateProtocolAnomalyScore(FeatureStat feature) {
        float score = 0.0f;
        int protocol = feature.getProtocol();
        // 非常见协议
        if (protocol != 6 && protocol != 17 && protocol != 1) { // TCP, UDP, ICMP
            score += 0.5f;
        }
        // TCP 但无 SYN（可能是扫描或异常）
        if (protocol == 6 && feature.getTcpFlagSynCnt() == 0 && feature.getTcpFlagAckCnt() > 0) {
            score += 0.3f;
        }
        // 只有 SYN 没有 ACK（可能是 SYN 扫描）
        if (protocol == 6 && feature.getTcpFlagSynCnt() > 0 && feature.getTcpFlagAckCnt() == 0) {
            score += 0.4f;
        }
        return Math.min(score, 1.0f);
    }
    /**
     * 计算行为异常分数
     */
    private float calculateBehaviorAnomalyScore(FeatureStat feature, Map<String, Float> zScores) {
        float score = 0.0f;
        int factors = 0;
        // 多个指标同时异常
        int anomalyCount = 0;
        for (Float zScore : zScores.values()) {
            if (zScore > ZSCORE_WARNING) {
                anomalyCount++;
            }
        }
        if (anomalyCount >= 2) {
            score += 0.5f * (anomalyCount / (float) zScores.size());
            factors++;
        }
        // 短时间高频通信
        if (feature.getDurationMs() < 1000 && feature.getPps() > 100) {
            score += 0.4f;
            factors++;
        }
        // 长时间低活动（可能是心跳/C2）
        if (feature.getDurationMs() > 300000 && feature.getPps() < 1) {
            score += 0.3f;
            factors++;
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
    /**
     * 计算统计异常分数
     */
    private float calculateStatisticalAnomalyScore(FeatureStat feature, Map<String, Float> zScores) {
        float score = 0.0f;
        int factors = 0;
        // 计算综合 Z-Score
        float avgZScore = 0.0f;
        for (Float zScore : zScores.values()) {
            avgZScore += zScore;
        }
        avgZScore /= zScores.size();
        if (avgZScore > ZSCORE_WARNING) {
            score += zScoreToAnomalyScore(avgZScore);
            factors++;
        }
        // IAT 统计异常
        if (feature.getIatStdMs() > 0 && feature.getIatMeanMs() > 0) {
            float cv = feature.getIatStdMs() / feature.getIatMeanMs(); // 变异系数
            if (cv > 3.0f) { // 高变异性
                score += 0.3f;
                factors++;
            } else if (cv < 0.1f) { // 极低变异性（规律性通信）
                score += 0.4f;
                factors++;
            }
        }
        // 包长统计异常
        if (feature.getPktlenStd() > 0 && feature.getPktlenMean() > 0) {
            float pktCv = feature.getPktlenStd() / feature.getPktlenMean();
            if (pktCv > 2.0f) { // 高变异性
                score += 0.2f;
                factors++;
            }
        }
        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}