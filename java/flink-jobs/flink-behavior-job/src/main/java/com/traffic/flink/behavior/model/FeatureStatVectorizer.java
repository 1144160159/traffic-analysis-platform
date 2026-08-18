package com.traffic.flink.behavior.model;

import com.traffic.proto.traffic.v1.FeatureStat;

import java.util.List;

/**
 * Ordered FeatureStat -> float vector mapping shared by governed runtimes.
 *
 * <p>The governed package signs the exact feature column order. Unknown
 * columns fail closed: substituting zero would make an incompatible package
 * look healthy during shadow evaluation.</p>
 */
public final class FeatureStatVectorizer {

    private FeatureStatVectorizer() {}

    public static float[] vectorize(FeatureStat feature, List<String> columns) {
        if (feature == null) {
            throw new IllegalArgumentException("feature is required");
        }
        if (columns == null || columns.isEmpty()) {
            throw new IllegalArgumentException("feature column contract is empty");
        }
        float[] values = new float[columns.size()];
        for (int index = 0; index < columns.size(); index++) {
            values[index] = value(feature, columns.get(index));
            if (!Float.isFinite(values[index])) {
                throw new IllegalArgumentException(
                        "feature column contains a non-finite value: " + columns.get(index));
            }
        }
        return values;
    }

    public static float value(FeatureStat feature, String column) {
        if (column == null || column.isBlank()) {
            throw new IllegalArgumentException("feature column name is empty");
        }
        switch (column) {
            case "pps": return feature.getPps();
            case "bps": return feature.getBps();
            case "up_down_ratio": return feature.getUpDownRatio();
            case "pktlen_mean": return feature.getPktlenMean();
            case "pktlen_std": return feature.getPktlenStd();
            case "iat_mean_ms": return feature.getIatMeanMs();
            case "iat_std_ms": return feature.getIatStdMs();
            case "active_mean_ms": return feature.getActiveMeanMs();
            case "idle_mean_ms": return feature.getIdleMeanMs();
            case "duration_ms": return feature.getDurationMs();
            case "tcp_flag_syn_cnt": return feature.getTcpFlagSynCnt();
            case "tcp_flag_ack_cnt": return feature.getTcpFlagAckCnt();
            case "tcp_init_win_bytes_fwd": return feature.getTcpInitWinBytesFwd();
            case "tcp_init_win_bytes_bwd": return feature.getTcpInitWinBytesBwd();
            case "protocol": return feature.getProtocol();
            default:
                throw new IllegalArgumentException(
                        "unsupported governed FeatureStat column: " + column);
        }
    }
}
