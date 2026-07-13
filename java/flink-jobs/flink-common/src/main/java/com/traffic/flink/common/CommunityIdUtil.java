package com.traffic.flink.common;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Base64;
import java.util.regex.Pattern;

/**
 * Community ID 计算工具类 (修复版)
 * 
 * 实现标准 Community ID v1.0 规范
 * https://github.com/corelight/community-id-spec
 * 
 * 修复内容：
 * - 完善 IPv6 格式校验
 * - 添加输入验证
 * - 优化性能
 */
public final class CommunityIdUtil {

    private static final String COMMUNITY_ID_VERSION = "1";
    private static final int SEED = 0;

    // IPv4 格式正则
    private static final Pattern IPV4_PATTERN = Pattern.compile(
            "^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$"
    );

    // IPv6 简化格式正则（支持 :: 压缩）
    private static final Pattern IPV6_PATTERN = Pattern.compile(
            "^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|" +
                    "^([0-9a-fA-F]{1,4}:){1,7}:$|" +
                    "^([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}$|" +
                    "^([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}$|" +
                    "^([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}$|" +
                    "^([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}$|" +
                    "^([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}$|" +
                    "^[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})$|" +
                    "^:((:[0-9a-fA-F]{1,4}){1,7}|:)$|" +
                    "^::$"
    );

    // ThreadLocal SHA-1 MessageDigest（避免线程安全问题）
    private static final ThreadLocal<MessageDigest> SHA1_DIGEST = ThreadLocal.withInitial(() -> {
        try {
            return MessageDigest.getInstance("SHA-1");
        } catch (NoSuchAlgorithmException e) {
            throw new RuntimeException("SHA-1 algorithm not available", e);
        }
    });

    private CommunityIdUtil() {
        // Utility class
    }

    /**
     * 计算 Community ID
     *
     * @param srcIp    源 IP 地址
     * @param dstIp    目标 IP 地址
     * @param srcPort  源端口
     * @param dstPort  目标端口
     * @param protocol 协议号 (6=TCP, 17=UDP, 1=ICMP)
     * @return Community ID 字符串，格式: "1:base64hash"
     */
    public static String compute(String srcIp, String dstIp, int srcPort, int dstPort, int protocol) {
        // 输入验证
        if (srcIp == null || srcIp.isEmpty()) {
            return "";
        }
        if (dstIp == null || dstIp.isEmpty()) {
            return "";
        }

        // 验证 IP 格式
        if (!isValidIp(srcIp) || !isValidIp(dstIp)) {
            return "";
        }

        // 验证端口范围
        srcPort = Math.max(0, Math.min(srcPort, 65535));
        dstPort = Math.max(0, Math.min(dstPort, 65535));

        // 验证协议范围
        protocol = Math.max(0, Math.min(protocol, 255));

        // 规范化：确保较小的 IP:Port 在前
        boolean needSwap = shouldSwap(srcIp, dstIp, srcPort, dstPort);

        String ip1 = needSwap ? dstIp : srcIp;
        String ip2 = needSwap ? srcIp : dstIp;
        int port1 = needSwap ? dstPort : srcPort;
        int port2 = needSwap ? srcPort : dstPort;

        // 获取 ThreadLocal 的 MessageDigest
        MessageDigest sha1 = SHA1_DIGEST.get();
        sha1.reset();

        // 添加 seed (2 bytes, network order)
        ByteBuffer seedBuffer = ByteBuffer.allocate(2).order(ByteOrder.BIG_ENDIAN);
        seedBuffer.putShort((short) SEED);
        sha1.update(seedBuffer.array());

        // 添加 IP 地址
        sha1.update(ipToBytes(ip1));
        sha1.update(ipToBytes(ip2));

        // 添加协议 (1 byte)
        sha1.update((byte) protocol);

        // 添加填充字节 (1 byte)
        sha1.update((byte) 0);

        // 添加端口 (2 bytes each, network order)
        ByteBuffer portBuffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN);
        portBuffer.putShort((short) port1);
        portBuffer.putShort((short) port2);
        sha1.update(portBuffer.array());

        byte[] hash = sha1.digest();
        String base64Hash = Base64.getEncoder().encodeToString(hash);

