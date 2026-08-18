//! PcapReplayOperation —— 探针侧有界对象回放执行(人工执行链:回放发生在所选探针位置)。
//!
//! 执行语义:
//!  1. 从对象存储取回 pcap(s3://bucket/key,路径式访问);
//!  2. sha256 与冻结清单比对,不符拒绝(applied=false,不重试);
//!  3. 有界回放(packet/byte 双上限)并做时间线平移(首包对齐 window_start,保相对时序);
//!  4. 逐包喂入 feed 钩子(生产接线将接到共享分支流发射;测试用计数桩);
//!  5. 产出 fence + 计数(applied=true 仅在 feed 全程成功时)。
//!
//! wire 回放(命令携带 interface,测试阶段数据集生成):
//!  feed 换成 WireInjector(经 AF_PACKET 向 veth 输入端注入真实 L2 帧),
//!  时间线按 WIRE_REPLAY_TIME_COMPRESSION 压缩 + 帧间最小间隔兜底,供输出端
//!  探针实时采集;回执 watermark 采用真实墙钟(帧物理上线,非虚拟时间线)。
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use s3::creds::Credentials;
use s3::region::Region;
use s3::Bucket;
use sha2::{Digest, Sha256};
use tracing::{debug, warn};

use crate::capture::pcap_offline::{PcapReplayer, ReplaySpeed};
use crate::capture::Capturer;
use crate::control::{ReplayWindowCommand, ValidatedReplayWindow};

/// wire 回放时间压缩比:数据集常跨数小时而采集窗口仅数分钟,按该因子压缩
/// 原始时间线(保留相对突发结构)。
pub const WIRE_REPLAY_TIME_COMPRESSION: u64 = 120;
/// wire 回放帧间最小间隔:压缩后密集突发仍受此下限约束,避免 veth 输入队列溢出丢帧。
pub const WIRE_REPLAY_MIN_GAP_MICROS: u64 = 100;

/// 对象存储接入配置(与 archiver 同源:PROBE_S3_* / S3_ENDPOINT)。
#[derive(Debug, Clone)]
pub struct ReplayObjectStoreConfig {
    pub endpoint: String,
    pub region: String,
    pub access_key: String,
    pub secret_key: String,
    /// 对象缓存目录(临时 pcap 落盘,回放后清理)
    pub cache_dir: PathBuf,
}

/// 回放结果(applied=true 表示对象校验+有界回放+feed 全程成功)。
#[derive(Debug, Clone, serde::Serialize)]
pub struct ReplayOutcome {
    pub applied: bool,
    pub packets: u64,
    pub bytes_consumed: u64,
    pub flows_emitted: u64,
    pub watermark_ms: i64,
    pub fence: serde_json::Value,
    pub detail: String,
}

/// feed 钩子:逐包接收(包数据, 平移后时间戳微秒)。生产接线接共享分支流发射;测试用桩。
/// Send 约束:执行在 tokio 异步上下文,feed 闭包须可跨 await 持有。
pub type PacketFeed<'a> = &'a mut (dyn FnMut(&[u8], i64) -> Result<()> + Send);

/// 对象取回端口(生产走 S3;测试可注入内存实现)。
#[async_trait::async_trait]
pub trait ObjectFetcher: Send + Sync {
    async fn fetch(&self, object_ref: &str) -> Result<Vec<u8>>;
}

/// S3 取回实现(rust-s3,路径式访问;与 archiver 同接入)。
pub struct S3ObjectFetcher {
    config: ReplayObjectStoreConfig,
}

impl S3ObjectFetcher {
    pub fn new(config: ReplayObjectStoreConfig) -> Self {
        Self { config }
    }

    fn bucket_for(&self, object_ref: &str) -> Result<(Bucket, String)> {
        let rest = object_ref
            .strip_prefix("s3://")
            .context("object_ref must start with s3://")?;
        let (bucket_name, key) = rest
            .split_once('/')
            .context("object_ref must be s3://bucket/key")?;
        let credentials = Credentials::new(
            Some(&self.config.access_key),
            Some(&self.config.secret_key),
            None,
            None,
            None,
        )?;
        let region = Region::Custom {
            region: self.config.region.clone(),
            endpoint: self.config.endpoint.clone(),
        };
        let mut bucket = Bucket::new(bucket_name, region, credentials)?;
        bucket.set_path_style();
        Ok((bucket, key.to_string()))
    }
}

