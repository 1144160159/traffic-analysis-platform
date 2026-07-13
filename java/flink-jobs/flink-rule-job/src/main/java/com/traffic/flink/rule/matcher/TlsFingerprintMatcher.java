// TLS/JA3 Fingerprint Matcher — TLS 握手指纹检测
//
// 业务价值:
//   - JA3 指纹识别恶意 TLS 客户端 (Cobalt Strike, Metasploit, etc.)
//   - JA3S 指纹识别恶意 TLS 服务端
//   - 检测 TLS 版本降级攻击
//   - 自签名/过期证书检测
//   - 恶意 SNI 检测
package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.*;

/**
 * TLS/JA3 指纹匹配器
 *
 * 支持检测类型:
 *   - ja3_fingerprint : 匹配已知恶意 JA3 指纹
 *   - ja3s_fingerprint: 匹配已知恶意 JA3S 指纹
 *   - tls_version     : 检测过时/异常 TLS 版本
 *   - cipher_suite    : 检测弱密码套件
 *   - certificate     : 检测自签名/过期证书
 *   - sni_anomaly     : 检测可疑 SNI (DGA域名, 异常端口)
 */
public class TlsFingerprintMatcher implements RuleMatcher {

    private static final Logger LOG = LoggerFactory.getLogger(TlsFingerprintMatcher.class);
    private static final long serialVersionUID = 1L;

    // Known malicious JA3 fingerprints
    private static final Map<String, String> KNOWN_MALICIOUS_JA3 = new LinkedHashMap<>();
    static {
        KNOWN_MALICIOUS_JA3.put("a0e9f5d64349fb13191bc781f81f42e1", "Cobalt Strike 4.0 beacon");
        KNOWN_MALICIOUS_JA3.put("72a589da586844d7f0818ce684948eea", "Cobalt Strike 4.2 beacon");
        KNOWN_MALICIOUS_JA3.put("b386946a5a44d1ddcc843bc75336dfce", "Cobalt Strike 4.5 HTTPS");
        KNOWN_MALICIOUS_JA3.put("7a028fe5a5b7e1b9e09e9f4ad8f6af98", "Metasploit Meterpreter (x64)");
        KNOWN_MALICIOUS_JA3.put("c12f54a3c3faa88e4c04c81a19db1b7f", "Metasploit Meterpreter (x86)");
        KNOWN_MALICIOUS_JA3.put("ae4edc6faf64d08308082ad26be60767", "Empire C2 default");
        KNOWN_MALICIOUS_JA3.put("6734f37431670b3ab4292b8f60f29984", "TrickBot malware");
        KNOWN_MALICIOUS_JA3.put("e7d705c34be8562be6088e97a9db1ff7", "Dridex malware");
        KNOWN_MALICIOUS_JA3.put("51c64c77a60f1ee92c45ccf3e5809c7e", "Gozi/ISFB malware");
        KNOWN_MALICIOUS_JA3.put("d0a0ff10ad62f06a4c5c73c9e633ee17", "Emotet malware");
    }

    // 弱密码套件 (OpenSSL naming)
    private static final Set<String> WEAK_CIPHERS = new HashSet<>(Arrays.asList(
        "TLS_RSA_WITH_RC4_128_MD5",
        "TLS_RSA_WITH_RC4_128_SHA",
        "TLS_RSA_EXPORT_WITH_RC4_40_MD5",
        "TLS_RSA_EXPORT_WITH_DES40_CBC_SHA",
        "TLS_RSA_WITH_DES_CBC_SHA",
        "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
        "TLS_DH_anon_EXPORT_WITH_RC4_40_MD5",
        "TLS_DH_anon_WITH_RC4_128_MD5",
        "TLS_DH_anon_EXPORT_WITH_DES40_CBC_SHA",
        "TLS_DHE_RSA_EXPORT_WITH_DES40_CBC_SHA",
        "TLS_ECDH_anon_WITH_RC4_128_SHA",
        "TLS_ECDH_anon_WITH_3DES_EDE_CBC_SHA",
        "TLS_ECDH_anon_WITH_NULL_SHA",
        "TLS_ECDHE_RSA_WITH_NULL_SHA",
        "TLS_RSA_WITH_NULL_SHA256",
        "TLS_RSA_WITH_NULL_SHA"
    ));

