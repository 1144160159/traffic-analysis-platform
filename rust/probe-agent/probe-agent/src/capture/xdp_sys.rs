// XDP 系统调用相关的常量和结构体定义

use std::os::unix::io::RawFd;

// SOL_XDP socket option level
pub const SOL_XDP: i32 = 283;

// XDP socket options
pub const XDP_MMAP_OFFSETS: i32 = 1;
pub const XDP_RX_RING: i32 = 2;
pub const XDP_TX_RING: i32 = 3;
pub const XDP_UMEM_REG: i32 = 4;
pub const XDP_UMEM_FILL_RING: i32 = 5;
pub const XDP_UMEM_COMPLETION_RING: i32 = 6;
pub const XDP_STATISTICS: i32 = 7;
pub const XDP_OPTIONS: i32 = 8;

// XDP mmap offsets for rings
pub const XDP_PGOFF_RX_RING: u64 = 0;
pub const XDP_PGOFF_TX_RING: u64 = 0x80000000;
pub const XDP_UMEM_PGOFF_FILL_RING: u64 = 0x100000000;
pub const XDP_UMEM_PGOFF_COMPLETION_RING: u64 = 0x180000000;

// XDP bind flags
pub const XDP_SHARED_UMEM: u16 = 1 << 0;
pub const XDP_COPY: u16 = 1 << 1;
pub const XDP_ZEROCOPY: u16 = 1 << 2;
pub const XDP_USE_NEED_WAKEUP: u16 = 1 << 3;

// XDP ring flags
pub const XDP_RING_NEED_WAKEUP: u32 = 1 << 0;

/// XDP ring offset structure (per ring)
#[repr(C)]
#[derive(Debug, Default, Clone, Copy)]
pub struct XdpRingOffset {
    pub producer: u64,
    pub consumer: u64,
    pub desc: u64,
    pub flags: u64,
}

/// XDP mmap offsets structure (all rings)
#[repr(C)]
#[derive(Debug, Default, Clone, Copy)]
pub struct XdpMmapOffsets {
    pub rx: XdpRingOffset,
    pub tx: XdpRingOffset,
    pub fr: XdpRingOffset, // fill ring
    pub cr: XdpRingOffset, // completion ring
}

/// XDP UMEM registration structure
#[repr(C)]
#[derive(Debug, Default, Clone, Copy)]
pub struct XdpUmemReg {
    pub addr: u64,
    pub len: u64,
    pub chunk_size: u32,
    pub headroom: u32,
    pub flags: u32,
}

/// XDP statistics
#[repr(C)]
#[derive(Debug, Default, Clone, Copy)]
pub struct XdpStatistics {
    pub rx_dropped: u64,
    pub rx_invalid_descs: u64,
    pub tx_invalid_descs: u64,
    pub rx_ring_full: u64,
    pub rx_fill_ring_empty_descs: u64,
    pub tx_ring_empty_descs: u64,
}

/// XDP options
#[repr(C)]
#[derive(Debug, Default, Clone, Copy)]
pub struct XdpOptions {
    pub flags: u32,
}

/// sockaddr_xdp structure for AF_XDP sockets
#[repr(C)]
#[derive(Debug, Default, Clone, Copy)]
pub struct SockaddrXdp {
    pub sxdp_family: u16,
    pub sxdp_flags: u16,
    pub sxdp_ifindex: u32,
    pub sxdp_queue_id: u32,
    pub sxdp_shared_umem_fd: u32,
}

impl XdpMmapOffsets {
    /// Get mmap offsets from socket
    pub fn get(fd: RawFd) -> std::io::Result<Self> {
        let mut offsets = Self::default();
        let mut optlen = std::mem::size_of::<Self>() as libc::socklen_t;

        let ret = unsafe {
            libc::getsockopt(
                fd,
                SOL_XDP,
                XDP_MMAP_OFFSETS,
                &mut offsets as *mut _ as *mut libc::c_void,
                &mut optlen,
            )
        };

        if ret < 0 {
            return Err(std::io::Error::last_os_error());
        }

        Ok(offsets)
    }
}

