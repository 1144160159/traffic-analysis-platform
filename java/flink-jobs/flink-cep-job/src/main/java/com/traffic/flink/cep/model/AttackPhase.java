package com.traffic.flink.cep.model;

/**
 * ATT&CK 攻击阶段
 */
public enum AttackPhase {
    
    RECONNAISSANCE("reconnaissance", "侦察"),
    RESOURCE_DEVELOPMENT("resource_development", "资源开发"),
    INITIAL_ACCESS("initial_access", "初始访问"),
    EXECUTION("execution", "执行"),
    PERSISTENCE("persistence", "持久化"),
    PRIVILEGE_ESCALATION("privilege_escalation", "权限提升"),
    DEFENSE_EVASION("defense_evasion", "防御规避"),
    CREDENTIAL_ACCESS("credential_access", "凭据访问"),
    DISCOVERY("discovery", "发现"),
    LATERAL_MOVEMENT("lateral_movement", "横向移动"),
    COLLECTION("collection", "收集"),
    COMMAND_AND_CONTROL("command_and_control", "命令与控制"),
    EXFILTRATION("exfiltration", "数据外泄"),
    IMPACT("impact", "影响");

    private final String code;
    private final String description;

    AttackPhase(String code, String description) {
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