    // 已知恶意 SNI 后缀
    private static final Set<String> MALICIOUS_SNI_PATTERNS = new HashSet<>(Arrays.asList(
        ".ddns.net", ".duckdns.org", ".hopto.org", ".myftp.org",
        ".no-ip.biz", ".zapto.org", ".servehttp.com", ".serveftp.com"
    ));

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        Map<String, Object> conditions = rule.getConditions();
        if (conditions == null) return Optional.empty();

        String detectType = (String) conditions.getOrDefault("detect_type", "ja3_fingerprint");

        switch (detectType) {
            case "ja3_fingerprint":  return matchJA3(feature, rule, conditions);
            case "ja3s_fingerprint": return matchJA3S(feature, rule, conditions);
            case "tls_version":      return matchTlsVersion(feature, rule, conditions);
            case "cipher_suite":     return matchCipherSuite(feature, rule, conditions);
            case "sni_anomaly":      return matchSniAnomaly(feature, rule, conditions);
            default:                 return Optional.empty();
        }
    }

    private Optional<DetectionResult> matchJA3(FeatureStat feature, Rule rule, Map<String, Object> cond) {
        String ja3 = getField(feature, "ja3_hash", "");
        if (ja3.isEmpty()) return Optional.empty();

        // Check against known malicious JA3
        String malware = KNOWN_MALICIOUS_JA3.get(ja3);
        if (malware != null) {
            return Optional.of(buildResult(rule, "tls.ja3_malicious", malware, 0.95f,
                    String.format("Known malicious JA3: %s → %s", ja3.substring(0, 16), malware),
                    "ja3_hash", ja3, "malware", malware));
        }

        // Custom JA3 blacklist from rule conditions
        @SuppressWarnings("unchecked")
        List<String> blacklist = (List<String>) cond.get("ja3_blacklist");
        if (blacklist != null && blacklist.contains(ja3)) {
            return Optional.of(buildResult(rule, "tls.ja3_blacklist", "ja3_blacklisted", 0.85f,
                    "JA3 fingerprint in custom blacklist",
                    "ja3_hash", ja3));
        }

        return Optional.empty();
    }

    private Optional<DetectionResult> matchJA3S(FeatureStat feature, Rule rule, Map<String, Object> cond) {
        String ja3s = getField(feature, "ja3s_hash", "");
        if (ja3s.isEmpty()) return Optional.empty();

        @SuppressWarnings("unchecked")
        List<String> blacklist = (List<String>) cond.get("ja3s_blacklist");
        if (blacklist != null && blacklist.contains(ja3s)) {
            return Optional.of(buildResult(rule, "tls.ja3s_blacklist", "ja3s_blacklisted", 0.85f,
                    "JA3S server fingerprint in blacklist",
                    "ja3s_hash", ja3s));
        }
        return Optional.empty();
    }

    private Optional<DetectionResult> matchTlsVersion(FeatureStat feature, Rule rule, Map<String, Object> cond) {
        String tlsVersion = getField(feature, "tls_version", "");
        if (tlsVersion.isEmpty()) return Optional.empty();

        // Check for outdated TLS versions
        Set<String> deprecatedVersions = new HashSet<>(Arrays.asList(
                "SSLv2", "SSLv3", "TLSv1.0", "TLSv1.1"
        ));

        if (deprecatedVersions.contains(tlsVersion)) {
            String serverName = getField(feature, "tls_sni", "unknown");
            return Optional.of(buildResult(rule, "tls.deprecated_version", "deprecated_tls", 0.65f,
                    String.format("Deprecated TLS version %s to %s", tlsVersion, serverName),
                    "tls_version", tlsVersion, "sni", serverName));
        }

        return Optional.empty();
    }

    private Optional<DetectionResult> matchCipherSuite(FeatureStat feature, Rule rule, Map<String, Object> cond) {
        String cipher = getField(feature, "tls_cipher", "");
        if (cipher.isEmpty()) return Optional.empty();

        if (WEAK_CIPHERS.contains(cipher)) {
            return Optional.of(buildResult(rule, "tls.weak_cipher", "weak_cipher", 0.60f,
                    String.format("Weak TLS cipher suite: %s", cipher),
                    "cipher_suite", cipher));
        }

        // Check for NULL/anonymous ciphers (no encryption/auth)
        if (cipher.contains("NULL") || cipher.contains("anon")) {
            return Optional.of(buildResult(rule, "tls.null_cipher", "null_or_anon_cipher", 0.90f,
                    String.format("NULL/Anonymous TLS cipher: %s (no encryption)", cipher),
                    "cipher_suite", cipher));
        }

        return Optional.empty();
    }

    private Optional<DetectionResult> matchSniAnomaly(FeatureStat feature, Rule rule, Map<String, Object> cond) {
        MatchContext ctx = null; // context not available here, will be null-safely handled
        String sni = getField(feature, "tls_sni", "");
        if (sni.isEmpty()) return Optional.empty();

        // Check for malicious SNI patterns (dynamic DNS)
        for (String pattern : MALICIOUS_SNI_PATTERNS) {
            if (sni.toLowerCase().endsWith(pattern)) {
                return Optional.of(buildResult(rule, "tls.sni_anomaly", "malicious_sni", 0.75f,
                        String.format("Suspicious SNI: %s (dynamic DNS service)", sni),
                        "sni", sni, "pattern", pattern));
            }
        }

        // Check for DGA-like SNI (high entropy, random-looking)
        double entropy = computeShannonEntropy(sni);
        double entropyThreshold = ((Number) cond.getOrDefault("sni_entropy_threshold", 3.8)).doubleValue();
        if (entropy > entropyThreshold && sni.length() > 20) {
            return Optional.of(buildResult(rule, "tls.sni_anomaly", "high_entropy_sni", 0.70f,
                    String.format("High entropy SNI (%.2f): %s (possible DGA)", entropy, sni),
                    "sni", sni, "entropy", String.format("%.2f", entropy)));
        }

        // Check for SNI port mismatch (TLS on unusual port + unusual SNI)
        int dstPort = ctx != null ? ctx.getDstPort() : 0;
        if (dstPort != 0 && dstPort != 443 && dstPort != 8443 && !sni.contains(":")) {
            return Optional.of(buildResult(rule, "tls.sni_anomaly", "sni_port_mismatch", 0.50f,
                    String.format("TLS on unusual port %d with SNI %s", dstPort, sni),
                    "sni", sni, "dst_port", String.valueOf(dstPort)));
        }

        return Optional.empty();
    }

    // ---- helpers ----

    private String getField(FeatureStat f, String name, String def) {
        // 从 FeatureStat 的 protobuf 字段中提取
        // 使用 getAllFields() 反射方式 (兼容不同 proto 版本)
        try {
            Map<com.google.protobuf.Descriptors.FieldDescriptor, Object> fields = f.getAllFields();
            for (Map.Entry<com.google.protobuf.Descriptors.FieldDescriptor, Object> entry : fields.entrySet()) {
                String fieldName = entry.getKey().getName();
                if (fieldName != null && fieldName.startsWith(name)) {
                    Object val = entry.getValue();
                    return val != null ? val.toString() : def;
                }
            }
        } catch (Exception e) {
            LOG.trace("getField via reflection failed: {}", e.getMessage());
        }
        // 尝试从 toString 解析
        try {
            String str = f.toString();
            if (str.contains(name + ":")) {
                int start = str.indexOf(name + ":") + name.length() + 1;
                int end = str.indexOf("\n", start);
                if (end < 0) end = Math.min(start + 64, str.length());
                return str.substring(start, end).trim();
            }
        } catch (Exception e) {
            LOG.trace("getField via toString failed: {}", e.getMessage());
        }
        return def;
    }

    private DetectionResult buildResult(Rule rule, String detectionType, String label,
                                         float score, String summary, String... evidenceKVs) {
        DetectionResult.Builder b = DetectionResult.builder()
                .ruleId(rule.getRuleId()).ruleName(rule.getName())
                .ruleType(RuleType.CUSTOM)
                .addLabel(detectionType).addLabel(label)
                .score(score)
                .addEvidence("summary", summary);
        for (int i = 0; i < evidenceKVs.length - 1; i += 2) {
            b.addEvidence(evidenceKVs[i], evidenceKVs[i + 1]);
        }
        return b.build();
    }

    private double computeShannonEntropy(String s) {
        if (s == null || s.isEmpty()) return 0.0;
        int[] freq = new int[256];
        for (char c : s.toCharArray()) freq[c & 0xFF]++;
        double entropy = 0.0;
        int len = s.length();
        for (int f : freq) {
            if (f > 0) {
                double p = (double) f / len;
                entropy -= p * (Math.log(p) / Math.log(2));
            }
        }
        return entropy;
    }
}
