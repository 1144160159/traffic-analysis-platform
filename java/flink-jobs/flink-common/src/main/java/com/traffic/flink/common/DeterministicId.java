package com.traffic.flink.common;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Collection;
import java.util.Collections;
import java.util.List;
import java.util.UUID;

/**
 * Replay-stable identifiers and sampling decisions.
 *
 * <p>Every component is encoded with an explicit byte length. This prevents
 * concatenation collisions such as {@code ["ab", "c"] == ["a", "bc"]} and
 * preserves the distinction between {@code null} and the empty string.</p>
 */
public final class DeterministicId {
    private static final String DIGEST_ALGORITHM = "SHA-256";

    private DeterministicId() {
    }

    /** Returns an RFC-4122-variant UUID using a SHA-256, version-8 payload. */
    public static String uuid(String namespace, Object... components) {
        byte[] hash = digest(namespace, components);
        hash[6] = (byte) ((hash[6] & 0x0f) | 0x80); // UUID version 8 (custom)
        hash[8] = (byte) ((hash[8] & 0x3f) | 0x80); // RFC-4122 variant
        ByteBuffer bytes = ByteBuffer.wrap(hash);
        return new UUID(bytes.getLong(), bytes.getLong()).toString();
    }

    /** Returns a stable hexadecimal prefix; useful for legacy human-readable IDs. */
    public static String shortId(String namespace, int characters, Object... components) {
        if (characters < 1 || characters > 64) {
            throw new IllegalArgumentException("characters must be between 1 and 64");
        }
        return hex(digest(namespace, components)).substring(0, characters);
    }

    /**
     * Builds a stable ID from a set of event IDs. Input order does not affect
     * the result, while duplicate IDs remain significant.
     */
    public static String uuidFromSorted(String namespace, Collection<String> eventIds, Object... context) {
        List<String> sortedIds = new ArrayList<>(eventIds == null
                ? Collections.emptyList()
                : eventIds);
        Collections.sort(sortedIds);
        List<Object> components = new ArrayList<>(context.length + sortedIds.size() + 1);
        Collections.addAll(components, context);
        components.add(sortedIds.size());
        components.addAll(sortedIds);
        return uuid(namespace, components.toArray());
    }

    /** Returns the same sampling decision for every replay of the same input. */
    public static boolean sample(double rate, String namespace, Object... components) {
        if (Double.isNaN(rate) || rate < 0.0d || rate > 1.0d) {
            throw new IllegalArgumentException("rate must be between 0 and 1");
        }
        if (rate == 0.0d) {
            return false;
        }
        if (rate == 1.0d) {
            return true;
        }
        long upper53Bits = ByteBuffer.wrap(digest(namespace, components)).getLong() >>> 11;
        double fraction = upper53Bits * 0x1.0p-53;
        return fraction < rate;
    }

    static byte[] digest(String namespace, Object... components) {
        if (namespace == null || namespace.trim().isEmpty()) {
            throw new IllegalArgumentException("namespace must not be blank");
        }
        try {
            MessageDigest digest = MessageDigest.getInstance(DIGEST_ALGORITHM);
            update(digest, namespace);
            digest.update(intBytes(components.length));
            for (Object component : components) {
                update(digest, component == null ? null : String.valueOf(component));
            }
            return digest.digest();
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 is required by the JVM", e);
        }
    }

    private static void update(MessageDigest digest, String value) {
        if (value == null) {
            digest.update(intBytes(-1));
            return;
        }
        byte[] bytes = value.getBytes(StandardCharsets.UTF_8);
        digest.update(intBytes(bytes.length));
        digest.update(bytes);
    }

    private static byte[] intBytes(int value) {
        return ByteBuffer.allocate(Integer.BYTES).putInt(value).array();
    }

    private static String hex(byte[] bytes) {
        StringBuilder result = new StringBuilder(bytes.length * 2);
        for (byte value : bytes) {
            result.append(String.format("%02x", value & 0xff));
        }
        return result.toString();
    }
}
