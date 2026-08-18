use probe_agent::capture::{
    CaptureTimestamp, CaptureTimestampProvenance, TimestampPrecision, Umem, XdpCapture, XdpDesc,
};

#[test]
fn m02_xdp_timestamp_matrix() {
    let umem = Umem::from_buffer(vec![0; 4096]);
    let descriptors = [
        XdpDesc {
            addr: 0,
            len: 64,
            options: 0,
        },
        XdpDesc {
            addr: 2048,
            len: 128,
            options: 0,
        },
    ];
    let kernel = CaptureTimestamp::from_unix_parts(
        1_700_000_000,
        123_456_000,
        TimestampPrecision::Nanosecond,
    )
    .unwrap();
    let kernel = CaptureTimestamp::from_epoch_micros(
        kernel.epoch_micros(),
        CaptureTimestampProvenance::KernelPerFrame,
    );
    let receipt = CaptureTimestamp::from_epoch_micros(
        1_700_000_000_999_999,
        CaptureTimestampProvenance::DescriptorWallClockDegraded,
    );
    let frames =
        XdpCapture::consume_rx_with_timestamps(&umem, &descriptors, &[Some(kernel), None], receipt)
            .unwrap();

    assert_eq!(
        frames[0].captured_at.provenance,
        CaptureTimestampProvenance::KernelPerFrame
    );
    assert_eq!(frames[0].captured_at.epoch_micros(), kernel.epoch_micros());
    assert_eq!(
        frames[1].captured_at.provenance,
        CaptureTimestampProvenance::DescriptorWallClockDegraded
    );
    assert_eq!(frames[1].captured_at.epoch_micros(), receipt.epoch_micros());
    assert!(XdpCapture::consume_rx_with_timestamps(&umem, &descriptors, &[None], receipt).is_err());
}
