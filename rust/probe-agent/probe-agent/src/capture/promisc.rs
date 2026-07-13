use anyhow::{Context, Result};
use std::os::unix::io::RawFd;
use tracing::{debug, warn};

pub struct PromiscuousMode {
    interface: String,
    socket_fd: RawFd,
    original_flags: i32,
}

impl PromiscuousMode {
    pub fn enable(interface: &str) -> Result<Self> {
        let socket_fd = unsafe { libc::socket(libc::AF_INET, libc::SOCK_DGRAM, 0) };

        if socket_fd < 0 {
            anyhow::bail!(
                "Failed to create socket for ioctl: {}",
                std::io::Error::last_os_error()
            );
        }

        let original_flags = get_flags(socket_fd, interface)?;

        if (original_flags & libc::IFF_PROMISC) != 0 {
            debug!("Interface {} already in promiscuous mode", interface);
            return Ok(Self {
                interface: interface.to_string(),
                socket_fd,
                original_flags,
            });
        }

        let new_flags = original_flags | libc::IFF_PROMISC;
        set_flags(socket_fd, interface, new_flags).context("Failed to enable promiscuous mode")?;

        debug!(
            "Promiscuous mode enabled on {}: flags 0x{:x} -> 0x{:x}",
            interface, original_flags, new_flags
        );

        Ok(Self {
            interface: interface.to_string(),
            socket_fd,
            original_flags,
        })
    }

    fn restore(&mut self) -> Result<()> {
        let current_flags = get_flags(self.socket_fd, &self.interface)?;

        if current_flags == self.original_flags {
            debug!("Interface {} flags already restored", self.interface);
            return Ok(());
        }

        set_flags(self.socket_fd, &self.interface, self.original_flags)
            .context("Failed to restore original flags")?;

        debug!(
            "Promiscuous mode restored on {}: flags 0x{:x} -> 0x{:x}",
            self.interface, current_flags, self.original_flags
        );

        Ok(())
    }
}

impl Drop for PromiscuousMode {
    fn drop(&mut self) {
        if let Err(e) = self.restore() {
            warn!(
                "Failed to restore promiscuous mode on {}: {}",
                self.interface, e
            );
        }

        if self.socket_fd >= 0 {
            unsafe {
                libc::close(self.socket_fd);
            }
        }
    }
}

unsafe impl Send for PromiscuousMode {}
unsafe impl Sync for PromiscuousMode {}

pub fn set_promiscuous_mode(interface: &str, enable: bool) -> Result<()> {
    let socket_fd = unsafe { libc::socket(libc::AF_INET, libc::SOCK_DGRAM, 0) };

    if socket_fd < 0 {
        anyhow::bail!(
            "Failed to create socket for ioctl: {}",
            std::io::Error::last_os_error()
        );
    }

    let _socket_guard = SocketGuard(socket_fd);

    let current_flags = get_flags(socket_fd, interface)?;

    let new_flags = if enable {
        current_flags | libc::IFF_PROMISC
    } else {
        current_flags & !libc::IFF_PROMISC
    };

    if new_flags == current_flags {
        debug!(
            "Interface {} already in {} mode",
            interface,
            if enable { "promiscuous" } else { "normal" }
        );
        return Ok(());
    }

    set_flags(socket_fd, interface, new_flags)?;

    debug!(
        "Promiscuous mode {} on {}: flags 0x{:x} -> 0x{:x}",
        if enable { "enabled" } else { "disabled" },
        interface,
        current_flags,
        new_flags
    );

    Ok(())
}

pub fn get_promiscuous_mode(interface: &str) -> Result<bool> {
    let socket_fd = unsafe { libc::socket(libc::AF_INET, libc::SOCK_DGRAM, 0) };

    if socket_fd < 0 {
        anyhow::bail!(
            "Failed to create socket for ioctl: {}",
            std::io::Error::last_os_error()
        );
    }

    let _socket_guard = SocketGuard(socket_fd);

    let flags = get_flags(socket_fd, interface)?;

    Ok((flags & libc::IFF_PROMISC) != 0)
}

fn get_flags(socket_fd: RawFd, interface: &str) -> Result<i32> {
    let mut ifreq = create_ifreq(interface)?;

    let ret = unsafe { libc::ioctl(socket_fd, libc::SIOCGIFFLAGS, &mut ifreq) };

    if ret < 0 {
        anyhow::bail!(
            "SIOCGIFFLAGS failed for {}: {}",
            interface,
            std::io::Error::last_os_error()
        );
    }

    let flags = unsafe { ifreq.ifr_ifru.ifru_flags };

    Ok(flags as i32)
}

fn set_flags(socket_fd: RawFd, interface: &str, flags: i32) -> Result<()> {
    let mut ifreq = create_ifreq(interface)?;

    unsafe {
        ifreq.ifr_ifru.ifru_flags = flags as i16;
    }

    let ret = unsafe { libc::ioctl(socket_fd, libc::SIOCSIFFLAGS, &ifreq) };

    if ret < 0 {
        let err = std::io::Error::last_os_error();

        if err.raw_os_error() == Some(libc::EPERM) {
            anyhow::bail!(
                "Permission denied: Cannot set flags on {}. \
                 Run with root or grant CAP_NET_ADMIN capability: \
                 sudo setcap cap_net_admin+ep <binary>",
                interface
            );
        }

        anyhow::bail!("SIOCSIFFLAGS failed for {}: {}", interface, err);
    }

    Ok(())
}

fn create_ifreq(interface: &str) -> Result<libc::ifreq> {
    if interface.len() >= libc::IFNAMSIZ {
        anyhow::bail!(
            "Interface name too long: {} (max {} chars)",
            interface,
            libc::IFNAMSIZ - 1
        );
    }

    let mut ifreq: libc::ifreq = unsafe { std::mem::zeroed() };

    let name_bytes = interface.as_bytes();
    let dest = unsafe {
        std::slice::from_raw_parts_mut(ifreq.ifr_name.as_mut_ptr() as *mut u8, libc::IFNAMSIZ)
    };
    dest[..name_bytes.len()].copy_from_slice(name_bytes);

    Ok(ifreq)
}

struct SocketGuard(RawFd);

impl Drop for SocketGuard {
    fn drop(&mut self) {
        if self.0 >= 0 {
            unsafe {
                libc::close(self.0);
            }
        }
    }
}
