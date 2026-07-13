////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/PhishingDetectionModel.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.HashMap;
import java.util.Map;

/**
 * 钓鱼检测模型
 * 
 * 检测类型：
 * 1. 钓鱼网站访问 (phishing_site) - 访问钓鱼网站
 * 2. 凭证窃取 (credential_theft) - 凭证被窃取
 * 3. 表单提交 (form_submission) - 向可疑站点提交表单
 * 4. 重定向链 (redirect_chain) - 多重重定向钓鱼
 * 5. 克隆站点 (clone_site) - 访问克隆的合法网站
 * 
 * 特征分析：
 * - 短连接（一次性请求）
 * - 下载为主（获取页面）
 * - HTTPS 连接
 * - 数据提交行为
 * 
 * 注意：完整的钓鱼检测需要结合 URL 分析和威胁情报，
 * 这里基于流量模式进行辅助检测
 */
public class PhishingDetectionModel extends AbstractBehaviorModel {

    private static final long serialVersionUID = 1L;

    // 协议号
    private static final int PROTOCOL_TCP = 6;

    // 钓鱼检测阈值
    private static final float SHORT_DURATION_MS = 30000.0f; // 30秒
    private static final float VERY_SHORT_DURATION_MS = 5000.0f; // 5秒
    private static final float LOW_UP_DOWN_RATIO = 0.3f; // 下载为主
    private static final float HIGH_UP_DOWN_RATIO = 3.0f; // 上传数据

    public PhishingDetectionModel(BehaviorJobConfig config) {
        super(config, "phishing", config.getModelVersion(), config.getPhishingThreshold(),
              "phishing_site", "credential_theft", "form_submission", "redirect_chain", "clone_site", "normal");
    }

    @Override
    public String getDescription() {
        return "Phishing detection model for identifying phishing website access patterns";
    }

    @Override
    protected void doInitialize() throws Exception {
        LOG.info("Phishing detection model initialized with threshold: {}", threshold);
    }

