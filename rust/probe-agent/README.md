# Probe Agent - Traffic Analysis Platform

高性能网络流量采集探针，基于 Rust + eBPF/AF_XDP 实现。

## ✨ 特性

- ⚡ **高性能**: 支持 100Gbps 线速捕获，峰值 512Mpps
- 🔒 **零拷贝**: 基于 AF_XDP/eBPF 的零拷贝捕获
- 📦 **流聚合**: 实时五元组流聚合与会话化
- 💾 **PCAP 归档**: 双缓冲 + Zstd 压缩 + S3 上传
- 🔐 **安全传输**: mTLS 双向认证
- 📊 **可观测**: Prometheus 指标暴露

## 🚀 快速开始

### 前置依赖

- Rust 1.75+
- Linux Kernel 5.10+ (支持 eBPF)
- openEuler 22.03 LTS (推荐)

### 开发环境设置

```bash
# 运行自动化设置脚本
./scripts/setup-dev-openeuler.sh