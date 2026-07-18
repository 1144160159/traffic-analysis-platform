package com.traffic.flink.behavior.model;

import org.apache.flink.core.fs.FSDataInputStream;
import org.apache.flink.core.fs.FileSystem;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.io.InputStream;
import java.io.Serializable;
import java.net.URI;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.StandardCopyOption;
import java.security.MessageDigest;
import java.util.Comparator;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Downloads immutable model artifacts through Flink's configured filesystem.
 * The production cluster already configures the S3 filesystem for MinIO, so
 * this loader uses the same credentials and endpoint as checkpoints.
 */
public class MinioModelLoader implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(MinioModelLoader.class);

    private final String cacheDir;
    private final int maxCachedModels;
    private final ConcurrentHashMap<String, Path> cacheIndex = new ConcurrentHashMap<>();

    public MinioModelLoader(String cacheDir, int maxCachedModels) {
        this.cacheDir = cacheDir;
        this.maxCachedModels = Math.max(1, maxCachedModels);
    }

    /** Compatibility constructor retained for older callers. */
    public MinioModelLoader(String endpoint, String accessKey, String secretKey,
                            String bucket, String cacheDir, int maxCachedModels) {
        this(cacheDir, maxCachedModels);
    }

    public void initialize() {
        try {
            Files.createDirectories(Paths.get(cacheDir));
        } catch (IOException e) {
            throw new IllegalStateException("Cannot create model cache directory " + cacheDir, e);
        }
    }

    /** Downloads the model plus the required sibling feature_columns.json. */
    public Path downloadModel(String artifactUri) {
        if (artifactUri == null || artifactUri.isBlank()) {
            throw new IllegalArgumentException("artifact_uri is required");
        }
        String cacheKey = sha256Text(artifactUri);
        Path cached = cacheIndex.get(cacheKey);
        if (isUsable(cached) && isUsable(cached.getParent().resolve("feature_columns.json"))) {
            return cached;
        }

        URI uri = URI.create(artifactUri);
        String fileName = Paths.get(uri.getPath()).getFileName().toString();
        Path localDir = Paths.get(cacheDir, cacheKey);
        Path localModel = localDir.resolve(fileName);
        Path localColumns = localDir.resolve("feature_columns.json");

        try {
            Files.createDirectories(localDir);
            copyFromFlinkFileSystem(artifactUri, localModel);
            String remoteColumns = artifactUri.substring(0, artifactUri.lastIndexOf('/') + 1)
                    + "feature_columns.json";
            copyFromFlinkFileSystem(remoteColumns, localColumns);
            if (!isUsable(localModel) || !isUsable(localColumns)) {
                throw new IOException("model artifact or feature_columns.json is empty");
            }
            cacheIndex.put(cacheKey, localModel);
            evictCache();
            LOG.info("Validated model artifact download: uri={}, local={}, sha256={}",
                    artifactUri, localModel, sha256(localModel));
            return localModel;
        } catch (Exception e) {
            throw new IllegalStateException("Failed to download model artifact " + artifactUri, e);
        }
    }

    private static void copyFromFlinkFileSystem(String sourceUri, Path target) throws IOException {
        org.apache.flink.core.fs.Path remote = new org.apache.flink.core.fs.Path(sourceUri);
        FileSystem fileSystem = remote.getFileSystem();
        try (FSDataInputStream input = fileSystem.open(remote)) {
            Files.copy(input, target, StandardCopyOption.REPLACE_EXISTING);
        }
    }

    public String verifySha256(Path artifact, String expectedSha256) {
        String actual = sha256(artifact);
        if (expectedSha256 != null && !expectedSha256.isBlank()
                && !actual.equalsIgnoreCase(expectedSha256.trim())) {
            throw new IllegalStateException("Artifact SHA-256 mismatch: expected="
                    + expectedSha256 + ", actual=" + actual);
        }
        return actual;
    }

    public Path getFeatureColumnsPath(Path artifact) {
        Path columns = artifact.getParent().resolve("feature_columns.json");
        if (!isUsable(columns)) {
            throw new IllegalStateException("feature_columns.json is missing for " + artifact);
        }
        return columns;
    }

    private static boolean isUsable(Path path) {
        try {
            return path != null && Files.isRegularFile(path) && Files.size(path) > 0;
        } catch (IOException e) {
            return false;
        }
    }

    private void evictCache() {
        if (cacheIndex.size() <= maxCachedModels) {
            return;
        }
        try (java.util.stream.Stream<Path> dirs = Files.list(Paths.get(cacheDir))) {
            dirs.filter(Files::isDirectory)
                    .sorted(Comparator.comparingLong(MinioModelLoader::lastModified))
                    .limit(cacheIndex.size() - maxCachedModels)
                    .forEach(MinioModelLoader::deleteTree);
        } catch (IOException e) {
            LOG.warn("Unable to evict model cache: {}", e.getMessage());
        }
        cacheIndex.entrySet().removeIf(entry -> !isUsable(entry.getValue()));
    }

    private static long lastModified(Path path) {
        try {
            return Files.getLastModifiedTime(path).toMillis();
        } catch (IOException e) {
            return Long.MAX_VALUE;
        }
    }

    private static void deleteTree(Path root) {
        try (java.util.stream.Stream<Path> paths = Files.walk(root)) {
            paths.sorted(Comparator.reverseOrder()).forEach(path -> {
                try {
                    Files.deleteIfExists(path);
                } catch (IOException ignored) {
                    // Best effort cache cleanup.
                }
            });
        } catch (IOException ignored) {
            // Best effort cache cleanup.
        }
    }

    public static String sha256(Path file) {
        try (InputStream input = Files.newInputStream(file)) {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] buffer = new byte[8192];
            int read;
            while ((read = input.read(buffer)) >= 0) {
                digest.update(buffer, 0, read);
            }
            return toHex(digest.digest());
        } catch (Exception e) {
            throw new IllegalStateException("Cannot calculate SHA-256 for " + file, e);
        }
    }

    private static String sha256Text(String value) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return toHex(digest.digest(value.getBytes(java.nio.charset.StandardCharsets.UTF_8)))
                    .substring(0, 16);
        } catch (Exception e) {
            throw new IllegalStateException(e);
        }
    }

    private static String toHex(byte[] bytes) {
        StringBuilder value = new StringBuilder(bytes.length * 2);
        for (byte item : bytes) {
            value.append(String.format("%02x", item));
        }
        return value.toString();
    }

    public void close() {
        cacheIndex.clear();
    }
}
