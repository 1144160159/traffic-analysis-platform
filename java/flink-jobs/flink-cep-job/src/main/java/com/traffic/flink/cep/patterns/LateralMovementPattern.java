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
 * 横向移动模式
 * 
 * 模式描述：60 分钟内，攻击者从一台主机访问另一台主机，形成跳板链
 * 
 * 检测逻辑：
 * 1. 初始入侵告警
 * 2. 从被入侵主机发起的内部扫描/凭据访问
 * 3. 对新目标的成功访问
 */
public class LateralMovementPattern {

    // 初始入侵类型
    private static final Set<String> COMPROMISE_TYPES = new HashSet<>(Arrays.asList(
            "EXPLOIT", "RCE", "MALWARE", "BACKDOOR", "INITIAL_ACCESS"
    ));

    // 内部活动类型
    private static final Set<String> INTERNAL_ACTIVITY_TYPES = new HashSet<>(Arrays.asList(
            "INTERNAL_SCAN", "CREDENTIAL_DUMP", "PASS_THE_HASH", "SMB_RELAY"
    ));

    // 横向移动类型
    private static final Set<String> LATERAL_TYPES = new HashSet<>(Arrays.asList(
            "LATERAL_MOVEMENT", "REMOTE_SERVICE", "WMI_EXEC", "PSEXEC", "SSH_PIVOT"
    ));

    /**
     * 创建横向移动模式
     */
    public static Pattern<Alert, ?> create(PatternConfig config) {
        return Pattern.<Alert>begin("compromise")
                .where(new SimpleCondition<Alert>() {
                    @Override
                    public boolean filter(Alert alert) {
                        return isCompromiseAlert(alert);
                    }
                })
                .followedBy("internal_activity")
                .where(new IterativeCondition<Alert>() {
                    @Override
                    public boolean filter(Alert alert, Context<Alert> ctx) throws Exception {
                        if (!isInternalActivity(alert)) {
                            return false;
                        }
                        
                        // 源 IP 应该是之前被入侵的目标 IP
                        for (Alert compromise : ctx.getEventsForPattern("compromise")) {
                            if (compromise.getDstIp().equals(alert.getSrcIp())) {
                                return true;
                            }
                        }
                        return false;
                    }
                })
                .oneOrMore()
                .optional()
                .followedBy("lateral")
                .where(new IterativeCondition<Alert>() {
                    @Override
                    public boolean filter(Alert alert, Context<Alert> ctx) throws Exception {
                        if (!isLateralMovement(alert)) {
                            return false;
                        }
                        
                        // 验证目标是内部新主机
                        for (Alert internal : ctx.getEventsForPattern("internal_activity")) {
                            if (internal.getSrcIp().equals(alert.getSrcIp()) &&
                                !internal.getDstIp().equals(alert.getDstIp())) {
                                return true;
                            }
                        }
                        
                        // 或者直接从被入侵主机发起
                        for (Alert compromise : ctx.getEventsForPattern("compromise")) {
                            if (compromise.getDstIp().equals(alert.getSrcIp())) {
                                return true;
                            }
                        }
                        
                        return false;
                    }
                })
                .within(Time.minutes(config.getLateralMovementWindowMinutes()));
    }

    /**
     * 创建默认配置的模式
     */
    public static Pattern<Alert, ?> create() {
        return create(PatternConfig.defaultConfig());
    }

    /**
     * 判断是否是初始入侵告警
     */
    private static boolean isCompromiseAlert(Alert alert) {
        String type = alert.getAlertType().toUpperCase();
        if (COMPROMISE_TYPES.contains(type)) {
            return true;
        }
        
        for (String label : alert.getLabelsList()) {
            String lowerLabel = label.toLowerCase();
            if (lowerLabel.contains("exploit") || 
                lowerLabel.contains("rce") ||
                lowerLabel.contains("initial_access")) {
                return true;
            }
        }
        
        return false;
    }

    /**
     * 判断是否是内部活动
     */
    private static boolean isInternalActivity(Alert alert) {
        String type = alert.getAlertType().toUpperCase();
        if (INTERNAL_ACTIVITY_TYPES.contains(type)) {
            return true;
        }
        
        for (String label : alert.getLabelsList()) {
            String lowerLabel = label.toLowerCase();
            if (lowerLabel.contains("scan") || 
                lowerLabel.contains("credential") ||
                lowerLabel.contains("internal")) {
                return true;
            }
        }
        
        return false;
    }

    /**
     * 判断是否是横向移动
     */
    private static boolean isLateralMovement(Alert alert) {
        String type = alert.getAlertType().toUpperCase();
        if (LATERAL_TYPES.contains(type)) {
            return true;
        }
        
        for (String label : alert.getLabelsList()) {
            String lowerLabel = label.toLowerCase();
            if (lowerLabel.contains("lateral") || 
                lowerLabel.contains("pivot") ||
                lowerLabel.contains("remote_service")) {
                return true;
            }
        }
        
        return false;
    }
}