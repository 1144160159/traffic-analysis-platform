package com.traffic.proto.traffic.v1;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 */
@javax.annotation.Generated(
    value = "by gRPC proto compiler (version 1.61.0)",
    comments = "Source: traffic/v1/ingest.proto")
@io.grpc.stub.annotations.GrpcGenerated
public final class IngestServiceGrpc {

  private IngestServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "traffic.v1.IngestService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadFlowsRequest,
      com.traffic.proto.traffic.v1.UploadFlowsResponse> getUploadFlowsMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "UploadFlows",
      requestType = com.traffic.proto.traffic.v1.UploadFlowsRequest.class,
      responseType = com.traffic.proto.traffic.v1.UploadFlowsResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadFlowsRequest,
      com.traffic.proto.traffic.v1.UploadFlowsResponse> getUploadFlowsMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadFlowsRequest, com.traffic.proto.traffic.v1.UploadFlowsResponse> getUploadFlowsMethod;
    if ((getUploadFlowsMethod = IngestServiceGrpc.getUploadFlowsMethod) == null) {
      synchronized (IngestServiceGrpc.class) {
        if ((getUploadFlowsMethod = IngestServiceGrpc.getUploadFlowsMethod) == null) {
          IngestServiceGrpc.getUploadFlowsMethod = getUploadFlowsMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.UploadFlowsRequest, com.traffic.proto.traffic.v1.UploadFlowsResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "UploadFlows"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UploadFlowsRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UploadFlowsResponse.getDefaultInstance()))
              .setSchemaDescriptor(new IngestServiceMethodDescriptorSupplier("UploadFlows"))
              .build();
        }
      }
    }
    return getUploadFlowsMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.StreamFlowsRequest,
      com.traffic.proto.traffic.v1.StreamFlowsResponse> getStreamFlowsMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "StreamFlows",
      requestType = com.traffic.proto.traffic.v1.StreamFlowsRequest.class,
      responseType = com.traffic.proto.traffic.v1.StreamFlowsResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.BIDI_STREAMING)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.StreamFlowsRequest,
      com.traffic.proto.traffic.v1.StreamFlowsResponse> getStreamFlowsMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.StreamFlowsRequest, com.traffic.proto.traffic.v1.StreamFlowsResponse> getStreamFlowsMethod;
    if ((getStreamFlowsMethod = IngestServiceGrpc.getStreamFlowsMethod) == null) {
      synchronized (IngestServiceGrpc.class) {
        if ((getStreamFlowsMethod = IngestServiceGrpc.getStreamFlowsMethod) == null) {
          IngestServiceGrpc.getStreamFlowsMethod = getStreamFlowsMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.StreamFlowsRequest, com.traffic.proto.traffic.v1.StreamFlowsResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.BIDI_STREAMING)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "StreamFlows"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.StreamFlowsRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.StreamFlowsResponse.getDefaultInstance()))
              .setSchemaDescriptor(new IngestServiceMethodDescriptorSupplier("StreamFlows"))
              .build();
        }
      }
    }
    return getStreamFlowsMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadSessionsRequest,
      com.traffic.proto.traffic.v1.UploadSessionsResponse> getUploadSessionsMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "UploadSessions",
      requestType = com.traffic.proto.traffic.v1.UploadSessionsRequest.class,
      responseType = com.traffic.proto.traffic.v1.UploadSessionsResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadSessionsRequest,
      com.traffic.proto.traffic.v1.UploadSessionsResponse> getUploadSessionsMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadSessionsRequest, com.traffic.proto.traffic.v1.UploadSessionsResponse> getUploadSessionsMethod;
    if ((getUploadSessionsMethod = IngestServiceGrpc.getUploadSessionsMethod) == null) {
      synchronized (IngestServiceGrpc.class) {
        if ((getUploadSessionsMethod = IngestServiceGrpc.getUploadSessionsMethod) == null) {
          IngestServiceGrpc.getUploadSessionsMethod = getUploadSessionsMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.UploadSessionsRequest, com.traffic.proto.traffic.v1.UploadSessionsResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "UploadSessions"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UploadSessionsRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UploadSessionsResponse.getDefaultInstance()))
              .setSchemaDescriptor(new IngestServiceMethodDescriptorSupplier("UploadSessions"))
              .build();
        }
      }
    }
    return getUploadSessionsMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadPcapIndexRequest,
      com.traffic.proto.traffic.v1.UploadPcapIndexResponse> getUploadPcapIndexMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "UploadPcapIndex",
      requestType = com.traffic.proto.traffic.v1.UploadPcapIndexRequest.class,
      responseType = com.traffic.proto.traffic.v1.UploadPcapIndexResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadPcapIndexRequest,
      com.traffic.proto.traffic.v1.UploadPcapIndexResponse> getUploadPcapIndexMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UploadPcapIndexRequest, com.traffic.proto.traffic.v1.UploadPcapIndexResponse> getUploadPcapIndexMethod;
    if ((getUploadPcapIndexMethod = IngestServiceGrpc.getUploadPcapIndexMethod) == null) {
      synchronized (IngestServiceGrpc.class) {
        if ((getUploadPcapIndexMethod = IngestServiceGrpc.getUploadPcapIndexMethod) == null) {
          IngestServiceGrpc.getUploadPcapIndexMethod = getUploadPcapIndexMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.UploadPcapIndexRequest, com.traffic.proto.traffic.v1.UploadPcapIndexResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "UploadPcapIndex"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UploadPcapIndexRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UploadPcapIndexResponse.getDefaultInstance()))
              .setSchemaDescriptor(new IngestServiceMethodDescriptorSupplier("UploadPcapIndex"))
              .build();
        }
      }
    }
    return getUploadPcapIndexMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.HeartbeatRequest,
      com.traffic.proto.traffic.v1.HeartbeatResponse> getHeartbeatMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Heartbeat",
      requestType = com.traffic.proto.traffic.v1.HeartbeatRequest.class,
      responseType = com.traffic.proto.traffic.v1.HeartbeatResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.HeartbeatRequest,
      com.traffic.proto.traffic.v1.HeartbeatResponse> getHeartbeatMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.HeartbeatRequest, com.traffic.proto.traffic.v1.HeartbeatResponse> getHeartbeatMethod;
    if ((getHeartbeatMethod = IngestServiceGrpc.getHeartbeatMethod) == null) {
      synchronized (IngestServiceGrpc.class) {
        if ((getHeartbeatMethod = IngestServiceGrpc.getHeartbeatMethod) == null) {
          IngestServiceGrpc.getHeartbeatMethod = getHeartbeatMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.HeartbeatRequest, com.traffic.proto.traffic.v1.HeartbeatResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "Heartbeat"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.HeartbeatRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.HeartbeatResponse.getDefaultInstance()))
              .setSchemaDescriptor(new IngestServiceMethodDescriptorSupplier("Heartbeat"))
              .build();
        }
      }
    }
    return getHeartbeatMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.RegisterProbeRequest,
      com.traffic.proto.traffic.v1.RegisterProbeResponse> getRegisterProbeMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "RegisterProbe",
      requestType = com.traffic.proto.traffic.v1.RegisterProbeRequest.class,
      responseType = com.traffic.proto.traffic.v1.RegisterProbeResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.RegisterProbeRequest,
      com.traffic.proto.traffic.v1.RegisterProbeResponse> getRegisterProbeMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.RegisterProbeRequest, com.traffic.proto.traffic.v1.RegisterProbeResponse> getRegisterProbeMethod;
    if ((getRegisterProbeMethod = IngestServiceGrpc.getRegisterProbeMethod) == null) {
      synchronized (IngestServiceGrpc.class) {
        if ((getRegisterProbeMethod = IngestServiceGrpc.getRegisterProbeMethod) == null) {
          IngestServiceGrpc.getRegisterProbeMethod = getRegisterProbeMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.RegisterProbeRequest, com.traffic.proto.traffic.v1.RegisterProbeResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "RegisterProbe"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.RegisterProbeRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.RegisterProbeResponse.getDefaultInstance()))
              .setSchemaDescriptor(new IngestServiceMethodDescriptorSupplier("RegisterProbe"))
              .build();
        }
      }
    }
    return getRegisterProbeMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static IngestServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<IngestServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<IngestServiceStub>() {
        @java.lang.Override
        public IngestServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new IngestServiceStub(channel, callOptions);
        }
      };
    return IngestServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static IngestServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<IngestServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<IngestServiceBlockingStub>() {
        @java.lang.Override
        public IngestServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new IngestServiceBlockingStub(channel, callOptions);
        }
      };
    return IngestServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static IngestServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<IngestServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<IngestServiceFutureStub>() {
        @java.lang.Override
        public IngestServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new IngestServiceFutureStub(channel, callOptions);
        }
      };
    return IngestServiceFutureStub.newStub(factory, channel);
  }

  /**
   */
  public interface AsyncService {

    /**
     * <pre>
     * 批量上报 Flow 事件
     * </pre>
     */
    default void uploadFlows(com.traffic.proto.traffic.v1.UploadFlowsRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadFlowsResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getUploadFlowsMethod(), responseObserver);
    }

    /**
     * <pre>
     * 流式上报 Flow 事件（双向流）
     * </pre>
     */
    default io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.StreamFlowsRequest> streamFlows(
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.StreamFlowsResponse> responseObserver) {
      return io.grpc.stub.ServerCalls.asyncUnimplementedStreamingCall(getStreamFlowsMethod(), responseObserver);
    }

    /**
     * <pre>
     * 批量上报 Session 事件
     * </pre>
     */
    default void uploadSessions(com.traffic.proto.traffic.v1.UploadSessionsRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadSessionsResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getUploadSessionsMethod(), responseObserver);
    }

    /**
     * <pre>
     * 上报 PCAP 索引元数据
     * </pre>
     */
    default void uploadPcapIndex(com.traffic.proto.traffic.v1.UploadPcapIndexRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadPcapIndexResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getUploadPcapIndexMethod(), responseObserver);
    }

    /**
     * <pre>
     * 探针心跳与配置同步
     * </pre>
     */
    default void heartbeat(com.traffic.proto.traffic.v1.HeartbeatRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.HeartbeatResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getHeartbeatMethod(), responseObserver);
    }

    /**
     * <pre>
     * 探针注册
     * </pre>
     */
    default void registerProbe(com.traffic.proto.traffic.v1.RegisterProbeRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.RegisterProbeResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getRegisterProbeMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service IngestService.
   */
  public static abstract class IngestServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return IngestServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service IngestService.
   */
  public static final class IngestServiceStub
      extends io.grpc.stub.AbstractAsyncStub<IngestServiceStub> {
    private IngestServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected IngestServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new IngestServiceStub(channel, callOptions);
    }

    /**
     * <pre>
     * 批量上报 Flow 事件
     * </pre>
     */
    public void uploadFlows(com.traffic.proto.traffic.v1.UploadFlowsRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadFlowsResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getUploadFlowsMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * 流式上报 Flow 事件（双向流）
     * </pre>
     */
    public io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.StreamFlowsRequest> streamFlows(
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.StreamFlowsResponse> responseObserver) {
      return io.grpc.stub.ClientCalls.asyncBidiStreamingCall(
          getChannel().newCall(getStreamFlowsMethod(), getCallOptions()), responseObserver);
    }

    /**
     * <pre>
     * 批量上报 Session 事件
     * </pre>
     */
    public void uploadSessions(com.traffic.proto.traffic.v1.UploadSessionsRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadSessionsResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getUploadSessionsMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * 上报 PCAP 索引元数据
     * </pre>
     */
    public void uploadPcapIndex(com.traffic.proto.traffic.v1.UploadPcapIndexRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadPcapIndexResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getUploadPcapIndexMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * 探针心跳与配置同步
     * </pre>
     */
    public void heartbeat(com.traffic.proto.traffic.v1.HeartbeatRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.HeartbeatResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getHeartbeatMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * 探针注册
     * </pre>
     */
    public void registerProbe(com.traffic.proto.traffic.v1.RegisterProbeRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.RegisterProbeResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getRegisterProbeMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service IngestService.
   */
  public static final class IngestServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<IngestServiceBlockingStub> {
    private IngestServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected IngestServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new IngestServiceBlockingStub(channel, callOptions);
    }

    /**
     * <pre>
     * 批量上报 Flow 事件
     * </pre>
     */
    public com.traffic.proto.traffic.v1.UploadFlowsResponse uploadFlows(com.traffic.proto.traffic.v1.UploadFlowsRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getUploadFlowsMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * 批量上报 Session 事件
     * </pre>
     */
    public com.traffic.proto.traffic.v1.UploadSessionsResponse uploadSessions(com.traffic.proto.traffic.v1.UploadSessionsRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getUploadSessionsMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * 上报 PCAP 索引元数据
     * </pre>
     */
    public com.traffic.proto.traffic.v1.UploadPcapIndexResponse uploadPcapIndex(com.traffic.proto.traffic.v1.UploadPcapIndexRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getUploadPcapIndexMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * 探针心跳与配置同步
     * </pre>
     */
    public com.traffic.proto.traffic.v1.HeartbeatResponse heartbeat(com.traffic.proto.traffic.v1.HeartbeatRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getHeartbeatMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * 探针注册
     * </pre>
     */
    public com.traffic.proto.traffic.v1.RegisterProbeResponse registerProbe(com.traffic.proto.traffic.v1.RegisterProbeRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getRegisterProbeMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service IngestService.
   */
  public static final class IngestServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<IngestServiceFutureStub> {
    private IngestServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected IngestServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new IngestServiceFutureStub(channel, callOptions);
    }

    /**
     * <pre>
     * 批量上报 Flow 事件
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.UploadFlowsResponse> uploadFlows(
        com.traffic.proto.traffic.v1.UploadFlowsRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getUploadFlowsMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * 批量上报 Session 事件
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.UploadSessionsResponse> uploadSessions(
        com.traffic.proto.traffic.v1.UploadSessionsRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getUploadSessionsMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * 上报 PCAP 索引元数据
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.UploadPcapIndexResponse> uploadPcapIndex(
        com.traffic.proto.traffic.v1.UploadPcapIndexRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getUploadPcapIndexMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * 探针心跳与配置同步
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.HeartbeatResponse> heartbeat(
        com.traffic.proto.traffic.v1.HeartbeatRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getHeartbeatMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * 探针注册
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.RegisterProbeResponse> registerProbe(
        com.traffic.proto.traffic.v1.RegisterProbeRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getRegisterProbeMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_UPLOAD_FLOWS = 0;
  private static final int METHODID_UPLOAD_SESSIONS = 1;
  private static final int METHODID_UPLOAD_PCAP_INDEX = 2;
  private static final int METHODID_HEARTBEAT = 3;
  private static final int METHODID_REGISTER_PROBE = 4;
  private static final int METHODID_STREAM_FLOWS = 5;

  private static final class MethodHandlers<Req, Resp> implements
      io.grpc.stub.ServerCalls.UnaryMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ServerStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ClientStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.BidiStreamingMethod<Req, Resp> {
    private final AsyncService serviceImpl;
    private final int methodId;

    MethodHandlers(AsyncService serviceImpl, int methodId) {
      this.serviceImpl = serviceImpl;
      this.methodId = methodId;
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public void invoke(Req request, io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        case METHODID_UPLOAD_FLOWS:
          serviceImpl.uploadFlows((com.traffic.proto.traffic.v1.UploadFlowsRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadFlowsResponse>) responseObserver);
          break;
        case METHODID_UPLOAD_SESSIONS:
          serviceImpl.uploadSessions((com.traffic.proto.traffic.v1.UploadSessionsRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadSessionsResponse>) responseObserver);
          break;
        case METHODID_UPLOAD_PCAP_INDEX:
          serviceImpl.uploadPcapIndex((com.traffic.proto.traffic.v1.UploadPcapIndexRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UploadPcapIndexResponse>) responseObserver);
          break;
        case METHODID_HEARTBEAT:
          serviceImpl.heartbeat((com.traffic.proto.traffic.v1.HeartbeatRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.HeartbeatResponse>) responseObserver);
          break;
        case METHODID_REGISTER_PROBE:
          serviceImpl.registerProbe((com.traffic.proto.traffic.v1.RegisterProbeRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.RegisterProbeResponse>) responseObserver);
          break;
        default:
          throw new AssertionError();
      }
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public io.grpc.stub.StreamObserver<Req> invoke(
        io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        case METHODID_STREAM_FLOWS:
          return (io.grpc.stub.StreamObserver<Req>) serviceImpl.streamFlows(
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.StreamFlowsResponse>) responseObserver);
        default:
          throw new AssertionError();
      }
    }
  }

  public static final io.grpc.ServerServiceDefinition bindService(AsyncService service) {
    return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
        .addMethod(
          getUploadFlowsMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.UploadFlowsRequest,
              com.traffic.proto.traffic.v1.UploadFlowsResponse>(
                service, METHODID_UPLOAD_FLOWS)))
        .addMethod(
          getStreamFlowsMethod(),
          io.grpc.stub.ServerCalls.asyncBidiStreamingCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.StreamFlowsRequest,
              com.traffic.proto.traffic.v1.StreamFlowsResponse>(
                service, METHODID_STREAM_FLOWS)))
        .addMethod(
          getUploadSessionsMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.UploadSessionsRequest,
              com.traffic.proto.traffic.v1.UploadSessionsResponse>(
                service, METHODID_UPLOAD_SESSIONS)))
        .addMethod(
          getUploadPcapIndexMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.UploadPcapIndexRequest,
              com.traffic.proto.traffic.v1.UploadPcapIndexResponse>(
                service, METHODID_UPLOAD_PCAP_INDEX)))
        .addMethod(
          getHeartbeatMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.HeartbeatRequest,
              com.traffic.proto.traffic.v1.HeartbeatResponse>(
                service, METHODID_HEARTBEAT)))
        .addMethod(
          getRegisterProbeMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.RegisterProbeRequest,
              com.traffic.proto.traffic.v1.RegisterProbeResponse>(
                service, METHODID_REGISTER_PROBE)))
        .build();
  }

  private static abstract class IngestServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    IngestServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return com.traffic.proto.traffic.v1.IngestProto.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("IngestService");
    }
  }

  private static final class IngestServiceFileDescriptorSupplier
      extends IngestServiceBaseDescriptorSupplier {
    IngestServiceFileDescriptorSupplier() {}
  }

  private static final class IngestServiceMethodDescriptorSupplier
      extends IngestServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    IngestServiceMethodDescriptorSupplier(java.lang.String methodName) {
      this.methodName = methodName;
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.MethodDescriptor getMethodDescriptor() {
      return getServiceDescriptor().findMethodByName(methodName);
    }
  }

  private static volatile io.grpc.ServiceDescriptor serviceDescriptor;

  public static io.grpc.ServiceDescriptor getServiceDescriptor() {
    io.grpc.ServiceDescriptor result = serviceDescriptor;
    if (result == null) {
      synchronized (IngestServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new IngestServiceFileDescriptorSupplier())
              .addMethod(getUploadFlowsMethod())
              .addMethod(getStreamFlowsMethod())
              .addMethod(getUploadSessionsMethod())
              .addMethod(getUploadPcapIndexMethod())
              .addMethod(getHeartbeatMethod())
              .addMethod(getRegisterProbeMethod())
              .build();
        }
      }
    }
    return result;
  }
}
