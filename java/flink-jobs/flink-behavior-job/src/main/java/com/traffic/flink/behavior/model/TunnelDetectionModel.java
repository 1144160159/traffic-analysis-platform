package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.HashMap;
import java.util.Map;

/**
 * 隧道检测模型
 * 
 * 检测类型：
 * 1. DNS 隧道 (dns_tunnel) - DNS 协议中的数据隐藏
 * 2. ICMP 隧道 (icmp_tunnel) - ICMP 协议中的数据隐藏
 * 3. HTTP 隧道 (http_tunnel) - HTTP 协议中的隐蔽通道
 * 4. SSH 隧道 (ssh_tunnel) - SSH 端口转发
 * 5. 通用隐蔽通道 (covert_channel) - 其他隐蔽通信
 * 
 * 特征分析：
 * - DNS 隧道：异常大的 DNS 查询/响应、高频 DNS 请求
 * - ICMP 隧道：异常大的 ICMP 载荷、规律性通信
 * - HTTP 隧道：长连接、持续数据传输
 */
public class TunnelDetectionModel extends AbstractBehaviorModel {

    private static final long serialVersionUID = 1L;

    // 协议号
    private static final int PROTOCOL_ICMP = 1;
    private static final int PROTOCOL_TCP = 6;
    private static final int PROTOCOL_UDP = 17;

    // DNS 隧道检测阈值
    private static final float DNS_HIGH_PKT_SIZE_THRESHOLD = 200.0f;
    private static final float DNS_HIGH_FREQ_THRESHOLD = 10.0f; // 10 PPS
    private static final float DNS_HIGH_UP_DOWN_RATIO = 1.5f;

    // ICMP 隧道检测阈值
    private static final float ICMP_HIGH_PKT_SIZE_THRESHOLD = 100.0f;
    private static final float ICMP_HIGH_FREQ_THRESHOLD = 5.0f;
    private static final float ICMP_LONG_DURATION_THRESHOLD = 60000.0f; // 60秒

    // HTTP 隧道检测阈值
    private static final float HTTP_LONG_DURATION_THRESHOLD = 300000.0f; // 5分钟
    private static final float HTTP_LOW_IAT_STD_THRESHOLD = 10.0f;
    private static final float HTTP_HIGH_BPS_THRESHOLD = 100000.0f;

    public TunnelDetectionModel(BehaviorJobConfig config) {
        super(config, "tunnel", config.getModelVersion(), config.getTunnelThreshold(),
              "dns_tunnel", "icmp_tunnel", "http_tunnel", "ssh_tunnel", "covert_channel", "normal");
    }

