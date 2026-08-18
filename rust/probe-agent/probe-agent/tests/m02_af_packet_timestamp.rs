use libc::{c_void, cmsghdr, mmsghdr, msghdr, timespec, SCM_TIMESTAMPNS, SOL_SOCKET};
use probe_agent::capture::{
    AfPacketCapture, CaptureTimestamp, CaptureTimestampProvenance, TimestampPrecision,
};

fn empty_message() -> mmsghdr {
    mmsghdr {
        msg_hdr: msghdr {
            msg_name: std::ptr::null_mut(),
            msg_namelen: 0,
            msg_iov: std::ptr::null_mut(),
            msg_iovlen: 0,
            msg_control: std::ptr::null_mut(),
            msg_controllen: 0,
            msg_flags: 0,
        },
        msg_len: 0,
    }
}

#[test]
fn m02_af_packet_timestamp_matrix() {
    let degraded = CaptureTimestamp::from_epoch_micros(
        1_700_000_000_999_999,
        CaptureTimestampProvenance::DescriptorWallClockDegraded,
    );
    let missing = AfPacketCapture::recv_frames_with_timestamps(&[empty_message()], degraded);
    assert_eq!(missing.len(), 1);
    assert_eq!(
        missing[0].provenance,
        CaptureTimestampProvenance::DescriptorWallClockDegraded
    );

    let bytes = unsafe { libc::CMSG_SPACE(std::mem::size_of::<timespec>() as u32) as usize };
    let mut control = vec![0usize; bytes.div_ceil(std::mem::size_of::<usize>())];
    let mut message = empty_message();
    message.msg_hdr.msg_control = control.as_mut_ptr().cast::<c_void>();
    message.msg_hdr.msg_controllen = control.len() * std::mem::size_of::<usize>();
    unsafe {
        let header = message.msg_hdr.msg_control.cast::<cmsghdr>();
        (*header).cmsg_level = SOL_SOCKET;
        (*header).cmsg_type = SCM_TIMESTAMPNS;
        (*header).cmsg_len = libc::CMSG_LEN(std::mem::size_of::<timespec>() as u32) as usize;
        std::ptr::write_unaligned(
            libc::CMSG_DATA(header).cast::<timespec>(),
            timespec {
                tv_sec: 1_700_000_000,
                tv_nsec: 123_456_789,
            },
        );
    }

    let kernel = AfPacketCapture::recv_frames_with_timestamps(&[message], degraded);
    assert_eq!(kernel.len(), 1);
    assert_eq!(
        kernel[0].provenance,
        CaptureTimestampProvenance::KernelPerFrame
    );
    assert_eq!(kernel[0].source_precision, TimestampPrecision::Nanosecond);
    assert_eq!(kernel[0].precision_loss_nanos, 789);
    assert_eq!(kernel[0].epoch_micros(), 1_700_000_000_123_456);
}
