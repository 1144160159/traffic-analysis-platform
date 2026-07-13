package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.io.Serializable;
import java.util.Optional;

/**
 * 规则匹配器接口
 */
public interface RuleMatcher extends Serializable {
    
    /**
     * 匹配特征与规则
     *
     * @param feature 特征数据
     * @param rule    规则
     * @param context 匹配上下文
     * @return 匹配结果，如果不匹配返回 Optional.empty()
     */
    Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context);

    /**
     * 初始化匹配器
     */
    default void initialize() {
        // 默认空实现
    }

    /**
     * 关闭匹配器
     */
    default void close() {
        // 默认空实现
    }
}