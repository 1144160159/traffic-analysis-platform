package com.traffic.flink.pcap.util;

import com.google.common.hash.BloomFilter;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class BloomFilterUtilTest {

    @Test
    @DisplayName("创建 BloomFilter 并序列化/反序列化")
    void testCreateAndSerialize() {
        List<String> ips = Arrays.asList(
                "192.168.1.1",
                "192.168.1.2",
                "10.0.0.1",
                "172.16.0.100"
        );

        BloomFilter<String> filter = BloomFilterUtil.createFromIps(ips, 100);
        
        // 验证包含添加的 IP
        for (String ip : ips) {
            assertThat(filter.mightContain(ip)).isTrue();
        }

        // 序列化
        String base64 = BloomFilterUtil.toBase64(filter);
        assertThat(base64).isNotEmpty();

        // 反序列化
        BloomFilter<String> restored = BloomFilterUtil.fromBase64(base64);
        assertThat(restored).isNotNull();

        // 验证恢复后仍然包含原 IP
        for (String ip : ips) {
            assertThat(restored.mightContain(ip)).isTrue();
        }
    }

    @Test
    @DisplayName("BloomFilter 不存在的元素返回 false")
    void testNotContains() {
        List<String> ips = Arrays.asList("192.168.1.1", "192.168.1.2");
        BloomFilter<String> filter = BloomFilterUtil.createFromIps(ips, 100);

        // 未添加的 IP 大概率返回 false
        assertThat(filter.mightContain("8.8.8.8")).isFalse();
        assertThat(filter.mightContain("1.1.1.1")).isFalse();
    }

    @Test
    @DisplayName("mightContain 快速检查")
    void testMightContain() {
        List<String> ips = Arrays.asList("192.168.1.1", "10.0.0.1");
        String base64 = BloomFilterUtil.toBase64(BloomFilterUtil.createFromIps(ips, 100));

        assertThat(BloomFilterUtil.mightContain(base64, "192.168.1.1")).isTrue();
        assertThat(BloomFilterUtil.mightContain(base64, "8.8.8.8")).isFalse();
        assertThat(BloomFilterUtil.mightContain(null, "192.168.1.1")).isFalse();
        assertThat(BloomFilterUtil.mightContain("", "192.168.1.1")).isFalse();
    }

    @Test
    @DisplayName("合并多个 BloomFilter")
    void testMergeFilters() {
        List<String> ips1 = Arrays.asList("192.168.1.1", "192.168.1.2");
        List<String> ips2 = Arrays.asList("10.0.0.1", "10.0.0.2");

        String filter1 = BloomFilterUtil.toBase64(BloomFilterUtil.createFromIps(ips1, 100));
        String filter2 = BloomFilterUtil.toBase64(BloomFilterUtil.createFromIps(ips2, 100));

        String merged = BloomFilterUtil.mergeFilters(Arrays.asList(filter1, filter2));
        assertThat(merged).isNotEmpty();

        // 合并后应包含所有 IP
        assertThat(BloomFilterUtil.mightContain(merged, "192.168.1.1")).isTrue();
        assertThat(BloomFilterUtil.mightContain(merged, "10.0.0.1")).isTrue();
    }
}