#[async_trait::async_trait]
impl ObjectFetcher for S3ObjectFetcher {
    async fn fetch(&self, object_ref: &str) -> Result<Vec<u8>> {
        let (bucket, key) = self.bucket_for(object_ref)?;
        let response = bucket.get_object(&key).await?;
        Ok(response.bytes().to_vec())
    }
}

/// 临时文件序号:并发执行(测试并行/同探针多命令)时避免共享同名临时 pcap。
static TEMP_FILE_SEQ: AtomicU64 = AtomicU64::new(0);

/// 对象暂存结果:Ready(已取回、hash 校验通过、临时落盘)或 Rejected(语义失败回执)。
enum StagedObject {
    Ready(PathBuf),
    Rejected(ReplayOutcome),
}

/// 取对象 → sha256 冻结校验 → 临时落盘(进程内回放与 wire 回放共用)。
async fn stage_validated_object(
    fetcher: &dyn ObjectFetcher,
    cache_dir: &Path,
    cmd: &ReplayWindowCommand,
) -> Result<StagedObject> {
    // 1. 取对象
    let raw = match fetcher.fetch(&cmd.object_ref).await {
        Ok(raw) => raw,
        Err(e) => {
            return Ok(StagedObject::Rejected(ReplayOutcome {
                applied: false,
                packets: 0,
                bytes_consumed: 0,
                flows_emitted: 0,
                watermark_ms: 0,
                fence: serde_json::json!({"kind": "source_fence", "detail": format!("fetch object: {e}")}),
                detail: format!("fetch object: {e}"),
            }))
        }
    };

    // 2. sha256 冻结清单校验
    let digest = hex::encode(Sha256::digest(&raw));
    if !digest.eq_ignore_ascii_case(&cmd.object_sha256) {
        return Ok(StagedObject::Rejected(ReplayOutcome {
            applied: false,
            packets: 0,
            bytes_consumed: 0,
            flows_emitted: 0,
            watermark_ms: 0,
            fence: serde_json::json!({"kind": "source_fence", "detail": "object sha256 mismatch"}),
            detail: format!("object sha256 mismatch: manifest={} actual={}", cmd.object_sha256, digest),
        }));
    }

    // 3. 临时落盘(文件名带进程内序号,避免并发执行共享同名文件)
    let seq = TEMP_FILE_SEQ.fetch_add(1, Ordering::Relaxed);
    let tmp_path = cache_dir.join(format!(
        "replay-{}-{}-{}-{}.pcap",
        cmd.run_id,
        cmd.fencing_token,
        std::process::id(),
        seq
    ));
    if let Err(e) = tokio::fs::write(&tmp_path, &raw).await {
        return Ok(StagedObject::Rejected(ReplayOutcome {
            applied: false,
            packets: 0,
            bytes_consumed: 0,
            flows_emitted: 0,
            watermark_ms: 0,
            fence: serde_json::json!({"kind": "source_fence", "detail": format!("stage object: {e}")}),
            detail: format!("stage object: {e}"),
        }));
    }
    Ok(StagedObject::Ready(tmp_path))
}

async fn cleanup_temp(tmp_path: &Path) {
    if let Err(e) = tokio::fs::remove_file(tmp_path).await {
        warn!("replay temp file cleanup failed: {e}");
    }
}

/// 执行有界对象回放。校验失败的语义错误经 ReplayOutcome.applied=false 表达(不 panic、不重试)。
pub async fn execute_replay_window(
    fetcher: &dyn ObjectFetcher,
    cache_dir: &Path,
    validated: &ValidatedReplayWindow,
    feed: PacketFeed<'_>,
) -> Result<ReplayOutcome> {
    let cmd: &ReplayWindowCommand = &validated.command;
    let staged = stage_validated_object(fetcher, cache_dir, cmd).await?;
    let tmp_path = match staged {
        StagedObject::Ready(path) => path,
        StagedObject::Rejected(outcome) => return Ok(outcome),
    };

    let outcome = replay_bounded_file(&tmp_path, cmd, feed).await;

    // 4. 清理临时文件(失败也清理)
    cleanup_temp(&tmp_path).await;
    Ok(outcome)
}

