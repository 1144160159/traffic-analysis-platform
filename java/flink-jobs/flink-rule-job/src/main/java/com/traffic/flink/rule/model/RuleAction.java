package com.traffic.flink.rule.model;

import com.fasterxml.jackson.annotation.JsonValue;

/**
 * 规则动作枚举
 */
public enum RuleAction {
    
    /**
     * 更新/创建规则
     */
    UPDATE("update"),

    /**
     * 删除规则
     */
    DELETE("delete"),

    /**
     * 启用规则
     */
    ENABLE("enable"),

    /**
     * 禁用规则
     */
    DISABLE("disable"),

    /**
     * 同步规则（批量）
     */
    SYNC("sync");

    private final String value;

    RuleAction(String value) {
        this.value = value;
    }

    @JsonValue
    public String getValue() {
        return value;
    }

    public static RuleAction fromValue(String value) {
        for (RuleAction action : values()) {
            if (action.value.equalsIgnoreCase(value)) {
                return action;
            }
        }
        return UPDATE; // 默认更新
    }
}