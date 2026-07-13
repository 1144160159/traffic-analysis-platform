// rust/probe-agent/probe-agent/ebpf/src/main.rs
#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::xdp_action,
    macros::{map, xdp},
    maps::XskMap,
    programs::XdpContext,
};

/// XSK Map - 用于重定向到 AF_XDP Socket
#[map]
static XSKS_MAP: XskMap = XskMap::with_max_entries(64, 0);

/// XDP 程序入口
#[xdp]
pub fn xdp_redirect(ctx: XdpContext) -> u32 {
    match try_xdp_redirect(&ctx) {
        Ok(action) => action,
        Err(_) => xdp_action::XDP_PASS,
    }
}

/// XDP 处理逻辑
#[inline(always)]
fn try_xdp_redirect(ctx: &XdpContext) -> Result<u32, ()> {
    // 获取 RX 队列 ID
    let queue_id = unsafe { (*ctx.ctx).rx_queue_index };

    // 尝试重定向到 XSK
    match XSKS_MAP.redirect(queue_id, 0) {
        Ok(_) => Ok(xdp_action::XDP_REDIRECT),
        Err(_) => {
            // 如果没有对应的 XSK，放行
            Ok(xdp_action::XDP_PASS)
        }
    }
}

/// Panic handler
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe { core::hint::unreachable_unchecked() }
}
