package com.traffic.flink.pcap.process;

import java.io.Serializable;
import java.util.Collections;
import java.util.List;

/** Immutable, deterministic result of pure PCAP manifest validation. */
public final class PcapManifestValidation implements Serializable {
    private static final long serialVersionUID = 1L;

    public enum Disposition { COMPLETE_V2, LEGACY_V1_COMPATIBLE, REJECTED }

    private final Disposition disposition;
    private final List<String> reasons;
    private final String projectionIdentity;

    PcapManifestValidation(Disposition disposition, List<String> reasons, String projectionIdentity) {
        this.disposition = disposition;
        this.reasons = Collections.unmodifiableList(List.copyOf(reasons));
        this.projectionIdentity = projectionIdentity;
    }

    public Disposition getDisposition() { return disposition; }
    public List<String> getReasons() { return reasons; }
    public String getProjectionIdentity() { return projectionIdentity; }
    public boolean isAccepted() { return disposition != Disposition.REJECTED; }
}