/// 执行有界 wire 回放(测试阶段数据集生成):帧经 AF_PACKET 注入指定接口,
/// 时间线压缩 + 帧间最小间隔;帧物理上线,watermark 用真实墙钟。
/// 注入失败立即返回 applied=false(已注入计数如实登记,不伪造成功)。
pub async fn execute_wire_replay_window(
    fetcher: &dyn ObjectFetcher,
    cache_dir: &Path,
    validated: &ValidatedReplayWindow,
    inject: &mut (dyn FnMut(&[u8]) -> Result<()> + Send),
) -> Result<ReplayOutcome> {
    let cmd: &ReplayWindowCommand = &validated.command;
    let iface = match cmd.interface.as_deref().filter(|s| !s.trim().is_empty()) {
        Some(name) => name.to_string(),
        None => {
            return Ok(ReplayOutcome {
                applied: false,
                packets: 0,
                bytes_consumed: 0,
                flows_emitted: 0,
                watermark_ms: 0,
                fence: serde_json::json!({"kind": "source_fence", "mode": "wire", "detail": "wire replay requires command.interface"}),
                detail: "wire replay requires command.interface".to_string(),
            })
        }
    };
    let staged = stage_validated_object(fetcher, cache_dir, cmd).await?;
    let tmp_path = match staged {
        StagedObject::Ready(path) => path,
        StagedObject::Rejected(outcome) => return Ok(outcome),
    };

    let outcome = wire_replay_bounded_file(&tmp_path, cmd, &iface, inject).await;
    cleanup_temp(&tmp_path).await;
    Ok(outcome)
}

async fn wire_replay_bounded_file(
    path: &Path,
    cmd: &ReplayWindowCommand,
    iface: &str,
    inject: &mut (dyn FnMut(&[u8]) -> Result<()> + Send),
) -> ReplayOutcome {
    let rejected = |detail: String| ReplayOutcome {
        applied: false,
        packets: 0,
        bytes_consumed: 0,
        flows_emitted: 0,
        watermark_ms: now_ms(),
        fence: serde_json::json!({"kind": "source_fence", "mode": "wire", "interface": iface, "detail": detail}),
        detail,
    };
    let mut replayer = match PcapReplayer::new(&path.to_string_lossy(), ReplaySpeed::MaxSpeed, false) {
        Ok(r) => r,
        Err(e) => return rejected(format!("open pcap: {e}")),
    };
    if let Err(e) = replayer.start().await {
        return rejected(format!("start replay: {e}"));
    }

    let mut packets: u64 = 0;
    let mut bytes_consumed: u64 = 0;
    let mut inject_errors: u64 = 0;
    let mut first_ts: Option<i64> = None;
    let mut wall_base: Option<tokio::time::Instant> = None;
    let mut prev_target: Option<tokio::time::Instant> = None;

    loop {
        match replayer.poll() {
            Ok(Some(batch)) => {
                for (packet, captured_at) in batch.iter() {
                    if cmd.packet_limit > 0 && packets >= cmd.packet_limit {
                        break;
                    }
                    if cmd.byte_limit > 0 && bytes_consumed + packet.len() as u64 > cmd.byte_limit {
                        break;
                    }
                    let ts = captured_at.epoch_micros() as i64;
                    let (base, first) = match (wall_base, first_ts) {
                        (Some(b), Some(f)) => (b, f),
                        (_, _) => {
                            let now = tokio::time::Instant::now();
                            if first_ts.is_none() {
                                first_ts = Some(ts);
                                wall_base = Some(now);
                                (now, ts)
                            } else {
                                (wall_base.unwrap_or(now), first_ts.unwrap_or(ts))
                            }
                        }
                    };
                    // 时间线压缩:相对首包的原始间隔除以压缩比,保留相对突发结构。
                    let rel_micros = (ts - first).max(0) as u64 / WIRE_REPLAY_TIME_COMPRESSION;
                    let mut target = base + Duration::from_micros(rel_micros);
                    if let Some(prev) = prev_target {
                        let floor = prev + Duration::from_micros(WIRE_REPLAY_MIN_GAP_MICROS);
                        if target < floor {
                            target = floor;
                        }
                    }
                    prev_target = Some(target);
                    tokio::time::sleep_until(target).await;

                    bytes_consumed += packet.len() as u64;
                    packets += 1;
                    if let Err(e) = inject(packet) {
                        inject_errors += 1;
                        let detail = format!("wire inject on {iface}: {e}");
                        return ReplayOutcome {
                            applied: false,
                            packets,
                            bytes_consumed,
                            flows_emitted: 0,
                            watermark_ms: now_ms(),
                            fence: serde_json::json!({
                                "kind": "source_fence",
                                "mode": "wire",
                                "interface": iface,
                                "object_ref": cmd.object_ref,
                                "packets": packets,
                                "bytes_consumed": bytes_consumed,
                                "inject_errors": inject_errors,
                                "detail": detail,
                            }),
                            detail,
                        };
                    }
                }
                if cmd.packet_limit > 0 && packets >= cmd.packet_limit {
                    break;
                }
                if cmd.byte_limit > 0 && bytes_consumed >= cmd.byte_limit {
                    break;
                }
            }
            Ok(None) => break,
            Err(e) => {
                debug!("wire replay poll ended: {e}");
                break;
            }
        }
    }

    let detail = if inject_errors == 0 {
        "complete".to_string()
    } else {
        format!("wire inject errors: {inject_errors}")
    };
    ReplayOutcome {
        applied: inject_errors == 0,
        packets,
        bytes_consumed,
        flows_emitted: 0,
        watermark_ms: now_ms(),
        fence: serde_json::json!({
            "kind": "source_fence",
            "mode": "wire",
            "interface": iface,
            "object_ref": cmd.object_ref,
            "object_sha256": cmd.object_sha256,
            "packets": packets,
            "bytes_consumed": bytes_consumed,
            "inject_errors": inject_errors,
            "packet_limit": cmd.packet_limit,
            "byte_limit": cmd.byte_limit,
            "fencing_token": cmd.fencing_token,
            "pacing": {
                "time_compression": WIRE_REPLAY_TIME_COMPRESSION,
                "min_gap_us": WIRE_REPLAY_MIN_GAP_MICROS,
            },
            "detail": detail,
        }),
        detail,
    }
}

