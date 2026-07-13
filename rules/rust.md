# Rust 开发规范

基于 [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/)、[The Rust Performance Book](https://nnethercote.github.io/perf-book/)、[Rust Sec](https://rustsec.org/)。

## 1. 命名

```rust
// 模块：snake_case
mod flow_aggregator;    // ✓
mod FlowAggregator;     // ✗

// 类型：PascalCase (CamelCase)
struct FlowTable { ... }
enum CaptureMode { ... }
trait Capturer { ... }

// 函数/方法：snake_case
fn parse_packet(data: &[u8]) -> Result<Packet> { ... }
fn flow_count(&self) -> usize { ... }

// 常量/静态：SCREAMING_SNAKE_CASE
const MAX_FLOW_CAPACITY: usize = 1_000_000;
static METRICS_REGISTRY: Lazy<Registry> = Lazy::new(Registry::new);

// 构造器：约定 new / from_ / with_
impl FlowTable {
    pub fn new(capacity: usize) -> Self { ... }
    pub fn from_config(config: &Config) -> Result<Self> { ... }
    pub fn with_partitions(partitions: usize, capacity: usize) -> Self { ... }
}

// 转换：From / Into / AsRef / AsMut
impl From<&Config> for GrpcSenderConfig { ... }
```

## 2. 错误处理

```rust
// 库代码：定义明确的 Error 类型
#[derive(Error, Debug)]
pub enum CaptureError {
    #[error("interface {0} not found")]
    InterfaceNotFound(String),
    
    #[error("XDP load failed: {0}")]
    XdpLoad(#[source] io::Error),
    
    #[error("packet too large: {size} bytes")]
    PacketTooLarge { size: usize },
}

// 应用代码：anyhow::Result
use anyhow::{Context, Result};
fn main() -> Result<()> {
    let config = load_config().context("failed to load config")?;
    Ok(())
}

// 禁止：
//   unwrap() / expect() 在生产代码路径
//   Box<dyn Error> 作为库 API（用 enum）
//   panic! 替代错误返回

// 必须使用 Context 添加上下文
let file = File::open(path)
    .with_context(|| format!("failed to open {}", path))?;
```

## 3. 所有权与性能

```rust
// 优先借用而非复制
fn process(&self, data: &[u8]) -> Result<Flow> { ... }  // ✓
fn process(&self, data: Vec<u8>) -> Result<Flow> { ... } // ✗ (不必要)

// 大类型用 Arc 共享
let flow_table = Arc::new(PartitionedFlowTable::new(16, 65536));

// 零拷贝解析
// 使用 etherparse 的切片解析，不分配额外内存
let packet = SlicedPacket::from_ethernet(data)?;

// 批量操作

// Vec 预分配
let mut events = Vec::with_capacity(estimated_count);

// 无锁并发：DashMap > RwLock<HashMap>（高并发读场景）
let flows: DashMap<FlowKey, FlowEntry> = DashMap::with_capacity(capacity);

// 禁止：
//   clone() 在 hot path
//   Arc<Mutex<>> 包裹大结构体
//   String 拼接循环中用 + 而非 String::with_capacity + push_str
```

## 4. unsafe 规范

```rust
// unsafe 代码必须：
// 1. 单独封装在安全抽象中
// 2. 用 // SAFETY: 注释说明前置条件
// 3. 在被调用处注明满足条件的原因

/// Returns a slice of UMEM data at the given offset.
/// 
/// # Safety
/// 
/// Caller must ensure addr + len <= self.size and the memory is valid.
pub unsafe fn get_data_unchecked(&self, addr: usize, len: usize) -> &[u8] {
    // SAFETY: UMEM is allocated with MAP_POPULATE and never freed while in use.
    unsafe { std::slice::from_raw_parts(self.addr.add(addr), len) }
}

// 禁止：
//   transmute 替代安全的类型转换
//   MaybeUninit 未初始化读取
//   多线程修改裸指针指向的数据（用 Atomic 或 Mutex）
```

## 5. 异步 (tokio)

```rust
// 避免在 async context 中阻塞
async fn process_batch(&self, batch: &PacketBatch) {
    // ✓ 异步 sleep
    tokio::time::sleep(Duration::from_millis(100)).await;
    
    // ✗ 同步 sleep（阻塞整个 task）
    // std::thread::sleep(Duration::from_millis(100));
}

// spawn 阻塞 CPU 任务
let result = tokio::task::spawn_blocking(move || {
    zstd::bulk::compress(&data, 3)
}).await?;

// 使用 select! 同时等待多个 future
tokio::select! {
    Some(batch) = rx.recv() => process(batch).await,
    _ = shutdown.notified() => return,
    _ = tokio::time::sleep(Duration::from_secs(1)) => flush(),
}

// Cancel safety: 确保 select! 分支取消时不会丢失数据
```

## 6. 测试

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_valid_packet() {
        let data = create_test_packet();
        let result = PacketParser::parse(&data, 0).unwrap();
        assert!(result.is_some());
        assert_eq!(result.unwrap().src_ip, "192.168.1.1");
    }

    #[test]
    fn test_parse_invalid_returns_none() {
        assert!(PacketParser::parse(&[0u8; 10], 0).unwrap().is_none());
    }

    // PCAP 回放端到端测试
    #[tokio::test]
    async fn test_pcap_replay_end_to_end() {
        let dir = TempDir::new().unwrap();
        create_test_pcap(dir.path());
        
        let mut replayer = PcapReplayer::new(
            dir.path().to_str().unwrap(),
            ReplaySpeed::MaxSpeed,
            false,
        ).unwrap();
        
        replayer.start().await.unwrap();
        let mut packets = 0;
        while let Ok(Some(batch)) = replayer.poll() {
            packets += batch.len();
        }
        assert!(packets > 0);
    }
}
```

## 7. 项目结构

```
src/
  main.rs           # 入口、组件编排
  lib.rs            # 库 root
  config.rs         # 配置解析（serde + YAML）
  capture/          # 采集层
    mod.rs          # Capturer trait + create_capturer
    xdp.rs          # AF_XDP 采集
    af_packet.rs    # AF_PACKET 回退
    pcap_offline.rs # PCAP 离线回放
    umem.rs         # UMEM 内存管理
  parser/           # 协议解析（etherparse）
  aggregator/       # Flow 聚合（分区流表 + 时间轮）
  archiver/         # PCAP 归档（TripleBuffer + S3）
  sender/           # gRPC 批量上报
  metrics/          # Prometheus 指标
```
