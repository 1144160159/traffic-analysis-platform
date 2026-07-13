package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.RuleType;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
import java.util.HashMap;
import java.util.Map;

/**
 * 规则匹配器工厂（增强版）
 * 
 * 管理所有类型的 RuleMatcher 实例
 * 
 * 新增：
 * - BruteForceMatcher
 */
public class MatcherFactory implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(MatcherFactory.class);

    private transient Map<RuleType, RuleMatcher> matchers;

    /**
     * 初始化所有匹配器
     */
    public void initialize() {
        matchers = new HashMap<>();

        // 注册所有匹配器
        matchers.put(RuleType.THRESHOLD, new ThresholdMatcher());
        matchers.put(RuleType.BLACKLIST, new BlacklistMatcher());
        matchers.put(RuleType.PORT_SCAN, new PortScanMatcher());
        matchers.put(RuleType.BRUTE_FORCE, new BruteForceMatcher());
        matchers.put(RuleType.DATA_EXFIL, new DataExfilMatcher());
        matchers.put(RuleType.ANOMALY, new AnomalyMatcher());
        matchers.put(RuleType.PROTOCOL_ANOMALY, new ProtocolAnomalyMatcher());
        matchers.put(RuleType.TLS_FINGERPRINT, new TlsFingerprintMatcher());

        LOG.info("MatcherFactory initialized with {} matchers", matchers.size());
    }

    /**
     * 获取指定类型的匹配器
     */
    public RuleMatcher getMatcher(RuleType type) {
        if (matchers == null) {
            initialize();
        }
        return matchers.get(type);
    }

    /**
     * 清理资源
     */
    public void close() {
        if (matchers != null) {
            matchers.clear();
        }
    }
}