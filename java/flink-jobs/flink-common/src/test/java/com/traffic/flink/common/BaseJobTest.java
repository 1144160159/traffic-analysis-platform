package com.traffic.flink.common;

import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * BaseJob 模板方法骨架测试:校验统一 checkpoint/重启纪律确实落到执行环境。
 */
public class BaseJobTest {

    private static final class FakeJob extends BaseJob {

        private static final long serialVersionUID = 1L;

        @Override
        protected String jobName() {
            return "fake-job";
        }

        @Override
        public void buildPipeline(StreamExecutionEnvironment env, JobParameters params) {
            // 无算子
        }
    }

    @Test
    public void createEnvironmentAppliesCheckpointAndRestartDefaults() {
        FakeJob job = new FakeJob();
        StreamExecutionEnvironment env = job.createEnvironment(new BaseJob.JobParameters());
        assertNotNull(env);
        assertEquals(60_000L, env.getCheckpointConfig().getCheckpointInterval());
        assertTrue(env.getCheckpointConfig().isExternalizedCheckpointsEnabled());
        assertEquals(1, env.getCheckpointConfig().getMaxConcurrentCheckpoints());
    }

    @Test
    public void jobParametersSupportExplicitOverridesAndDefaults() {
        BaseJob.JobParameters defaults = new BaseJob.JobParameters();
        assertEquals(60_000L, defaults.checkpointIntervalMsOrDefault(60_000L));
        assertEquals(3, defaults.restartAttempts());
        assertEquals(10L, defaults.restartDelaySeconds());

        BaseJob.JobParameters custom = new BaseJob.JobParameters()
                .checkpointIntervalMs(30_000L)
                .restartAttempts(5)
                .restartDelaySeconds(20L);
        assertEquals(30_000L, custom.checkpointIntervalMsOrDefault(60_000L));
        assertEquals(5, custom.restartAttempts());
        assertEquals(20L, custom.restartDelaySeconds());
    }
}
