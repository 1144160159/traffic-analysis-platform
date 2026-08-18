package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.BehaviorModel;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.functions.FlatMapFunction;
import org.apache.flink.util.Collector;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Simple synchronous behavior detector used for testing/debugging mode.
 */
public class SyncBehaviorDetector implements FlatMapFunction<FeatureStat, DetectionBehavior> {

	private static final long serialVersionUID = 1L;
	private static final Logger LOG = LoggerFactory.getLogger(SyncBehaviorDetector.class);

	private final BehaviorJobConfig config;
	private final ModelRegistry registry;

	public SyncBehaviorDetector(BehaviorJobConfig config, ModelRegistry registry) {
		this.config = config;
		this.registry = registry;
	}

	@Override
	public void flatMap(FeatureStat value, Collector<DetectionBehavior> out) throws Exception {
		if (value == null) {
			return;
		}

		List<ModelInferenceResult> results = runAllModels(value);
		ModelInferenceResult bestResult = selectBestResult(results);
		if (bestResult == null) {
			return;
		}

		if (bestResult.isDetected() || config.isDebugPrintEnabled()) {
			out.collect(toDetectionBehavior(value, bestResult));
		}
	}

	private List<ModelInferenceResult> runAllModels(FeatureStat feature) {
		List<ModelInferenceResult> results = new ArrayList<>();
		String tenantId = feature.hasHeader() ? feature.getHeader().getTenantId() : "";
		for (Map.Entry<String, BehaviorModel> entry : registry.getModelsForTenant(tenantId).entrySet()) {
			String modelName = entry.getKey();
			BehaviorModel model = entry.getValue();
			try {
				if (model.isReady()) {
					ModelInferenceResult result = model.infer(feature);
					if (result != null && !result.hasError()) {
						results.add(result);
						registry.recordInvocation(modelName);
					}
				}
			} catch (Exception e) {
				LOG.warn("Model {} inference failed: {}", modelName, e.getMessage());
				registry.recordError(modelName);
			}
		}
		return results;
	}

	private ModelInferenceResult selectBestResult(List<ModelInferenceResult> results) {
		if (results == null || results.isEmpty()) {
			return null;
		}

		ModelInferenceResult bestResult = null;
		float bestScore = 0.0f;
		for (ModelInferenceResult result : results) {
			if (result.isDetected() && result.getTopScore() > bestScore) {
				bestScore = result.getTopScore();
				bestResult = result;
			}
		}

		if (bestResult == null) {
			for (ModelInferenceResult result : results) {
				if (result.getTopScore() > bestScore) {
					bestScore = result.getTopScore();
					bestResult = result;
				}
			}
		}
		return bestResult;
	}

	private DetectionBehavior toDetectionBehavior(FeatureStat input, ModelInferenceResult result) {
		String tenantId = input.hasHeader() ? input.getHeader().getTenantId() : "";
		String modelVersion = registry.getModelVersion(tenantId, result.getModelName());
		if (modelVersion == null || modelVersion.isEmpty()) {
			modelVersion = result.getModelVersion();
		}
		return BehaviorDetectionEventFactory.build(
				input, result, modelVersion, System.currentTimeMillis());
	}
}
