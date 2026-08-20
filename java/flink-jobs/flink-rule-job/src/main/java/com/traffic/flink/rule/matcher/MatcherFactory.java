package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.RuleType;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collection;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 规则匹配器工厂(策略 + 简单工厂)
 *
 * 单一构造点 createMatcher() 维护"类型 → 具体策略"映射,新增检测类型只改
 * 这一处;initialize(Collection) 支持按作业/租户配置装配匹配器子集,
 * 未选中的类型 getMatcher() 返回 null(由调用方显式跳过)。
 */
public class MatcherFactory implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(MatcherFactory.class);

    private transient Map<RuleType, RuleMatcher> matchers;
    private transient List<RuleType> registeredTypes;

    /**
     * 默认产品族:全部已批准匹配器(向后兼容)。
     */
    public static List<RuleType> allDefaultTypes() {
        return Arrays.asList(
                RuleType.THRESHOLD,
                RuleType.BLACKLIST,
                RuleType.PORT_SCAN,
                RuleType.BRUTE_FORCE,
                RuleType.DATA_EXFIL,
                RuleType.ANOMALY,
                RuleType.PROTOCOL_ANOMALY,
                RuleType.TLS_FINGERPRINT);
    }

    /**
     * 初始化所有匹配器。
     */
    public synchronized void initialize() {
        initialize(allDefaultTypes());
    }

    /**
     * 按配置装配匹配器子集:只注册 enabledTypes 中的类型。线程安全;
     * 传 null 视为空集合(不注册任何匹配器)。
     *
     * @param enabledTypes 作业/租户配置允许的规则类型集合(可为 null)
     */
    public synchronized void initialize(Collection<RuleType> enabledTypes) {
        matchers = new HashMap<>();
        int requested = enabledTypes == null ? 0 : enabledTypes.size();
        if (enabledTypes != null) {
            for (RuleType type : enabledTypes) {
                RuleMatcher matcher = createMatcher(type);
                if (matcher != null) {
                    matchers.put(type, matcher);
                }
            }
        }
        registeredTypes = new ArrayList<>(matchers.keySet());
        warnUnregisteredEnumTypes();
        LOG.info("MatcherFactory initialized with {}/{} requested matchers", matchers.size(), requested);
    }

    /**
     * 枚举/注册表漂移告警:RuleType 枚举中未装配的类型会被静默跳过,
     * 这里显式告警,避免新增枚举值后遗忘注册。
     */
    private void warnUnregisteredEnumTypes() {
        for (RuleType type : RuleType.values()) {
            if (!matchers.containsKey(type)) {
                LOG.warn("RuleType {} has no registered matcher; rules of this type will not match", type);
            }
        }
    }

    /**
     * 单一构造点:类型 → 具体策略实例。新增检测类型只修改本方法。
     */
    private static RuleMatcher createMatcher(RuleType type) {
        switch (type) {
            case THRESHOLD:
                return new ThresholdMatcher();
            case BLACKLIST:
                return new BlacklistMatcher();
            case PORT_SCAN:
                return new PortScanMatcher();
            case BRUTE_FORCE:
                return new BruteForceMatcher();
            case DATA_EXFIL:
                return new DataExfilMatcher();
            case ANOMALY:
                return new AnomalyMatcher();
            case PROTOCOL_ANOMALY:
                return new ProtocolAnomalyMatcher();
            case TLS_FINGERPRINT:
                return new TlsFingerprintMatcher();
            default:
                LOG.warn("Unknown rule type {}, matcher not registered", type);
                return null;
        }
    }

    /**
     * 获取指定类型的匹配器(未注册类型返回 null)。线程安全;懒初始化。
     */
    public synchronized RuleMatcher getMatcher(RuleType type) {
        if (matchers == null) {
            initialize();
        }
        return matchers.get(type);
    }

    /**
     * 当前已注册的匹配器类型(不可变快照,可观测)。
     */
    public synchronized List<RuleType> registeredTypes() {
        return registeredTypes == null
                ? Collections.emptyList()
                : Collections.unmodifiableList(new ArrayList<>(registeredTypes));
    }

    /**
     * 清理资源(matchers 置 null 与懒初始化判定一致,registeredTypes 同步清空)。
     */
    public synchronized void close() {
        matchers = null;
        registeredTypes = null;
    }
}
