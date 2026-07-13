package com.traffic.flink.pcap.util;

import com.google.common.hash.BloomFilter;
import com.google.common.hash.Funnels;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.Collection;

/**
 * BloomFilter 工具类（增强版 v3）
 * 
 * ==================== 职责边界说明 ====================
 * 
 * 本工具类提供 BloomFilter 的创建、序列化、反序列化、查询、合并等功能。
 * 
 * **职责分工**：
 * 
 * 1. **Probe Agent（rust/probe-agent）**：
 *    - 在 PCAP 文件生成时，解析包头提取所有 IP 地址
 *    - 使用本工具类（Rust 版本）生成 BloomFilter
 *    - 将 BloomFilter Base64 编码后填充到 PcapIndexMeta.bloom_filter_b64 字段
 *    - 上报到 Ingest Gateway
 * 
 * 2. **Flink PCAP Index Job**：
 *    - 仅负责验证 bloom_filter_b64 字段是否存在
 *    - **不负责生成 BloomFilter**（避免重复解析 PCAP 文件，性能开销巨大）
 *    - 如果缺失，记录 Metrics（incMissingBloomFilter）并允许通过
 * 
 * 3. **Forensics Service（go/control-plane/forensics-service）**：
 *    - 使用本工具类（Go 版本）反序列化 BloomFilter
 *    - 快速过滤 PCAP 文件（根据 IP 是否可能存在）
 * 
 * ==================== 使用示例 ====================
 * 
 * // Probe Agent 侧（生成 BloomFilter）
 * List<String> ips = extractIpsFromPcap(pcapFile);
 * BloomFilter<String> filter = BloomFilterUtil.createFromIps(ips, ips.size() * 2);
 * String base64 = BloomFilterUtil.toBase64(filter);
 * pcapIndexMeta.setBloomFilterB64(base64);
 * 
 * // Forensics Service 侧（查询）
 * String bloomFilterB64 = pcapIndex.getBloomFilterB64();
 * if (BloomFilterUtil.mightContain(bloomFilterB64, targetIp)) {
 *     // 可能包含，继续精确查询
 * }
 * 
 * ==================== 性能优化建议 ====================
 * 
 * 1. **误判率（FPP）**：
 *    - 默认 0.01（1%）适合大多数场景
 *    - 取证场景可降低到 0.001（0.1%）以减少误判
 *    - 注意：FPP 越低，内存开销越大
 * 
 * 2. **预期大小（Expected Size）**：
 *    - 建议设置为实际 IP 数量的 1.5-2 倍
 *    - 过小会导致误判率升高
 *    - 过大会浪费内存
 * 
 * 3. **内存估算**：
 *    - 使用 estimateMemorySize() 方法提前评估
 *    - 典型场景：1000 个 IP，FPP=0.01，约占用 1.5 KB
 * 
 * ==================== 已知限制 ====================
 * 
 * 1. BloomFilter 仅支持"可能存在"判断，无法删除元素
 * 2. Base64 编码后体积增大约 33%
 * 3. Guava BloomFilter 不支持跨语言序列化（需 Rust/Go 各自实现）
 * 
 * @version 3.0
 * @since 2024-01-15
 */
public final class BloomFilterUtil {

    private static final Logger LOG = LoggerFactory.getLogger(BloomFilterUtil.class);

    private BloomFilterUtil() {
    }

    // 默认误判率
    private static final double DEFAULT_FPP = 0.01;
    
    // 最小预期大小
    private static final int MIN_EXPECTED_SIZE = 100;

    /**
     * 从 IP 列表创建 BloomFilter（使用默认误判率）
     *
     * @param ips  IP 地址列表
     * @param expectedSize 预期大小（如果为 0，使用列表大小的 2 倍）
     * @return BloomFilter
     */
    public static BloomFilter<String> createFromIps(Collection<String> ips, int expectedSize) {
        return createFromIps(ips, expectedSize, DEFAULT_FPP);
    }

    /**
     * 从 IP 列表创建 BloomFilter（可配置误判率）
     *
     * @param ips  IP 地址列表
     * @param expectedSize 预期大小
     * @param fpp 误判率（False Positive Probability，建议 0.001-0.01）
     * @return BloomFilter
     */
    public static BloomFilter<String> createFromIps(
            Collection<String> ips, 
            int expectedSize,
            double fpp
    ) {
        // ✅ 空指针防护
        if (ips == null || ips.isEmpty()) {
            LOG.warn("Creating empty BloomFilter: ips is null or empty");
            return BloomFilter.create(
                    Funnels.stringFunnel(StandardCharsets.UTF_8),
                    MIN_EXPECTED_SIZE,
                    fpp
            );
        }

        // 计算有效大小
        int size = expectedSize > 0 ? expectedSize : Math.max(ips.size() * 2, MIN_EXPECTED_SIZE);
        
        LOG.debug("Creating BloomFilter: size={}, fpp={}, actual_ips={}", size, fpp, ips.size());
        
        BloomFilter<String> filter = BloomFilter.create(
                Funnels.stringFunnel(StandardCharsets.UTF_8),
                size,
                fpp
        );

        int addedCount = 0;
        for (String ip : ips) {
            if (ip != null && !ip.isEmpty()) {
                filter.put(ip);
                addedCount++;
            }
        }

        LOG.debug("BloomFilter created: added {} IPs out of {} total", addedCount, ips.size());
        
        return filter;
    }

