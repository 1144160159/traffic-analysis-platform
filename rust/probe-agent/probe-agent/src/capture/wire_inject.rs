//! WireInjector —— 测试阶段 wire 回放执行臂:经 AF_PACKET 原始帧注入指定接口
//! (veth 对输入端),使冻结 pcap 样本以真实流量形式出现在输出端探针的实时采集面上。
//!
//! 语义:
//!  - 只做 L2 原始帧注入(无解析、无聚合、无检测)—— 分析有对应组件进行;
//!  - 接口经配置 allowlist 门禁(生产探针默认空列表 = 全部拒绝,fail-closed);
//!  - 打开接口/发送失败显式报错,绝不伪造成功;
//!  - 本执行臂仅服务测试/基准数据集生成,不进入生产采集链。
use std::ffi::CString;
use std::os::fd::RawFd;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use tracing::warn;

const SEND_TIMEOUT: Duration = Duration::from_secs(2);

/// 有界 wire 注入器:持有接口绑定的 AF_PACKET 原始套接字,逐帧发送完整 L2 帧。
pub struct WireInjector {
    interface: String,
    fd: RawFd,
    ifindex: i32,
}

impl WireInjector {
    /// 打开指定接口的注入通道。接口不存在/无权限(需 NET_RAW)时显式失败。
    pub fn open(interface: &str) -> Result<Self> {
        if interface.trim().is_empty() {
            bail!("wire inject interface is empty");
        }
        let ifname = CString::new(interface).context("interface name contains NUL byte")?;
        let ifindex = unsafe { libc::if_nametoindex(ifname.as_ptr()) };
        if ifindex == 0 {
            bail!("wire inject interface {interface} does not exist");
        }
        let ifindex = ifindex as i32;
        // SAFETY: socket() 常量调用;失败经 last_os_error 显式返回。
        let fd = unsafe {
            libc::socket(
                libc::AF_PACKET,
                libc::SOCK_RAW | libc::SOCK_CLOEXEC,
                libc::htons(libc::ETH_P_ALL as u16) as libc::c_int,
            )
        };
        if fd < 0 {
            bail!(
                "wire inject AF_PACKET socket on {interface}: {}",
                std::io::Error::last_os_error()
            );
        }
        // 发送超时上限:对端(veth 输出端)无人排空时不做无限阻塞,超时按错误记账。
        let timeout = libc::timeval {
            tv_sec: SEND_TIMEOUT.as_secs() as libc::time_t,
            tv_usec: SEND_TIMEOUT.subsec_micros() as libc::suseconds_t,
        };
        // SAFETY: SOL_SOCKET/SO_SNDTIMEO 常量;fd 已校验有效。
        let rc = unsafe {
            libc::setsockopt(
                fd,
                libc::SOL_SOCKET,
                libc::SO_SNDTIMEO,
                &timeout as *const libc::timeval as *const libc::c_void,
                std::mem::size_of::<libc::timeval>() as libc::socklen_t,
            )
        };
        if rc != 0 {
            let e = std::io::Error::last_os_error();
            unsafe { libc::close(fd) };
            bail!("wire inject setsockopt(SO_SNDTIMEO) on {interface}: {e}");
        }
        Ok(Self {
            interface: interface.to_string(),
            fd,
            ifindex,
        })
    }

    /// 注入一帧完整 L2 帧(pcap 记录原始字节,含以太头)。失败显式返回,由调用方记账。
    pub fn send_frame(&self, frame: &[u8]) -> Result<()> {
        if frame.is_empty() {
            bail!("refusing to inject empty frame on {}", self.interface);
        }
        // SAFETY: sockaddr_ll 为 POD;frame 指针与长度在调用期间有效;fd 已校验。
        let mut addr: libc::sockaddr_ll = unsafe { std::mem::zeroed() };
        addr.sll_family = libc::AF_PACKET as u16;
        addr.sll_protocol = libc::htons(libc::ETH_P_ALL as u16);
        addr.sll_ifindex = self.ifindex;
        let rc = unsafe {
            libc::sendto(
                self.fd,
                frame.as_ptr() as *const libc::c_void,
                frame.len(),
                0,
                &addr as *const libc::sockaddr_ll as *const libc::sockaddr,
                std::mem::size_of::<libc::sockaddr_ll>() as libc::socklen_t,
            )
        };
        if rc < 0 {
            bail!(
                "wire inject sendto on {}: {}",
                self.interface,
                std::io::Error::last_os_error()
            );
        }
        Ok(())
    }
}

impl Drop for WireInjector {
    fn drop(&mut self) {
        // SAFETY: fd 由 open 成功路径创建;重复 close 由打开失败的 bail 路径规避。
        unsafe { libc::close(self.fd) };
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_empty_or_missing_interface() {
        assert!(WireInjector::open("").is_err());
        assert!(WireInjector::open("no-such-iface-xyz").is_err());
    }

    /// lo 回环自检:打开注入器后在 lo 上注入一帧,经 AF_PACKET 接收端断言字节一致。
    /// 需要 NET_RAW(root);CI 无权限时忽略运行。
    #[test]
    #[ignore = "requires NET_RAW and a loopback interface"]
    fn injects_frame_on_loopback() {
        let injector = WireInjector::open("lo").expect("open injector on lo");
        let frame = [
            0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x08, 0x00,
            0x45, 0x00, 0x00, 0x1c, 0x00, 0x01, 0x00, 0x00, 0x40, 0xfd, 0x00, 0x00, 0x7f, 0x00,
            0x00, 0x01, 0x7f, 0x00, 0x00, 0x01, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x01,
            0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
        ];
        // 接收端:lo 上 AF_PACKET RX(阻塞,带超时)。
        let rx_fd = unsafe {
            libc::socket(
                libc::AF_PACKET,
                libc::SOCK_RAW | libc::SOCK_CLOEXEC,
                libc::htons(libc::ETH_P_ALL as u16) as libc::c_int,
            )
        };
        assert!(rx_fd >= 0, "open rx socket: {}", std::io::Error::last_os_error());
        let timeout = libc::timeval {
            tv_sec: 2,
            tv_usec: 0,
        };
        unsafe {
            libc::setsockopt(
                rx_fd,
                libc::SOL_SOCKET,
                libc::SO_RCVTIMEO,
                &timeout as *const libc::timeval as *const libc::c_void,
                std::mem::size_of::<libc::timeval>() as libc::socklen_t,
            );
        }
        injector.send_frame(&frame).expect("inject frame on lo");
        let mut buf = [0u8; 512];
        let n = unsafe { libc::recv(rx_fd, buf.as_mut_ptr() as *mut libc::c_void, buf.len(), 0) };
        unsafe { libc::close(rx_fd) };
        assert!(n >= 0, "recv: {}", std::io::Error::last_os_error());
        assert_eq!(n as usize, frame.len(), "lo must echo the exact injected frame");
        assert_eq!(&buf[..n as usize], &frame, "echoed frame bytes must match");
        warn!("loopback wire inject self-check passed ({} bytes)", n);
    }
}