    @Override
    public String getDescription() {
        return "Tunnel and covert channel detection model using traffic analysis";
    }

    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Tunnel detection model initialized with threshold: {}", threshold);
    }

    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();

        // 计算各类隧道的分数
        Map<String, Float> tunnelScores = new HashMap<>();

        // 根据协议分析
        int protocol = feature.getProtocol();

        // 1. DNS 隧道检测（UDP 端口 53）
        float dnsTunnelScore = calculateDnsTunnelScore(feature);
        tunnelScores.put("dns_tunnel", dnsTunnelScore);

        // 2. ICMP 隧道检测
        float icmpTunnelScore = calculateIcmpTunnelScore(feature);
        tunnelScores.put("icmp_tunnel", icmpTunnelScore);

        // 3. HTTP 隧道检测
        float httpTunnelScore = calculateHttpTunnelScore(feature);
        tunnelScores.put("http_tunnel", httpTunnelScore);

        // 4. SSH 隧道检测
        float sshTunnelScore = calculateSshTunnelScore(feature);
        tunnelScores.put("ssh_tunnel", sshTunnelScore);

        // 5. 通用隐蔽通道检测
        float covertChannelScore = calculateCovertChannelScore(feature);
        tunnelScores.put("covert_channel", covertChannelScore);

        // 找到最高分和对应标签
        String topLabel = "normal";
        float topScore = 0.0f;
        boolean detected = false;

        for (Map.Entry<String, Float> entry : tunnelScores.entrySet()) {
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
        for (Map.Entry<String, Float> entry : tunnelScores.entrySet()) {
            builder.addLabel(entry.getKey(), entry.getValue());
        }

        // 添加特征重要性
        builder.addFeatureImportance("protocol", (float) feature.getProtocol());
        builder.addFeatureImportance("pktlen_mean", feature.getPktlenMean());
        builder.addFeatureImportance("pps", feature.getPps());
        builder.addFeatureImportance("duration_ms", (float) feature.getDurationMs());
        builder.addFeatureImportance("iat_std_ms", feature.getIatStdMs());

        return builder.build();
    }

    /**
     * 计算 DNS 隧道分数
     * 特征：大 DNS 包、高频率、异常上下行比
     */
    private float calculateDnsTunnelScore(FeatureStat feature) {
        // 只分析 UDP 流量
        if (feature.getProtocol() != PROTOCOL_UDP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 大包（DNS 查询通常较小）
        if (feature.getPktlenMean() > DNS_HIGH_PKT_SIZE_THRESHOLD) {
            float pktScore = Math.min((feature.getPktlenMean() - DNS_HIGH_PKT_SIZE_THRESHOLD) / 300.0f, 1.0f);
            score += pktScore;
            factors++;
        }

        // 高频率 DNS 请求
        if (feature.getPps() > DNS_HIGH_FREQ_THRESHOLD) {
            float freqScore = Math.min(feature.getPps() / (DNS_HIGH_FREQ_THRESHOLD * 10), 1.0f);
            score += freqScore;
            factors++;
        }

        // 异常上下行比（隧道通常有大量数据传输）
        if (feature.getUpDownRatio() > DNS_HIGH_UP_DOWN_RATIO || 
            feature.getUpDownRatio() < 1.0f / DNS_HIGH_UP_DOWN_RATIO) {
            score += 0.5f;
            factors++;
        }

        // 长持续时间
        if (feature.getDurationMs() > 30000) { // 30秒
            score += 0.3f;
            factors++;
        }

        // 低 IAT 标准差（规律性通信）
        if (feature.getIatStdMs() < 100.0f && feature.getIatStdMs() > 0) {
            score += 0.4f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算 ICMP 隧道分数
     * 特征：大 ICMP 载荷、规律性、长持续时间
     */
    private float calculateIcmpTunnelScore(FeatureStat feature) {
        // 只分析 ICMP 流量
        if (feature.getProtocol() != PROTOCOL_ICMP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 大包（正常 ICMP 很小）
        if (feature.getPktlenMean() > ICMP_HIGH_PKT_SIZE_THRESHOLD) {
            float pktScore = Math.min((feature.getPktlenMean() - ICMP_HIGH_PKT_SIZE_THRESHOLD) / 500.0f, 1.0f);
            score += pktScore;
            factors++;
        }

        // 高频率 ICMP
        if (feature.getPps() > ICMP_HIGH_FREQ_THRESHOLD) {
            float freqScore = Math.min(feature.getPps() / (ICMP_HIGH_FREQ_THRESHOLD * 5), 1.0f);
            score += freqScore;
            factors++;
        }

        // 长持续时间
        if (feature.getDurationMs() > ICMP_LONG_DURATION_THRESHOLD) {
            score += 0.7f;
            factors++;
        }

        // 规律性通信（低 IAT 标准差）
        if (feature.getIatMeanMs() > 0 && feature.getIatStdMs() / feature.getIatMeanMs() < 0.5f) {
            score += 0.5f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算 HTTP 隧道分数
     * 特征：长连接、持续数据传输、规律性
     */
    private float calculateHttpTunnelScore(FeatureStat feature) {
        // 只分析 TCP 流量
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 长持续时间
        if (feature.getDurationMs() > HTTP_LONG_DURATION_THRESHOLD) {
            float durationScore = Math.min(feature.getDurationMs() / (HTTP_LONG_DURATION_THRESHOLD * 5), 1.0f);
            score += durationScore;
            factors++;
        }

        // 高吞吐量
        if (feature.getBps() > HTTP_HIGH_BPS_THRESHOLD) {
            float bpsScore = Math.min(feature.getBps() / (HTTP_HIGH_BPS_THRESHOLD * 10), 1.0f);
            score += bpsScore;
            factors++;
        }

        // 低 IAT 标准差（规律性）
        if (feature.getIatStdMs() < HTTP_LOW_IAT_STD_THRESHOLD && feature.getIatStdMs() > 0) {
            score += 0.6f;
            factors++;
        }

        // 双向通信
        if (feature.getUpDownRatio() > 0.5f && feature.getUpDownRatio() < 2.0f) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算 SSH 隧道分数
     * 特征：长连接、持续活动、端口转发模式
     */
    private float calculateSshTunnelScore(FeatureStat feature) {
        // 只分析 TCP 流量
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return 0.0f;
        }

        float score = 0.0f;
        int factors = 0;

        // 长持续时间
        if (feature.getDurationMs() > 600000) { // 10分钟
            float durationScore = Math.min(feature.getDurationMs() / 3600000.0f, 1.0f);
            score += durationScore;
            factors++;
        }

        // 持续活动（低 idle）
        if (feature.getIdleMeanMs() < 10000 && feature.getDurationMs() > 60000) {
            score += 0.5f;
            factors++;
        }

        // 双向通信
        if (feature.getUpDownRatio() > 0.3f && feature.getUpDownRatio() < 3.0f) {
            score += 0.4f;
            factors++;
        }

        // 加密特征（相对均匀的包长）
        if (feature.getPktlenStd() < feature.getPktlenMean() * 0.5f && feature.getPktlenMean() > 100) {
            score += 0.3f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }

    /**
     * 计算通用隐蔽通道分数
     * 特征：规律性、低变化、持续时间长
     */
    private float calculateCovertChannelScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 规律性通信（IAT 标准差相对均值较低）
        if (feature.getIatMeanMs() > 0) {
            float cv = feature.getIatStdMs() / feature.getIatMeanMs();
            if (cv < 0.3f && cv > 0) {
                score += (0.3f - cv) / 0.3f;
                factors++;
            }
        }

        // 包长变化小
        if (feature.getPktlenMean() > 0) {
            float pktCv = feature.getPktlenStd() / feature.getPktlenMean();
            if (pktCv < 0.2f && pktCv > 0) {
                score += (0.2f - pktCv) / 0.2f;
                factors++;
            }
        }

        // 长持续时间
        if (feature.getDurationMs() > 120000) { // 2分钟
            score += 0.3f;
            factors++;
        }

        // 持续活动
        if (feature.getActiveMeanMs() > feature.getIdleMeanMs() && feature.getActiveMeanMs() > 0) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score / factors, 1.0f) : 0.0f;
    }
}