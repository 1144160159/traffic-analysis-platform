use probe_agent::metrics;

#[test]
fn capture_counter_authority_matrix() {
    let before = metrics::capture_snapshot();

    // One production batch commit must retain its packet cardinality. Capture
    // drivers do not commit these counters, so this cannot be doubled there.
    metrics::inc_capture_local(64, 64 * 128);
    metrics::inc_capture_allocation_drop_local(3);
    metrics::inc_capture_kernel_drop_local(5);
    metrics::inc_capture_error_local();

    let after = metrics::capture_snapshot();
    assert_eq!(after.capture_packets - before.capture_packets, 64);
    assert_eq!(after.capture_bytes - before.capture_bytes, 64 * 128);
    assert_eq!(
        after.capture_allocation_drops - before.capture_allocation_drops,
        3
    );
    assert_eq!(after.capture_kernel_drops - before.capture_kernel_drops, 5);
    assert_eq!(after.capture_errors - before.capture_errors, 1);
}
