package com.traffic.flink.behavior.receipt;

import java.io.Serializable;

/**
 * RunDetectionResultRow —— run-scoped 每输入×detector typed disposition 结果行
 * (§7.4/76.44:DetectorDisposition 只属于输入×required detector;每输入恰一行)。
 * 落库 traffic.analysis_detections(调度域 per-input 结果权威存储)。
 */
public final class RunDetectionResultRow implements Serializable {

    private static final long serialVersionUID = 1L;

    public String tenantId;
    public String runId;
    public String executionSpecSha256;
    public String inputIdentity;
    public String detectorId;
    public String disposition;
    public double score;
    public String labels;
    public String evidenceRefs;
    public long tsMs;

    public RunDetectionResultRow() {}

    public RunDetectionResultRow(String tenantId, String runId, String executionSpecSha256,
                                 String inputIdentity, String detectorId, String disposition,
                                 double score, String labels, String evidenceRefs, long tsMs) {
        this.tenantId = tenantId;
        this.runId = runId;
        this.executionSpecSha256 = executionSpecSha256;
        this.inputIdentity = inputIdentity;
        this.detectorId = detectorId;
        this.disposition = disposition;
        this.score = score;
        this.labels = labels;
        this.evidenceRefs = evidenceRefs;
        this.tsMs = tsMs;
    }
}