        return COMMUNITY_ID_VERSION + ":" + base64Hash;
    }

    /**
     * 从五元组计算 Community ID（重载方法）
     */
    public static String compute(
            String srcIp, String dstIp,
            long srcPort, long dstPort,
            long protocol
    ) {
        return compute(srcIp, dstIp, (int) srcPort, (int) dstPort, (int) protocol);
    }

    /**
     * 验证 IP 地址格式
     */
    public static boolean isValidIp(String ip) {
        if (ip == null || ip.isEmpty()) {
            return false;
        }
        return isValidIPv4(ip) || isValidIPv6(ip);
    }

    /**
     * 验证 IPv4 地址格式
     */
    public static boolean isValidIPv4(String ip) {
        if (ip == null || ip.isEmpty()) {
            return false;
        }
        return IPV4_PATTERN.matcher(ip).matches();
    }

    /**
     * 验证 IPv6 地址格式
     */
    public static boolean isValidIPv6(String ip) {
        if (ip == null || ip.isEmpty()) {
            return false;
        }
        return IPV6_PATTERN.matcher(ip).matches();
    }

    /**
     * 判断是否需要交换源和目的
     */
    private static boolean shouldSwap(String srcIp, String dstIp, int srcPort, int dstPort) {
        int cmp = compareIp(srcIp, dstIp);
        if (cmp > 0) {
            return true;
        } else if (cmp < 0) {
            return false;
        } else {
            return srcPort > dstPort;
        }
    }

    /**
     * 比较两个 IP 地址
     */
    private static int compareIp(String ip1, String ip2) {
        byte[] bytes1 = ipToBytes(ip1);
        byte[] bytes2 = ipToBytes(ip2);

        // 不同长度（IPv4 vs IPv6）
        if (bytes1.length != bytes2.length) {
            return Integer.compare(bytes1.length, bytes2.length);
        }

        // 逐字节比较
        for (int i = 0; i < bytes1.length; i++) {
            int cmp = Integer.compare(bytes1[i] & 0xFF, bytes2[i] & 0xFF);
            if (cmp != 0) {
                return cmp;
            }
        }
        return 0;
    }

    /**
     * 将 IP 地址字符串转换为字节数组
     */
    private static byte[] ipToBytes(String ip) {
        if (ip == null || ip.isEmpty()) {
            return new byte[4];
        }

        if (ip.contains(":")) {
            return ipv6ToBytes(ip);
        } else {
            return ipv4ToBytes(ip);
        }
    }

    /**
     * IPv4 转字节数组
     */
    private static byte[] ipv4ToBytes(String ip) {
        byte[] bytes = new byte[4];
        String[] parts = ip.split("\\.");
        if (parts.length != 4) {
            return bytes;
        }
        try {
            for (int i = 0; i < 4; i++) {
                int value = Integer.parseInt(parts[i]);
                if (value < 0 || value > 255) {
                    return new byte[4];
                }
                bytes[i] = (byte) value;
            }
        } catch (NumberFormatException e) {
            // Return zero bytes on parse error
        }
        return bytes;
    }

    /**
     * IPv6 转字节数组
     */
    private static byte[] ipv6ToBytes(String ip) {
        byte[] bytes = new byte[16];

        // 处理 :: 压缩格式
        String expandedIp = expandIPv6(ip);
        String[] parts = expandedIp.split(":");

        int idx = 0;
        for (String part : parts) {
            if (part.isEmpty()) continue;
            try {
                int value = Integer.parseInt(part, 16);
                if (value < 0 || value > 0xFFFF) {
                    return new byte[16];
                }
                bytes[idx++] = (byte) ((value >> 8) & 0xFF);
                bytes[idx++] = (byte) (value & 0xFF);
            } catch (NumberFormatException e) {
                // Skip invalid parts
            }
            if (idx >= 16) break;
        }
        return bytes;
    }

    /**
     * 展开 IPv6 压缩格式
     */
    private static String expandIPv6(String ip) {
        if (!ip.contains("::")) {
            return ip;
        }

        // 处理 :: 在开头、中间、结尾的情况
        String[] halves = ip.split("::", -1);
        StringBuilder sb = new StringBuilder();

        String[] left = halves[0].isEmpty() ? new String[0] : halves[0].split(":");
        String[] right = halves.length > 1 && !halves[1].isEmpty()
                ? halves[1].split(":") : new String[0];

        int missing = 8 - left.length - right.length;

        for (String s : left) {
            sb.append(s).append(":");
        }
        for (int i = 0; i < missing; i++) {
            sb.append("0:");
        }
        for (int i = 0; i < right.length; i++) {
            sb.append(right[i]);
            if (i < right.length - 1) {
                sb.append(":");
            }
        }

        // 移除末尾多余的冒号
        String result = sb.toString();
        if (result.endsWith(":") && !result.endsWith("::")) {
            result = result.substring(0, result.length() - 1);
        }

        return result;
    }

    /**
     * 解析 Community ID 提取 Base64 部分
     * 
     * @param communityId Community ID 字符串（格式: "1:base64hash"）
     * @return Base64 编码的 hash 部分，如果格式无效返回 null
     */
    public static String extractHash(String communityId) {
        if (communityId == null || communityId.isEmpty()) {
            return null;
        }
        
        int colonIndex = communityId.indexOf(':');
        if (colonIndex < 0 || colonIndex >= communityId.length() - 1) {
            return null;
        }
        
        return communityId.substring(colonIndex + 1);
    }

    /**
     * 验证 Community ID 格式
     * 
     * @param communityId Community ID 字符串
     * @return 是否为有效格式
     */
    public static boolean isValidCommunityId(String communityId) {
        if (communityId == null || communityId.isEmpty()) {
            return false;
        }
        
        // 检查格式: "1:base64hash"
        if (!communityId.startsWith("1:")) {
            return false;
        }
        
        String hash = extractHash(communityId);
        if (hash == null || hash.isEmpty()) {
            return false;
        }
        
        // 验证 Base64 格式（SHA-1 hash 是 20 bytes，Base64 编码后约 28 字符）
        try {
            byte[] decoded = Base64.getDecoder().decode(hash);
            return decoded.length == 20; // SHA-1 hash 长度
        } catch (IllegalArgumentException e) {
            return false;
        }
    }
}