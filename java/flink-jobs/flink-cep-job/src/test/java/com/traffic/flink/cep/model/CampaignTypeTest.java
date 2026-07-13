////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/model/CampaignTypeTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.model;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * CampaignType 枚举单元测试
 */
class CampaignTypeTest {

    @Test
    @DisplayName("验证所有战役类型枚举值")
    void testAllTypes() {
        assertThat(CampaignType.values()).hasSize(7);
    }

    @Test
    @DisplayName("验证战役类型代码")
    void testTypeCodes() {
        assertThat(CampaignType.SCAN_EXPLOIT.getCode()).isEqualTo("scan_exploit");
        assertThat(CampaignType.BRUTE_FORCE.getCode()).isEqualTo("brute_force");
        assertThat(CampaignType.LATERAL_MOVEMENT.getCode()).isEqualTo("lateral_movement");
        assertThat(CampaignType.DATA_EXFILTRATION.getCode()).isEqualTo("data_exfiltration");
        assertThat(CampaignType.C2_COMMUNICATION.getCode()).isEqualTo("c2_communication");
        assertThat(CampaignType.PRIVILEGE_ESCALATION.getCode()).isEqualTo("privilege_escalation");
        assertThat(CampaignType.PERSISTENCE.getCode()).isEqualTo("persistence");
    }

    @Test
    @DisplayName("验证战役类型描述")
    void testTypeDescriptions() {
        assertThat(CampaignType.SCAN_EXPLOIT.getDescription()).isEqualTo("扫描-利用攻击链");
        assertThat(CampaignType.BRUTE_FORCE.getDescription()).isEqualTo("暴力破解攻击");
        assertThat(CampaignType.LATERAL_MOVEMENT.getDescription()).isEqualTo("横向移动");
        assertThat(CampaignType.DATA_EXFILTRATION.getDescription()).isEqualTo("数据外泄");
        assertThat(CampaignType.C2_COMMUNICATION.getDescription()).isEqualTo("C2通信");
    }
}