// Parser 模块（占位，S1.3 完整实现）
use anyhow::Result;

pub struct PacketParser;

impl PacketParser {
    pub fn new() -> Self {
        Self
    }

    pub fn parse(&self, _data: &[u8]) -> Result<()> {
        // TODO: 实现协议解析
        Ok(())
    }
}