impl XdpStatistics {
    /// Get XDP statistics from socket
    pub fn get(fd: RawFd) -> std::io::Result<Self> {
        let mut stats = Self::default();
        let mut optlen = std::mem::size_of::<Self>() as libc::socklen_t;

        let ret = unsafe {
            libc::getsockopt(
                fd,
                SOL_XDP,
                XDP_STATISTICS,
                &mut stats as *mut _ as *mut libc::c_void,
                &mut optlen,
            )
        };

        if ret < 0 {
            return Err(std::io::Error::last_os_error());
        }

        Ok(stats)
    }
}

/// Helper to set socket option for ring size
pub fn set_ring_size(fd: RawFd, opt: i32, size: u32) -> std::io::Result<()> {
    let ret = unsafe {
        libc::setsockopt(
            fd,
            SOL_XDP,
            opt,
            &size as *const _ as *const libc::c_void,
            std::mem::size_of::<u32>() as libc::socklen_t,
        )
    };

    if ret < 0 {
        return Err(std::io::Error::last_os_error());
    }

    Ok(())
}

/// Helper to register UMEM
pub fn register_umem(fd: RawFd, umem_reg: &XdpUmemReg) -> std::io::Result<()> {
    let ret = unsafe {
        libc::setsockopt(
            fd,
            SOL_XDP,
            XDP_UMEM_REG,
            umem_reg as *const _ as *const libc::c_void,
            std::mem::size_of::<XdpUmemReg>() as libc::socklen_t,
        )
    };

    if ret < 0 {
        return Err(std::io::Error::last_os_error());
    }

    Ok(())
}

/// Mmap a ring buffer
///
/// # Safety
/// The caller must ensure the fd is valid and the offset/size are correct.
pub unsafe fn mmap_ring(fd: RawFd, offset: u64, size: usize) -> std::io::Result<*mut u8> {
    let ptr = libc::mmap(
        std::ptr::null_mut(),
        size,
        libc::PROT_READ | libc::PROT_WRITE,
        libc::MAP_SHARED | libc::MAP_POPULATE,
        fd,
        offset as libc::off_t,
    );

    if ptr == libc::MAP_FAILED {
        return Err(std::io::Error::last_os_error());
    }

    Ok(ptr as *mut u8)
}

/// Unmap a ring buffer
///
/// # Safety
/// The caller must ensure ptr and size are valid.
pub unsafe fn munmap_ring(ptr: *mut u8, size: usize) -> std::io::Result<()> {
    let ret = libc::munmap(ptr as *mut libc::c_void, size);
    if ret < 0 {
        return Err(std::io::Error::last_os_error());
    }
    Ok(())
}

/// Calculate the size needed for mmap of a ring
pub fn ring_mmap_size(ring_offset: &XdpRingOffset, ring_size: u32, entry_size: usize) -> usize {
    // The ring needs space for:
    // - producer/consumer/flags (at their offsets)
    // - descriptor array (ring_size * entry_size)
    let desc_end = ring_offset.desc as usize + (ring_size as usize * entry_size);

    // Round up to page size
    let page_size = unsafe { libc::sysconf(libc::_SC_PAGESIZE) as usize };
    (desc_end + page_size - 1) & !(page_size - 1)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_struct_sizes() {
        // Verify struct sizes match kernel expectations
        assert_eq!(std::mem::size_of::<XdpRingOffset>(), 32);
        assert_eq!(std::mem::size_of::<XdpMmapOffsets>(), 128);
        assert_eq!(std::mem::size_of::<XdpUmemReg>(), 32);
        assert_eq!(std::mem::size_of::<SockaddrXdp>(), 16);
    }

    #[test]
    fn test_ring_mmap_size() {
        let offset = XdpRingOffset {
            producer: 0,
            consumer: 4,
            desc: 64,
            flags: 8,
        };

        // For a ring with 2048 entries of 8 bytes each
        let size = ring_mmap_size(&offset, 2048, 8);
        assert!(size >= 64 + 2048 * 8);
        // Should be page-aligned
        let page_size = unsafe { libc::sysconf(libc::_SC_PAGESIZE) as usize };
        assert_eq!(size % page_size, 0);
    }
}
