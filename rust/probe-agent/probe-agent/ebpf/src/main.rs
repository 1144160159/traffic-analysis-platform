// rust/probe-agent/probe-agent/ebpf/src/main.rs
#![no_std]
#![no_main]

use aya_bpf::{
    bindings::xdp_action,
    macros::{map, xdp},
    maps::XskMap,
    programs::XdpContext,
};

/// XDP Socket Map
/// 用于将数据包重定向到 AF_XDP Socket
#[map]
static XSKS_MAP: XskMap = XskMap::with_max_entries(64, 0);

/// XDP 程序入口点
/// 
/// 逻辑：
/// 1. 获取当前 RX 队列 ID
/// 2. 将数据包重定向到对应的 XSK Socket
/// 3. 如果重定向失败，放行数据包
#[xdp]
pub fn xdp_redirect(ctx: XdpContext) -> u32 {
    match try_xdp_redirect(ctx) {
        Ok(ret) => ret,
        Err(_) => xdp_action::XDP_PASS,
    }
}

fn try_xdp_redirect(ctx: XdpContext) -> Result<u32, ()> {
    // 获取 RX 队列索引
    let queue_id = unsafe { (*ctx.ctx).rx_queue_index };
    
    // 重定向到 XSK Socket
    // 如果成功，返回 XDP_REDIRECT
    // 如果 Map 中没有对应的 Socket，返回 XDP_PASS
    match XSKS_MAP.redirect(queue_id, 0) {
        Ok(_) => Ok(xdp_action::XDP_REDIRECT),
        Err(_) => Ok(xdp_action::XDP_PASS),
    }
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe { core::hint::unreachable_unchecked() }
}