async fn replay_bounded_file(
    path: &Path,
    cmd: &ReplayWindowCommand,
    feed: PacketFeed<'_>,
) -> ReplayOutcome {
    let mut replayer = match PcapReplayer::new(&path.to_string_lossy(), ReplaySpeed::MaxSpeed, false) {
        Ok(r) => r,
        Err(e) => {
            return ReplayOutcome {
                applied: false,
                packets: 0,
                bytes_consumed: 0,
                flows_emitted: 0,
                watermark_ms: 0,
                fence: serde_json::json!({"kind": "source_fence", "detail": format!("open pcap: {e}")}),
                detail: format!("open pcap: {e}"),
            }
        }
    };
    if let Err(e) = replayer.start().await {
        return ReplayOutcome {
            applied: false,
            packets: 0,
            bytes_consumed: 0,
            flows_emitted: 0,
            watermark_ms: 0,
            fence: serde_json::json!({"kind": "source_fence", "detail": format!("start replay: {e}")}),
            detail: format!("start replay: {e}"),
        };
    }

    let mut packets: u64 = 0;
    let mut bytes_consumed: u64 = 0;
    let mut flows_emitted: u64 = 0;
    let mut first_ts_micros: Option<i64> = None;
    let mut last_ts_micros: i64 = 0;

    loop {
        match replayer.poll() {
            Ok(Some(batch)) => {
                for (packet, captured_at) in batch.iter() {
                    if cmd.packet_limit > 0 && packets >= cmd.packet_limit {
                        break;
                    }
                    if cmd.byte_limit > 0 && bytes_consumed + packet.len() as u64 > cmd.byte_limit {
                        break;
                    }
                    let ts = captured_at.epoch_micros() as i64;
                    // 时间线平移:首包对齐 run 窗口起点(数据集常携带 1970 纪元时间戳),
                    // 保相对时序;feed 收到平移后的时间戳。
                    let shift = if let Some(first) = first_ts_micros {
                        cmd.window_start_ms * 1000 - first
                    } else {
                        0
                    };
                    if first_ts_micros.is_none() {
                        first_ts_micros = Some(ts);
                    }
                    let shifted_ts = ts + shift;
                    last_ts_micros = shifted_ts;
                    bytes_consumed += packet.len() as u64;
                    packets += 1;
                    if let Err(e) = feed(packet, shifted_ts) {
                        return ReplayOutcome {
                            applied: false,
                            packets,
                            bytes_consumed,
                            flows_emitted,
                            watermark_ms: last_ts_micros / 1000,
                            fence: serde_json::json!({"kind": "source_fence", "detail": format!("feed: {e}")}),
                            detail: format!("feed: {e}"),
                        };
                    }
                    flows_emitted += 1;
                }
                if cmd.packet_limit > 0 && packets >= cmd.packet_limit {
                    break;
                }
                if cmd.byte_limit > 0 && bytes_consumed >= cmd.byte_limit {
                    break;
                }
            }
            Ok(None) => break,
            Err(e) => {
                debug!("replay poll ended: {e}");
                break;
            }
        }
    }

    let watermark_ms = if first_ts_micros.is_some() {
        last_ts_micros / 1000
    } else {
        cmd.window_start_ms
    };

    ReplayOutcome {
        applied: true,
        packets,
        bytes_consumed,
        flows_emitted,
        watermark_ms,
        fence: serde_json::json!({
            "kind": "source_fence",
            "object_ref": cmd.object_ref,
            "object_sha256": cmd.object_sha256,
            "packets": packets,
            "bytes_consumed": bytes_consumed,
            "packet_limit": cmd.packet_limit,
            "byte_limit": cmd.byte_limit,
            "fencing_token": cmd.fencing_token,
            "detail": "complete",
        }),
        detail: "complete".into(),
    }
}

