package com.traffic.flink.common;

import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;

import java.io.Serializable;
import java.util.concurrent.TimeUnit;

/**
 * Flink 作业骨架(GoF Template Method 落点)。
 *
 * 目标:收敛各作业 main 中重复的 checkpoint/重启策略/水印样板,把"稳定 uid +
 * state TTL + RocksDB + checkpoint 30-60s + timeout ≤ 10min"的纪律从口头约定
 * 变成可执行模板。作业 main 继承本类并实现 buildPipeline(env, params),
 * 逐步替换手写样板;既有作业可先保持现状,新作业一律继承本类。
 */
public abstract class BaseJob implements Serializable {

    private static final long serialVersionUID = 1L;

    /** 作业名(用于日志与默认命名前缀)。 */
    protected abstract String jobName();

    /**
     * 模板方法:创建并按统一纪律配置执行环境。
     * 子类可通过 configure(env, params) 追加自定义配置。
     */
    public final StreamExecutionEnvironment createEnvironment(JobParameters params) {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        long intervalMs = params.checkpointIntervalMsOrDefault(defaultCheckpointIntervalMs());
        configureCheckpointing(env, intervalMs);
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                params.restartAttempts(),
                Time.of(params.restartDelaySeconds(), TimeUnit.SECONDS)));
        configure(env, params);
        return env;
    }

    /** 统一 checkpoint 纪律:EXACTLY_ONCE、外部化、单并发、超时 ≤ 10min。 */
    protected void configureCheckpointing(StreamExecutionEnvironment env, long intervalMs) {
        env.enableCheckpointing(intervalMs, CheckpointingMode.EXACTLY_ONCE);
        CheckpointConfig cfg = env.getCheckpointConfig();
        cfg.setCheckpointTimeout(Math.min(intervalMs * 10L, TimeUnit.MINUTES.toMillis(10)));
        cfg.setMinPauseBetweenCheckpoints(intervalMs);
        cfg.setMaxConcurrentCheckpoints(1);
        cfg.enableExternalizedCheckpoints(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION);
    }

    /** 默认 checkpoint 间隔 60s(agent.md:30-60s)。 */
    protected long defaultCheckpointIntervalMs() {
        return 60_000L;
    }

    /** 子类钩子:追加自定义环境配置(默认无操作)。 */
    protected void configure(StreamExecutionEnvironment env, JobParameters params) {
        // 默认无操作
    }

    /** 子类必须实现:构建算子管线(每个算子设置稳定 uid、state TTL、RocksDB)。 */
    public abstract void buildPipeline(StreamExecutionEnvironment env, JobParameters params)
            throws Exception;

    /** 作业级配置参数统一承载(避免各作业自行散落魔法值)。 */
    public static class JobParameters implements Serializable {

        private static final long serialVersionUID = 1L;

        private Long checkpointIntervalMs;
        private String checkpointPath;
        private Integer restartAttempts = 3;
        private Long restartDelaySeconds = 10L;

        public JobParameters checkpointIntervalMs(long value) {
            this.checkpointIntervalMs = value;
            return this;
        }

        public long checkpointIntervalMsOrDefault(long defaultValue) {
            return checkpointIntervalMs == null ? defaultValue : checkpointIntervalMs;
        }

        public JobParameters checkpointPath(String value) {
            this.checkpointPath = value;
            return this;
        }

        public String checkpointPath() {
            return checkpointPath;
        }

        public JobParameters restartAttempts(int value) {
            this.restartAttempts = value;
            return this;
        }

        public int restartAttempts() {
            return restartAttempts;
        }

        public JobParameters restartDelaySeconds(long value) {
            this.restartDelaySeconds = value;
            return this;
        }

        public long restartDelaySeconds() {
            return restartDelaySeconds;
        }
    }
}
