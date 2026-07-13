package com.traffic.flink.cep.patterns;

import com.traffic.proto.traffic.v1.Alert;

import org.apache.flink.cep.pattern.Pattern;
import org.apache.flink.cep.pattern.conditions.IterativeCondition;
import org.apache.flink.cep.pattern.conditions.SimpleCondition;
import org.apache.flink.streaming.api.windowing.time.Time;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

/**
 * 暴力破解模式
 * 
 * 模式描述：10 分钟内，同一源 IP 对同一目标触发 >= 5 次登录失败，随后成功登录
 * 
 * 攻击链：凭据访问 → 初始访问
 */
public class BruteForcePattern {

    // 登录失败类型
    private static final Set<String> FAILED_TYPES = new HashSet<>(Arrays.asList(
            "AUTH_FAILED", "LOGIN_FAILED", "BRUTE_FORCE", "CREDENTIAL_ACCESS"
    ));

    // 登录成功类型
    private static final Set<String> SUCCESS_TYPES = new HashSet<>(Arrays.asList(
            "AUTH_SUCCESS", "LOGIN_SUCCESS", "SESSION_CREATED"
    ));

    /**
     * 创建暴力破解模式
     */
    public static Pattern<Alert, ?> create(PatternConfig config) {
        return Pattern.<Alert>begin("failed")
                .where(new SimpleCondition<Alert>() {
                    @Override
                    public boolean filter(Alert alert) {
                        return isFailedLogin(alert);
                    }
                })
                .timesOrMore(config.getMinFailedAttempts())
                .greedy()
                .followedBy("success")
                .where(new IterativeCondition<Alert>() {
                    @Override
                    public boolean filter(Alert alert, Context<Alert> ctx) throws Exception {
                        if (!isSuccessLogin(alert)) {
                            return false;
                        }
                        
                        // 确保成功登录的目标与失败尝试目标一致
                        for (Alert failed : ctx.getEventsForPattern("failed")) {
                            if (failed.getDstIp().equals(alert.getDstIp()) &&
                                failed.getDstPort() == alert.getDstPort()) {
                                return true;
                            }
                        }
                        return false;
                    }
                })
                .within(Time.minutes(config.getBruteForceWindowMinutes()));
    }

    /**
     * 创建默认配置的模式
     */
    public static Pattern<Alert, ?> create() {
        return create(PatternConfig.defaultConfig());
    }

    private static boolean isFailedLogin(Alert alert) {
        String type = alert.getAlertType().toUpperCase();
        if (FAILED_TYPES.contains(type)) {
            return true;
        }
        
        for (String label : alert.getLabelsList()) {
            if (label.toLowerCase().contains("brute") || 
                label.toLowerCase().contains("failed") ||
                label.toLowerCase().contains("auth_fail")) {
                return true;
            }
        }
        
        return false;
    }

    private static boolean isSuccessLogin(Alert alert) {
        String type = alert.getAlertType().toUpperCase();
        if (SUCCESS_TYPES.contains(type)) {
            return true;
        }
        
        for (String label : alert.getLabelsList()) {
            if (label.toLowerCase().contains("success") ||
                label.toLowerCase().contains("auth_ok")) {
                return true;
            }
        }
        
        return false;
    }
}