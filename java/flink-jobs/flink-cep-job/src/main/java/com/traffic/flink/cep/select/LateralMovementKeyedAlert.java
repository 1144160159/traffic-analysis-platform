package com.traffic.flink.cep.select;

import com.traffic.proto.traffic.v1.Alert;

import java.io.Serializable;
import java.util.Objects;

/**
 * 横向移动 CEP 输入包装：key = tenant + 主机 IP（src 或 dst 扇出键），
 * alert = 原始告警。
 *
 * 每条告警由 CepJob 扇出到 srcIp 与 dstIp 两个 key，使跳板主机 B
 * （compromise.dst=B 与 internal.src=B）在同一条 CEP 分区中相遇，
 * 修复原实现按单一 (tenant,srcIp) key 导致跨主机链永不匹配的问题。
 */
public final class LateralMovementKeyedAlert implements Serializable {

    private static final long serialVersionUID = 1L;

    private final String partitionKey;
    private final Alert alert;

    public LateralMovementKeyedAlert(String partitionKey, Alert alert) {
        if (partitionKey == null || partitionKey.isBlank()) {
            throw new IllegalArgumentException("lateral movement partition key must not be blank");
        }
        if (alert == null) {
            throw new IllegalArgumentException("lateral movement alert must not be null");
        }
        this.partitionKey = partitionKey;
        this.alert = alert;
    }

    public String getPartitionKey() {
        return partitionKey;
    }

    public Alert getAlert() {
        return alert;
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        LateralMovementKeyedAlert that = (LateralMovementKeyedAlert) o;
        return partitionKey.equals(that.partitionKey) && alert.getAlertId().equals(that.alert.getAlertId());
    }

    @Override
    public int hashCode() {
        return Objects.hash(partitionKey, alert.getAlertId());
    }

    @Override
    public String toString() {
        return "LateralMovementKeyedAlert{key=" + partitionKey + ", alert=" + alert.getAlertId() + "}";
    }
}
