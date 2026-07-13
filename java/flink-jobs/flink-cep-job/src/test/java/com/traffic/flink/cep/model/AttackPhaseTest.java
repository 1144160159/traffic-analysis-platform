////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/model/AttackPhaseTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.model;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * AttackPhase 枚举单元测试
 */
class AttackPhaseTest {

    @Test
    @DisplayName("验证所有攻击阶段枚举值")
    void testAllPhases() {
        assertThat(AttackPhase.values()).hasSize(14);
    }

    @Test
    @DisplayName("验证 ATT&CK 阶段代码")
    void testPhaseCodes() {
        assertThat(AttackPhase.RECONNAISSANCE.getCode()).isEqualTo("reconnaissance");
        assertThat(AttackPhase.INITIAL_ACCESS.getCode()).isEqualTo("initial_access");
        assertThat(AttackPhase.EXECUTION.getCode()).isEqualTo("execution");
        assertThat(AttackPhase.PERSISTENCE.getCode()).isEqualTo("persistence");
        assertThat(AttackPhase.PRIVILEGE_ESCALATION.getCode()).isEqualTo("privilege_escalation");
        assertThat(AttackPhase.DEFENSE_EVASION.getCode()).isEqualTo("defense_evasion");
        assertThat(AttackPhase.CREDENTIAL_ACCESS.getCode()).isEqualTo("credential_access");
        assertThat(AttackPhase.DISCOVERY.getCode()).isEqualTo("discovery");
        assertThat(AttackPhase.LATERAL_MOVEMENT.getCode()).isEqualTo("lateral_movement");
        assertThat(AttackPhase.COLLECTION.getCode()).isEqualTo("collection");
        assertThat(AttackPhase.COMMAND_AND_CONTROL.getCode()).isEqualTo("command_and_control");
        assertThat(AttackPhase.EXFILTRATION.getCode()).isEqualTo("exfiltration");
        assertThat(AttackPhase.IMPACT.getCode()).isEqualTo("impact");
    }

    @Test
    @DisplayName("验证阶段描述")
    void testPhaseDescriptions() {
        assertThat(AttackPhase.RECONNAISSANCE.getDescription()).isEqualTo("侦察");
        assertThat(AttackPhase.LATERAL_MOVEMENT.getDescription()).isEqualTo("横向移动");
        assertThat(AttackPhase.EXFILTRATION.getDescription()).isEqualTo("数据外泄");
    }
}