    @Override
    protected ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures) {
        long startTime = System.nanoTime();

        // 钓鱼检测主要针对 TCP/HTTPS 流量
        if (feature.getProtocol() != PROTOCOL_TCP) {
            return ModelInferenceResult.empty(name, version);
        }

        Map<String, Float> scores = new HashMap<>();

        // 1. 钓鱼网站访问检测
        scores.put("phishing_site", calculatePhishingSiteScore(feature));

        // 2. 凭证窃取检测
        scores.put("credential_theft", calculateCredentialTheftScore(feature));

        // 3. 表单提交检测
        scores.put("form_submission", calculateFormSubmissionScore(feature));

        // 4. 重定向链检测
        scores.put("redirect_chain", calculateRedirectChainScore(feature));

        // 5. 克隆站点检测
        scores.put("clone_site", calculateCloneSiteScore(feature));

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
        builder.addFeatureImportance("up_down_ratio", feature.getUpDownRatio());
        builder.addFeatureImportance("pktlen_mean", feature.getPktlenMean());
        builder.addFeatureImportance("pps", feature.getPps());

        return builder.build();
    }

    /**
     * 计算钓鱼网站访问分数
     * 特征：短连接、下载为主、一次性请求
     */
    private float calculatePhishingSiteScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 短持续时间（访问后快速离开）
        if (feature.getDurationMs() > 0 && feature.getDurationMs() < SHORT_DURATION_MS) {
            float durationScore = (SHORT_DURATION_MS - feature.getDurationMs()) / SHORT_DURATION_MS;
            score += durationScore * 0.3f;
            factors++;
        }

        // 下载为主（获取页面内容）
        if (feature.getUpDownRatio() > 0 && feature.getUpDownRatio() < LOW_UP_DOWN_RATIO) {
            float ratioScore = (LOW_UP_DOWN_RATIO - feature.getUpDownRatio()) / LOW_UP_DOWN_RATIO;
            score += ratioScore * 0.3f;
            factors++;
        }

        // 中等包大小（HTML 页面）
        if (feature.getPktlenMean() > 200 && feature.getPktlenMean() < 1200) {
            score += 0.2f;
            factors++;
        }

        // 有数据传输
        if (feature.getBps() > 10000) {
            score += 0.1f;
            factors++;
        }

        return factors > 0 ? Math.min(score, 1.0f) : 0.0f;
    }

    /**
     * 计算凭证窃取分数
     * 特征：先下载后上传、特定数据模式
     */
    private float calculateCredentialTheftScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 中等持续时间（查看页面后提交）
        if (feature.getDurationMs() > 5000 && feature.getDurationMs() < 120000) {
            score += 0.3f;
            factors++;
        }

        // 有上传数据（提交凭证）
        if (feature.getUpDownRatio() > 0.1f && feature.getUpDownRatio() < 2.0f) {
            score += 0.3f;
            factors++;
        }

        // 小到中等的上传数据量（用户名密码）
        float upBytes = feature.getBps() * feature.getUpDownRatio() / (1 + feature.getUpDownRatio());
        if (upBytes > 100 && upBytes < 10000) {
            score += 0.2f;
            factors++;
        }

        // TCP 有完整握手
        if (feature.getTcpFlagSynCnt() > 0 && feature.getTcpFlagAckCnt() > 0) {
            score += 0.1f;
            factors++;
        }

        return factors > 0 ? Math.min(score, 1.0f) : 0.0f;
    }

    /**
     * 计算表单提交分数
     * 特征：POST 请求特征、数据上传
     */
    private float calculateFormSubmissionScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 短到中等持续时间
        if (feature.getDurationMs() > 1000 && feature.getDurationMs() < 60000) {
            score += 0.2f;
            factors++;
        }

        // 有明显的上传（表单数据）
        if (feature.getUpDownRatio() > 0.5f && feature.getUpDownRatio() < HIGH_UP_DOWN_RATIO) {
            score += 0.4f;
            factors++;
        }

        // 中等 PPS
        if (feature.getPps() > 2 && feature.getPps() < 20) {
            score += 0.2f;
            factors++;
        }

        // 有一定的数据量
        if (feature.getBps() > 5000 && feature.getBps() < 500000) {
            score += 0.2f;
            factors++;
        }

        return factors > 0 ? Math.min(score, 1.0f) : 0.0f;
    }

    /**
     * 计算重定向链分数
     * 特征：多个短连接、快速跳转
     */
    private float calculateRedirectChainScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 非常短的持续时间（快速重定向）
        if (feature.getDurationMs() > 0 && feature.getDurationMs() < VERY_SHORT_DURATION_MS) {
            float durationScore = (VERY_SHORT_DURATION_MS - feature.getDurationMs()) / VERY_SHORT_DURATION_MS;
            score += durationScore * 0.4f;
            factors++;
        }

        // 下载为主（302/301 响应）
        if (feature.getUpDownRatio() > 0 && feature.getUpDownRatio() < 0.2f) {
            score += 0.3f;
            factors++;
        }

        // 小数据量（重定向响应很小）
        if (feature.getBps() < 50000 && feature.getBps() > 0) {
            score += 0.2f;
            factors++;
        }

        // 低 PPS
        if (feature.getPps() > 0 && feature.getPps() < 10) {
            score += 0.1f;
            factors++;
        }

        return factors > 0 ? Math.min(score, 1.0f) : 0.0f;
    }

    /**
     * 计算克隆站点分数
     * 特征：类似正常网站访问但有细微差异
     */
    private float calculateCloneSiteScore(FeatureStat feature) {
        float score = 0.0f;
        int factors = 0;

        // 正常的访问时长
        if (feature.getDurationMs() > 10000 && feature.getDurationMs() < 300000) {
            score += 0.2f;
            factors++;
        }

        // 下载为主
        if (feature.getUpDownRatio() > 0 && feature.getUpDownRatio() < 0.5f) {
            score += 0.2f;
            factors++;
        }

        // 有一定的数据传输（页面资源）
        if (feature.getBps() > 50000) {
            score += 0.2f;
            factors++;
        }

        // 中等 PPS（加载多个资源）
        if (feature.getPps() > 5 && feature.getPps() < 50) {
            score += 0.2f;
            factors++;
        }

        // 注意：克隆站点的精确检测需要 URL 和证书分析
        // 这里只能基于流量模式给出低置信度的分数

        return factors > 0 ? Math.min(score * 0.5f, 1.0f) : 0.0f;
    }
}