    /**
     * 将 BloomFilter 序列化为 Base64 字符串
     *
     * @param filter BloomFilter
     * @return Base64 编码的字符串
     */
    public static String toBase64(BloomFilter<String> filter) {
        if (filter == null) {
            LOG.warn("Attempting to serialize null BloomFilter");
            return "";
        }

        try (ByteArrayOutputStream baos = new ByteArrayOutputStream()) {
            filter.writeTo(baos);
            String base64 = Base64.getEncoder().encodeToString(baos.toByteArray());
            LOG.debug("BloomFilter serialized to Base64: size={} bytes", baos.size());
            return base64;
        } catch (IOException e) {
            LOG.error("Failed to serialize BloomFilter: {}", e.getMessage(), e);
            throw new RuntimeException("Failed to serialize BloomFilter", e);
        }
    }

    /**
     * 从 Base64 字符串反序列化 BloomFilter
     *
     * @param base64 Base64 编码的字符串
     * @return BloomFilter，如果输入无效则返回 null
     */
    public static BloomFilter<String> fromBase64(String base64) {
        if (base64 == null || base64.isEmpty()) {
            LOG.debug("fromBase64 called with null or empty string");
            return null;
        }

        try {
            byte[] bytes = Base64.getDecoder().decode(base64);
            try (ByteArrayInputStream bais = new ByteArrayInputStream(bytes)) {
                BloomFilter<String> filter = BloomFilter.readFrom(
                        bais, 
                        Funnels.stringFunnel(StandardCharsets.UTF_8)
                );
                LOG.debug("BloomFilter deserialized from Base64: size={} bytes", bytes.length);
                return filter;
            }
        } catch (IllegalArgumentException e) {
            LOG.error("Invalid Base64 input: {}", e.getMessage());
            return null;
        } catch (IOException e) {
            LOG.error("Failed to deserialize BloomFilter: {}", e.getMessage(), e);
            throw new RuntimeException("Failed to deserialize BloomFilter", e);
        }
    }

    /**
     * 检查 IP 是否可能存在于 BloomFilter 中
     *
     * @param filterBase64 Base64 编码的 BloomFilter
     * @param ip           要检查的 IP
     * @return true 如果可能存在，false 如果一定不存在
     */
    public static boolean mightContain(String filterBase64, String ip) {
        if (filterBase64 == null || filterBase64.isEmpty() || ip == null || ip.isEmpty()) {
            return false;
        }

        try {
            BloomFilter<String> filter = fromBase64(filterBase64);
            return filter != null && filter.mightContain(ip);
        } catch (Exception e) {
            LOG.error("Error checking BloomFilter: {}", e.getMessage(), e);
            return false;
        }
    }

    /**
     * 合并多个 BloomFilter
     *
     * @param filterBase64List BloomFilter Base64 字符串列表
     * @return 合并后的 BloomFilter Base64 字符串
     */
    public static String mergeFilters(Collection<String> filterBase64List) {
        if (filterBase64List == null || filterBase64List.isEmpty()) {
            LOG.warn("mergeFilters called with null or empty list");
            return "";
        }

        BloomFilter<String> merged = null;
        int mergedCount = 0;

        for (String base64 : filterBase64List) {
            if (base64 == null || base64.isEmpty()) {
                continue;
            }

            try {
                BloomFilter<String> filter = fromBase64(base64);
                if (filter == null) {
                    continue;
                }

                if (merged == null) {
                    merged = filter;
                } else {
                    merged.putAll(filter);
                }
                mergedCount++;
            } catch (Exception e) {
                LOG.error("Error merging BloomFilter: {}", e.getMessage(), e);
            }
        }

        if (merged == null) {
            LOG.warn("No valid BloomFilters to merge");
            return "";
        }

        LOG.debug("Merged {} BloomFilters", mergedCount);
        return toBase64(merged);
    }

    /**
     * 估算 BloomFilter 的内存大小（字节）
     *
     * @param expectedInsertions 预期插入数量
     * @param fpp 误判率
     * @return 估算的内存大小（字节）
     */
    public static long estimateMemorySize(long expectedInsertions, double fpp) {
        // Bloom Filter 内存计算公式：m = -n * ln(p) / (ln(2))^2
        // 其中：n = expectedInsertions, p = fpp, m = bits
        double bitsPerElement = -Math.log(fpp) / (Math.log(2) * Math.log(2));
        long totalBits = (long) (expectedInsertions * bitsPerElement);
        long totalBytes = (totalBits + 7) / 8; // bits -> bytes
        
        return totalBytes;
    }
}