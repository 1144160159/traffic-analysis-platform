package com.traffic.flink.rule.source;

import com.fasterxml.jackson.databind.JsonNode;

import java.io.ByteArrayOutputStream;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Iterator;
import java.util.List;
import java.util.Map;

/** Cross-language typed canonical checksum for the nested RuleCommand rule. */
final class RuleCommandChecksum {

    static final String ALGORITHM = "md5-typed-canonical-json-v1";

    private RuleCommandChecksum() {
    }

    static String calculate(JsonNode rule) {
        if (rule == null || !rule.isObject()) {
            throw new IllegalArgumentException("wire rule object is required for checksum");
        }
        try {
            ByteArrayOutputStream canonical = new ByteArrayOutputStream();
            writeValue(canonical, rule);
            byte[] digest = MessageDigest.getInstance("MD5").digest(canonical.toByteArray());
            StringBuilder hex = new StringBuilder(digest.length * 2);
            for (byte value : digest) {
                hex.append(String.format("%02x", value & 0xff));
            }
            return hex.toString();
        } catch (IllegalArgumentException error) {
            throw error;
        } catch (Exception error) {
            throw new IllegalStateException("cannot calculate wire rule checksum", error);
        }
    }

    private static void writeValue(ByteArrayOutputStream target, JsonNode value) {
        if (value == null || value.isNull()) {
            target.write('z');
        } else if (value.isBoolean()) {
            target.write(value.booleanValue() ? 't' : 'f');
        } else if (value.isTextual()) {
            writeBytes(target, 's', value.textValue().getBytes(StandardCharsets.UTF_8));
        } else if (value.isNumber()) {
            writeBytes(target, 'd', normalizeNumber(value.asText()).getBytes(StandardCharsets.UTF_8));
        } else if (value.isArray()) {
            writeAscii(target, "a" + value.size() + ":");
            for (JsonNode item : value) {
                writeValue(target, item);
            }
        } else if (value.isObject()) {
            List<String> keys = new ArrayList<>();
            Iterator<Map.Entry<String, JsonNode>> fields = value.fields();
            while (fields.hasNext()) {
                keys.add(fields.next().getKey());
            }
            Collections.sort(keys);
            writeAscii(target, "o" + keys.size() + ":");
            for (String key : keys) {
                writeBytes(target, 's', key.getBytes(StandardCharsets.UTF_8));
                writeValue(target, value.get(key));
            }
        } else {
            throw new IllegalArgumentException(
                    "unsupported JSON checksum value " + value.getNodeType());
        }
    }

    private static void writeBytes(ByteArrayOutputStream target, int marker, byte[] value) {
        target.write(marker);
        writeAscii(target, Integer.toString(value.length));
        target.write(':');
        target.write(value, 0, value.length);
    }

    private static void writeAscii(ByteArrayOutputStream target, String value) {
        byte[] bytes = value.getBytes(StandardCharsets.US_ASCII);
        target.write(bytes, 0, bytes.length);
    }

    static String normalizeNumber(String raw) {
        String value = raw == null ? "" : raw.trim();
        if (value.isEmpty()) {
            throw new IllegalArgumentException("empty JSON number");
        }
        String sign = "";
        if (value.charAt(0) == '-') {
            sign = "-";
            value = value.substring(1);
        }

        int exponent = 0;
        int separator = Math.max(value.indexOf('e'), value.indexOf('E'));
        if (separator >= 0) {
            exponent = new BigInteger(value.substring(separator + 1)).intValueExact();
            value = value.substring(0, separator);
        }
        String[] parts = value.split("\\.", -1);
        if (parts.length > 2 || parts[0].isEmpty()
                || (parts.length == 2 && parts[1].isEmpty())) {
            throw new IllegalArgumentException("invalid JSON number");
        }
        String digits = parts[0] + (parts.length == 2 ? parts[1] : "");
        for (int index = 0; index < digits.length(); index++) {
            if (!Character.isDigit(digits.charAt(index))) {
                throw new IllegalArgumentException("invalid JSON number digit");
            }
        }
        int first = 0;
        while (first < digits.length() && digits.charAt(first) == '0') {
            first++;
        }
        if (first == digits.length()) {
            return "0";
        }
        int last = digits.length() - 1;
        while (last > first && digits.charAt(last) == '0') {
            last--;
        }
        String significant = digits.substring(first, last + 1);
        int scientificExponent = Math.addExact(
                Math.subtractExact(parts[0].length(), first + 1), exponent);
        StringBuilder normalized = new StringBuilder(sign)
                .append(significant.charAt(0));
        if (significant.length() > 1) {
            normalized.append('.').append(significant.substring(1));
        }
        return normalized.append('e').append(scientificExponent).toString();
    }
}
