package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.HashMap;
import java.util.Map;

/**
 * 加密流量分析模型
 * 
 * 检测类型：
 * 1. 可疑 TLS (suspicious_tls) - 异常 TLS 握手或通信
 * 2. 自签名证书 (self_signed) - 使用自签名证书
 * 3. 非标准加密 (non_standard_crypto) - 非标准加密协议
 * 4. 加密隧道 (encrypted_tunnel) - 隐藏在加密中的隧道
 * 5. Tor 流量 (tor_traffic) - Tor 网络流量特征
 * 
 * 特征分析：
 * - 包长分布
 * - 握手模式
 * - 通信规律性
 */
public class EncryptedTrafficModel extends AbstractBehaviorModel {

    private static final long serialVersionUID = 1L;

    private static final int PROTOCOL_TCP = 6;

    // TLS 特征阈值
    private static final float TLS_PKT_SIZE_MIN = 40.0f;
    private static final float TLS_PKT_SIZE_MAX = 1500.0f;
    private static final float TLS_LONG_DURATION = 60000.0f;

    public EncryptedTrafficModel(BehaviorJobConfig config) {
        super(config, "encrypted", config.getModelVersion(), config.getEncryptedTrafficThreshold(),
              "suspicious_tls", "self_signed", "non_standard_crypto", "encrypted_tunnel", "tor_traffic", "normal");
    }

    @Override
    public String getDescription() {
        return "Encrypted traffic analysis model for detecting malicious encrypted communications";
    }

    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Encrypted traffic model initialized with threshold: {}", threshold);
    }

    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();

        Map<String, Float> scores = new HashMap<>();

        // 1. 可疑 TLS 检测
        scores.put("suspicious_tls", calculateSuspiciousTlsScore(feature));

        // 2. 自签名证书检测（基于流量特征推断）
        scores.put("self_signed", calculateSelfSignedScore(feature));

        // 3. 非标准加密检测
        scores.put("non_standard_crypto", calculateNonStandardCryptoScore(feature));

        // 4. 加密隧道检测
        scores.put("encrypted_tunnel", calculateEncryptedTunnelScore(feature));

        // 5. Tor 流量检测
        scores.put("tor_traffic", calculateTorTrafficScore(feature));

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

        builder.addFeatureImportance("pktlen_mean", feature.getPktlenMean());
        builder.addFeatureImportance("pktlen_std", feature.getPktlenStd());
        builder.addFeatureImportance("duration_ms", (float) feature.getDurationMs());

        return builder.build();
    }

    private float calculateSuspiciousTlsScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 异常小的包（可能是恶意软件精简实现）
        if (feature.getPktlenMean() < 100 && feature.getPktlenMean() > 40) {
            score += 0.4f;
            factors++;
        }

        // 异常规律的包长（加密数据应该相对均匀）
        if (feature.getPktlenStd() < 50 && feature.getPktlenMean() > 100) {
            score += 0.3f;
            factors++;
        }

        // 快速连接断开
        if (feature.getDurationMs() < 5000 && feature.getPps() > 10) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    private float calculateSelfSignedScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 短连接（可能跳过证书验证）
        if (feature.getDurationMs() < 10000) {
            score += 0.2f;
            factors++;
        }

        // 小握手包
        if (feature.getPktlenMean() < 500) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    private float calculateNonStandardCryptoScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 非 TCP 的加密特征（可能是自定义协议）
        if (feature.getProtocol() != PROTOCOL_TCP) {
            // 高熵特征（通过包长分布推断）
            if (feature.getPktlenStd() / (feature.getPktlenMean() + 1) < 0.3f) {
                score += 0.4f;
                factors++;
            }
        }

        // 规律性通信
        if (feature.getIatStdMs() / (feature.getIatMeanMs() + 1) < 0.2f) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    private float calculateEncryptedTunnelScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 长连接
        if (feature.getDurationMs() > TLS_LONG_DURATION) {
            score += 0.4f;
            factors++;
        }

        // 持续高吞吐
        if (feature.getBps() > 100000 && feature.getDurationMs() > 30000) {
            score += 0.3f;
            factors++;
        }

        // 双向通信
        if (feature.getUpDownRatio() > 0.3f && feature.getUpDownRatio() < 3.0f) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    private float calculateTorTrafficScore(FeatureStat feature) {
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // Tor 典型特征：固定包大小（512 字节 cell）
        if (Math.abs(feature.getPktlenMean() - 512) < 50) {
            score += 0.5f;
            factors++;
        }

        // 低包长标准差
        if (feature.getPktlenStd() < 100) {
            score += 0.3f;
            factors++;
        }

        // 长连接
        if (feature.getDurationMs() > 60000) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}