/// 当前毫秒时间戳(命令回执用)。
pub fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    struct StubFetcher(Vec<u8>);
    #[async_trait::async_trait]
    impl ObjectFetcher for StubFetcher {
        async fn fetch(&self, _: &str) -> Result<Vec<u8>> {
            Ok(self.0.clone())
        }
    }

    fn valid_command() -> ReplayWindowCommand {
        ReplayWindowCommand {
            tenant_id: "default".into(),
            task_id: "task-1".into(),
            run_id: "run-1".into(),
            execution_spec_sha256: "spec-1".into(),
            probe_id: "probe-agent".into(),
            object_ref: "s3://analysis-bench/pcap/x.pcap".into(),
            object_sha256: String::new(), // 测试内填充
            interface: None,
            window_start_ms: 1_700_000_000_000,
            window_end_ms: 1_700_000_600_000,
            packet_limit: 1000,
            byte_limit: 0,
            fencing_token: "fence-1".into(),
        }
    }

    #[tokio::test]
    async fn hash_mismatch_rejected_without_replay() {
        let fetcher = StubFetcher(vec![1, 2, 3, 4]);
        let mut cmd = valid_command();
        cmd.object_sha256 = "a".repeat(64);
        let validated = cmd.validate().unwrap();
        let dir = std::env::temp_dir();
        let mut fed = 0;
        let outcome = execute_replay_window(&fetcher, &dir, &validated, &mut |_, _| {
            fed += 1;
            Ok(())
        })
        .await
        .unwrap();
        assert!(!outcome.applied);
        assert_eq!(fed, 0);
        assert!(outcome.detail.contains("sha256 mismatch"));
    }

    #[tokio::test]
    async fn replay_counts_parsed_packets() {
        // 手工构造 pcap:全局头 + 两个伪造包记录(仅验证有界回放与 feed 调用计数)
        let mut pcap = vec![0xd4, 0xc3, 0xb2, 0xa1, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0, 0, 1, 0, 0, 0];
        let pkt1 = vec![0xaa; 64];
        let pkt2 = vec![0xbb; 32];
        for pkt in [&pkt1, &pkt2] {
            let mut rec = Vec::new();
            rec.extend_from_slice(&0u32.to_le_bytes()); // ts_sec
            rec.extend_from_slice(&0u32.to_le_bytes()); // ts_usec
            rec.extend_from_slice(&(pkt.len() as u32).to_le_bytes()); // incl_len
            rec.extend_from_slice(&(pkt.len() as u32).to_le_bytes()); // orig_len
            rec.extend_from_slice(pkt);
            pcap.extend_from_slice(&rec);
        }
        let digest = hex::encode(Sha256::digest(&pcap));
        let fetcher = StubFetcher(pcap.clone());
        let mut cmd = valid_command();
        cmd.object_sha256 = digest;
        let validated = cmd.validate().unwrap();
        let dir = std::env::temp_dir();
        let mut fed = 0;
        let outcome = execute_replay_window(&fetcher, &dir, &validated, &mut |_, _| {
            fed += 1;
            Ok(())
        })
        .await
        .unwrap();
        assert!(outcome.applied, "detail={}", outcome.detail);
        assert_eq!(outcome.packets, 2);
        assert_eq!(fed, 2);
        assert!(outcome.fence["packets"] == 2);
    }

    #[tokio::test]
    async fn wire_replay_rejects_missing_interface() {
        let fetcher = StubFetcher(vec![1, 2, 3]);
        let mut cmd = valid_command();
        cmd.object_sha256 = hex::encode(Sha256::digest(&vec![1u8, 2, 3]));
        cmd.interface = None;
        let validated = cmd.validate().unwrap();
        let dir = std::env::temp_dir();
        let mut injected = 0;
        let outcome = execute_wire_replay_window(&fetcher, &dir, &validated, &mut |_| {
            injected += 1;
            Ok(())
        })
        .await
        .unwrap();
        assert!(!outcome.applied);
        assert_eq!(injected, 0);
        assert_eq!(outcome.fence["mode"], "wire");
        assert!(outcome.detail.contains("requires command.interface"));
    }

    #[tokio::test]
    async fn wire_replay_injects_bounded_packets_with_wire_fence() {
        let mut pcap = vec![0xd4, 0xc3, 0xb2, 0xa1, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0, 0, 1, 0, 0, 0];
        let pkt1 = vec![0xaa; 64];
        let pkt2 = vec![0xbb; 32];
        for pkt in [&pkt1, &pkt2] {
            let mut rec = Vec::new();
            rec.extend_from_slice(&0u32.to_le_bytes()); // ts_sec
            rec.extend_from_slice(&0u32.to_le_bytes()); // ts_usec
            rec.extend_from_slice(&(pkt.len() as u32).to_le_bytes()); // incl_len
            rec.extend_from_slice(&(pkt.len() as u32).to_le_bytes()); // orig_len
            rec.extend_from_slice(pkt);
            pcap.extend_from_slice(&rec);
        }
        let digest = hex::encode(Sha256::digest(&pcap));
        let fetcher = StubFetcher(pcap);
        let mut cmd = valid_command();
        cmd.object_sha256 = digest;
        cmd.interface = Some("ta-veth-in".into());
        let validated = cmd.validate().unwrap();
        let dir = std::env::temp_dir();
        let mut injected: Vec<u8> = Vec::new();
        let outcome = execute_wire_replay_window(&fetcher, &dir, &validated, &mut |frame| {
            injected.push(frame[0]);
            Ok(())
        })
        .await
        .unwrap();
        assert!(outcome.applied, "detail={}", outcome.detail);
        assert_eq!(outcome.packets, 2);
        assert_eq!(injected, vec![0xaa, 0xbb], "wire feed must receive each frame once");
        assert_eq!(outcome.fence["mode"], "wire");
        assert_eq!(outcome.fence["interface"], "ta-veth-in");
        assert_eq!(outcome.fence["pacing"]["time_compression"], WIRE_REPLAY_TIME_COMPRESSION);
    }

    #[tokio::test]
    async fn wire_replay_fails_closed_on_inject_error() {
        let mut pcap = vec![0xd4, 0xc3, 0xb2, 0xa1, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0, 0, 1, 0, 0, 0];
        let mut rec = Vec::new();
        rec.extend_from_slice(&0u32.to_le_bytes());
        rec.extend_from_slice(&0u32.to_le_bytes());
        rec.extend_from_slice(&64u32.to_le_bytes());
        rec.extend_from_slice(&64u32.to_le_bytes());
        rec.extend_from_slice(&vec![0xcc; 64]);
        pcap.extend_from_slice(&rec);
        let digest = hex::encode(Sha256::digest(&pcap));
        let fetcher = StubFetcher(pcap);
        let mut cmd = valid_command();
        cmd.object_sha256 = digest;
        cmd.interface = Some("ta-veth-in".into());
        let validated = cmd.validate().unwrap();
        let dir = std::env::temp_dir();
        let outcome = execute_wire_replay_window(&fetcher, &dir, &validated, &mut |_| {
            anyhow::bail!("inject failed")
        })
        .await
        .unwrap();
        assert!(!outcome.applied);
        assert_eq!(outcome.packets, 1, "failed frame must be counted");
        assert!(outcome.fence["inject_errors"] == 1);
        assert!(outcome.detail.contains("inject failed"));
    }
}
