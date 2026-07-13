package com.traffic.flink.behavior.model;

import com.traffic.proto.traffic.v1.FeatureStat;

import java.io.Serializable;
import java.util.List;

/**
 * 行为检测模型接口
 * 
 * 所有行为检测模型必须实现此接口
 */
public interface BehaviorModel extends Serializable, AutoCloseable {

    /**
     * 获取模型名称
     */
    String getName();

    /**
     * 获取模型版本
     */
    String getVersion();

    /**
     * 获取模型描述
     */
    String getDescription();

    /**
     * 初始化模型
     * 
     * @throws Exception 初始化失败时抛出异常
     */
    void initialize() throws Exception;

    /**
     * 模型是否就绪
     */
    boolean isReady();

    /**
     * 对单个特征进行推理
     * 
     * @param feature 输入特征
     * @return 推理结果
     */
    ModelInferenceResult infer(FeatureStat feature);

    /**
     * 批量推理
     * 
     * @param features 特征列表
     * @return 推理结果列表
     */
    List<ModelInferenceResult> inferBatch(List<FeatureStat> features);

    /**
     * 检查是否有模型更新
     * 
     * @return 最新版本号，如果无更新返回 null
     */
    String checkForUpdate();

    /**
     * 重新加载模型
     * 
     * @return 新的模型实例
     * @throws Exception 重加载失败时抛出异常
     */
    BehaviorModel reload() throws Exception;

    /**
     * 获取模型支持的标签列表
     */
    List<String> getSupportedLabels();

    /**
     * 获取模型阈值
     */
    float getThreshold();

    /**
     * 关闭模型，释放资源
     */
    @Override
    void close() throws Exception;
}