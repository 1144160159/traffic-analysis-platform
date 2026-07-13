package com.traffic.flink.cep.model;

/**
 * 战役类型枚举
 */
public enum CampaignType {
    
    /**
     * 扫描后利用
     */
    SCAN_EXPLOIT("scan_exploit", "扫描-利用攻击链"),

    /**
     * 暴力破解后成功
     */
    BRUTE_FORCE("brute_force", "暴力破解攻击"),

    /**
     * 横向移动
     */
    LATERAL_MOVEMENT("lateral_movement", "横向移动"),

    /**
     * 数据外泄
     */
    DATA_EXFILTRATION("data_exfiltration", "数据外泄"),

    /**
     * C2 通信
     */
    C2_COMMUNICATION("c2_communication", "C2通信"),

    /**
     * 权限提升
     */
    PRIVILEGE_ESCALATION("privilege_escalation", "权限提升"),

    /**
     * 持久化
     */
    PERSISTENCE("persistence", "持久化");

    private final String code;
    private final String description;

    CampaignType(String code, String description) {
        this.code = code;
        this.description = description;
    }

    public String getCode() {
        return code;
    }

    public String getDescription() {
        return description;
    }
}