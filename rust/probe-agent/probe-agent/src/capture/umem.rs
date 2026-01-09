use anyhow::{Result, Context};
use memmap2::MmapMut;

#[derive(Debug, Clone)]
pub struct UmemConfig {
    pub frame_size: usize,       // 单帧大小（默认 4096，Kunpeng 严格对齐）
    pub frame_count: usize,      // 帧数量（默认 4096）
    pub fill_queue_size: u32,    // Fill Ring 大小
    pub comp_queue_size: u32,    // Completion Ring 大小
}

impl Default for UmemConfig {
    fn default() -> Self {
        Self {
            frame_size: 4096,
            frame_count: 4096,
            fill_queue_size: 2048,
            comp_queue_size: 2048,
        }
    }
}

pub struct Umem {
    area: MmapMut,
    frame_size: usize,
    frame_count: usize,
}

impl Umem {
    pub fn new(config: &UmemConfig) -> Result<Self> {
        let total_size = config.frame_size * config.frame_count;
        
        // 使用 mmap 分配对齐的内存
        let area = MmapMut::map_anon(total_size)
            .context("Failed to allocate UMEM")?;

        tracing::info!(
            "UMEM allocated: {} frames × {} bytes = {} MB",
            config.frame_count,
            config.frame_size,
            total_size / 1024 / 1024
        );

        Ok(Self {
            area,
            frame_size: config.frame_size,
            frame_count: config.frame_count,
        })
    }

    /// 获取指定索引的帧（只读）
    pub fn get_frame(&self, idx: usize) -> Result<&[u8]> {
        if idx >= self.frame_count {
            anyhow::bail!("Frame index {} out of bounds (max: {})", idx, self.frame_count);
        }
        let offset = idx * self.frame_size;
        Ok(&self.area[offset..offset + self.frame_size])
    }

    /// 获取指定索引的帧（可写）
    pub fn get_frame_mut(&mut self, idx: usize) -> Result<&mut [u8]> {
        if idx >= self.frame_count {
            anyhow::bail!("Frame index {} out of bounds (max: {})", idx, self.frame_count);
        }
        let offset = idx * self.frame_size;
        Ok(&mut self.area[offset..offset + self.frame_size])
    }

    /// 获取 UMEM 基地址（用于 XDP 设置）
    pub fn as_ptr(&self) -> *const u8 {
        self.area.as_ptr()
    }

    pub fn as_mut_ptr(&mut self) -> *mut u8 {
        self.area.as_mut_ptr()
    }

    pub fn frame_size(&self) -> usize {
        self.frame_size
    }

    pub fn frame_count(&self) -> usize {
        self.frame_count
    }

    pub fn total_size(&self) -> usize {
        self.frame_size * self.frame_count
    }
}

unsafe impl Send for Umem {}
unsafe impl Sync for Umem {}