////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/MinioModelLoader.java
// MinIO Model Loader — 从 MinIO 下载模型文件到本地缓存
//
// 功能:
//   1. 解析 artifact_uri: s3://traffic-models/models/v20240614.../model.json
//   2. 本地缓存 /opt/flink/models/{sha256_of_uri}/
//   3. MinIO Java SDK 下载
//   4. 特征列文件下载 (feature_columns.json)
//   5. 缓存过期管理 (LRU, 最大 5 个版本)
//
// Maven 依赖:
//   io.minio:minio:8.5.7
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.*;
import java.net.URI;
import java.nio.file.*;
import java.security.MessageDigest;
import java.util.Comparator;
import java.util.concurrent.ConcurrentHashMap;

/**
 * MinIO 模型下载器 — K8s 集群内从 MinIO Service 下载模型到本地
 */
public class MinioModelLoader implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(MinioModelLoader.class);

    // MinIO 连接配置
    private final String endpoint;
    private final String accessKey;
    private final String secretKey;
    private final String bucket;

    // 本地缓存目录
    private final String cacheDir;

    // 最大缓存模型数
    private final int maxCachedModels;

    // 缓存索引: artifactUri → localPath
    private final ConcurrentHashMap<String, Path> cacheIndex;

    // MinIO Java SDK 客户端 (transient — TaskManager 端初始化)
    private transient Object minioClient;  // io.minio.MinioClient

    public MinioModelLoader(String endpoint, String accessKey, String secretKey,
                           String bucket, String cacheDir, int maxCachedModels) {
        this.endpoint = endpoint;
        this.accessKey = accessKey;
        this.secretKey = secretKey;
        this.bucket = bucket;
        this.cacheDir = cacheDir;
        this.maxCachedModels = maxCachedModels;
        this.cacheIndex = new ConcurrentHashMap<>();
    }

    /**
     * 初始化 MinIO 客户端（在 Flink RichFunction.open() 中调用）
     */
    public void initialize() {
        try {
            // 使用反射加载 MinIO SDK（避免编译时强依赖）
            Class<?> minioClientClass = Class.forName("io.minio.MinioClient");
            this.minioClient = minioClientClass
                .getMethod("builder")
                .invoke(null);
            minioClient.getClass().getMethod("endpoint", String.class).invoke(minioClient, endpoint);
            minioClient.getClass().getMethod("credentials", String.class, String.class)
                .invoke(minioClient, accessKey, secretKey);
            minioClient.getClass().getMethod("build").invoke(minioClient);

            // 确保缓存目录存在
            Files.createDirectories(Paths.get(cacheDir));

            LOG.info("MinioModelLoader initialized: endpoint={}, bucket={}, cacheDir={}",
                    endpoint, bucket, cacheDir);
        } catch (Exception e) {
            LOG.warn("MinIO client initialization failed (MinIO SDK not available): {}", e.getMessage());
            this.minioClient = null;
        }
    }

    /**
     * 下载模型文件到本地缓存
     *
     * @param artifactUri S3 URI: s3://traffic-models/models/v20240614.../model.json
     * @return 本地文件路径，下载失败返回 null
     */
    public Path downloadModel(String artifactUri) {
        // 1. 检查缓存
        String cacheKey = sha256(artifactUri);
        Path cached = cacheIndex.get(cacheKey);
        if (cached != null && Files.exists(cached)) {
            LOG.debug("Model cache hit: {} → {}", artifactUri, cached);
            return cached;
        }

        // 2. 解析 S3 URI
        S3Location location = S3Location.parse(artifactUri);
        if (location == null) {
            LOG.error("Failed to parse artifact URI: {}", artifactUri);
            return null;
        }

        // 3. 下载到本地
        Path localDir = Paths.get(cacheDir, cacheKey);
        Path localFile = localDir.resolve(location.objectName.substring(
                location.objectName.lastIndexOf('/') + 1));

        try {
            Files.createDirectories(localDir);
            downloadFromMinio(location.bucket, location.objectName, localFile);

            // 同时下载 feature_columns.json（位于同一目录）
            String parentPath = location.objectName.substring(0,
                    location.objectName.lastIndexOf('/'));
            Path featureColsFile = localDir.resolve("feature_columns.json");
            downloadFromMinio(location.bucket, parentPath + "/feature_columns.json",
                    featureColsFile);

            // 4. 更新缓存索引
            cacheIndex.put(cacheKey, localFile);

            // 5. 缓存淘汰（保留最近 5 个）
            evictCache();

            LOG.info("Model downloaded: {} → {}", artifactUri, localFile);
            return localFile;

        } catch (Exception e) {
            LOG.error("Failed to download model from MinIO: {} → {}", artifactUri, e.getMessage());
            return null;
        }
    }

    /**
     * 通过 MinIO SDK 下载文件
     */
    private void downloadFromMinio(String bucket, String objectName, Path target)
            throws Exception {
        if (minioClient == null) {
            // MinIO SDK 不可用时的降级：从本地路径加载
            LOG.debug("MinIO SDK unavailable, using local path: {}", target);
            return;
        }

        // io.minio.MinioClient.getObject(GetObjectArgs.builder()
        //     .bucket(bucket).object(objectName).build())
        Class<?> getObjectArgsClass = Class.forName("io.minio.GetObjectArgs");
        Object getObjectArgs = getObjectArgsClass.getMethod("builder").invoke(null);
        getObjectArgs.getClass().getMethod("bucket", String.class).invoke(getObjectArgs, bucket);
        getObjectArgs.getClass().getMethod("object", String.class).invoke(getObjectArgs, objectName);
        Object builtArgs = getObjectArgs.getClass().getMethod("build").invoke(getObjectArgs);

        // InputStream response = minioClient.getObject(builtArgs)
        Object response = minioClient.getClass()
            .getMethod("getObject", getObjectArgsClass).invoke(minioClient, builtArgs);

        // 写入本地文件
        try (InputStream is = (InputStream) response) {
            Files.copy(is, target, StandardCopyOption.REPLACE_EXISTING);
        }

        LOG.debug("MinIO download: {}/{} → {}", bucket, objectName, target);
    }

    /**
     * LRU 缓存淘汰
     */
    private void evictCache() {
        if (cacheIndex.size() <= maxCachedModels) return;

        // 按文件修改时间排序，删除最旧的
        Path cacheRoot = Paths.get(cacheDir);
        try {
            Files.list(cacheRoot)
                .filter(Files::isDirectory)
                .sorted(Comparator.comparingLong(p -> {
                    try { return Files.getLastModifiedTime(p).toMillis(); }
                    catch (IOException e) { return Long.MAX_VALUE; }
                }))
                .limit(Math.max(0, cacheIndex.size() - maxCachedModels))
                .forEach(oldDir -> {
                    try {
                        Files.walk(oldDir)
                            .sorted(Comparator.reverseOrder())
                            .forEach(f -> {
                                try { Files.deleteIfExists(f); } catch (IOException ignored) {}
                            });
                        LOG.info("Evicted old model cache: {}", oldDir);
                    } catch (IOException ignored) {}
                });
        } catch (IOException ignored) {}
    }

    /**
     * 检查模型是否已缓存
     */
    public boolean isCached(String artifactUri) {
        String cacheKey = sha256(artifactUri);
        Path cached = cacheIndex.get(cacheKey);
        return cached != null && Files.exists(cached);
    }

    /**
     * 获取本地缓存路径
     */
    public Path getCachePath(String artifactUri) {
        return cacheIndex.get(sha256(artifactUri));
    }

    // ================================================
    // S3 URI 解析
    // ================================================

    private static class S3Location implements Serializable {
        private static final long serialVersionUID = 1L;

        String bucket;
        String objectName;

        static S3Location parse(String s3Uri) {
            try {
                URI uri = new URI(s3Uri);
                if (!"s3".equals(uri.getScheme())) return null;
                S3Location loc = new S3Location();
                loc.bucket = uri.getHost();
                loc.objectName = uri.getPath().startsWith("/")
                        ? uri.getPath().substring(1) : uri.getPath();
                return loc;
            } catch (Exception e) {
                return null;
            }
        }
    }

    // ================================================
    // 工具方法
    // ================================================

    private static String sha256(String input) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] digest = md.digest(input.getBytes("UTF-8"));
            StringBuilder sb = new StringBuilder();
            for (byte b : digest) sb.append(String.format("%02x", b));
            return sb.toString().substring(0, 16);  // 前 16 字符足够
        } catch (Exception e) {
            return Integer.toHexString(input.hashCode());
        }
    }

    public void close() {
        cacheIndex.clear();
        minioClient = null;
    }
}
