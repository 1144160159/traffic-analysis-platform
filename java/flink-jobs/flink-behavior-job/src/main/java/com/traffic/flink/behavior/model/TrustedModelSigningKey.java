package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import org.bouncycastle.jce.provider.BouncyCastleProvider;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.security.Security;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;

/** Loads the independently delivered Ed25519 model-signing trust root. */
public final class TrustedModelSigningKey {
    private final PublicKey publicKey;
    private final String sha256;

    private TrustedModelSigningKey(PublicKey publicKey, String sha256) {
        this.publicKey = publicKey;
        this.sha256 = sha256;
    }

    public static TrustedModelSigningKey fromConfig(BehaviorJobConfig config) {
        try {
            if (Security.getProvider("BC") == null) {
                Security.addProvider(new BouncyCastleProvider());
            }
            String pem;
            if (config.getModelSigningPublicKeyFile() != null
                    && !config.getModelSigningPublicKeyFile().isBlank()) {
                pem = Files.readString(Path.of(config.getModelSigningPublicKeyFile()));
            } else {
                pem = new String(Base64.getDecoder().decode(
                        config.getModelSigningPublicKeyPemBase64()), StandardCharsets.US_ASCII);
            }
            String body = pem.replace("-----BEGIN PUBLIC KEY-----", "")
                    .replace("-----END PUBLIC KEY-----", "").replaceAll("\\s", "");
            PublicKey key = KeyFactory.getInstance("Ed25519", "BC")
                    .generatePublic(new X509EncodedKeySpec(Base64.getDecoder().decode(body)));
            return new TrustedModelSigningKey(key, sha256(key.getEncoded()));
        } catch (Exception error) {
            throw new IllegalArgumentException("trusted model signing public key is invalid", error);
        }
    }

    public PublicKey publicKey() {
        return publicKey;
    }

    public String sha256() {
        return sha256;
    }

    private static String sha256(byte[] value) throws Exception {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(value);
        StringBuilder result = new StringBuilder(64);
        for (byte item : digest) {
            result.append(String.format("%02x", item));
        }
        return result.toString();
    }
}
