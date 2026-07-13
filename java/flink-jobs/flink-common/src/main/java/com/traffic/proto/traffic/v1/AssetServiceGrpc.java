package com.traffic.proto.traffic.v1;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 */
@javax.annotation.Generated(
    value = "by gRPC proto compiler (version 1.61.0)",
    comments = "Source: traffic/v1/asset.proto")
@io.grpc.stub.annotations.GrpcGenerated
public final class AssetServiceGrpc {

  private AssetServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "traffic.v1.AssetService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UpsertAssetRequest,
      com.traffic.proto.traffic.v1.UpsertAssetResponse> getUpsertAssetMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "UpsertAsset",
      requestType = com.traffic.proto.traffic.v1.UpsertAssetRequest.class,
      responseType = com.traffic.proto.traffic.v1.UpsertAssetResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UpsertAssetRequest,
      com.traffic.proto.traffic.v1.UpsertAssetResponse> getUpsertAssetMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.UpsertAssetRequest, com.traffic.proto.traffic.v1.UpsertAssetResponse> getUpsertAssetMethod;
    if ((getUpsertAssetMethod = AssetServiceGrpc.getUpsertAssetMethod) == null) {
      synchronized (AssetServiceGrpc.class) {
        if ((getUpsertAssetMethod = AssetServiceGrpc.getUpsertAssetMethod) == null) {
          AssetServiceGrpc.getUpsertAssetMethod = getUpsertAssetMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.UpsertAssetRequest, com.traffic.proto.traffic.v1.UpsertAssetResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "UpsertAsset"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UpsertAssetRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.UpsertAssetResponse.getDefaultInstance()))
              .setSchemaDescriptor(new AssetServiceMethodDescriptorSupplier("UpsertAsset"))
              .build();
        }
      }
    }
    return getUpsertAssetMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.GetAssetRequest,
      com.traffic.proto.traffic.v1.GetAssetResponse> getGetAssetMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "GetAsset",
      requestType = com.traffic.proto.traffic.v1.GetAssetRequest.class,
      responseType = com.traffic.proto.traffic.v1.GetAssetResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.GetAssetRequest,
      com.traffic.proto.traffic.v1.GetAssetResponse> getGetAssetMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.GetAssetRequest, com.traffic.proto.traffic.v1.GetAssetResponse> getGetAssetMethod;
    if ((getGetAssetMethod = AssetServiceGrpc.getGetAssetMethod) == null) {
      synchronized (AssetServiceGrpc.class) {
        if ((getGetAssetMethod = AssetServiceGrpc.getGetAssetMethod) == null) {
          AssetServiceGrpc.getGetAssetMethod = getGetAssetMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.GetAssetRequest, com.traffic.proto.traffic.v1.GetAssetResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "GetAsset"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.GetAssetRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.GetAssetResponse.getDefaultInstance()))
              .setSchemaDescriptor(new AssetServiceMethodDescriptorSupplier("GetAsset"))
              .build();
        }
      }
    }
    return getGetAssetMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.ListAssetsRequest,
      com.traffic.proto.traffic.v1.ListAssetsResponse> getListAssetsMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ListAssets",
      requestType = com.traffic.proto.traffic.v1.ListAssetsRequest.class,
      responseType = com.traffic.proto.traffic.v1.ListAssetsResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.ListAssetsRequest,
      com.traffic.proto.traffic.v1.ListAssetsResponse> getListAssetsMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.ListAssetsRequest, com.traffic.proto.traffic.v1.ListAssetsResponse> getListAssetsMethod;
    if ((getListAssetsMethod = AssetServiceGrpc.getListAssetsMethod) == null) {
      synchronized (AssetServiceGrpc.class) {
        if ((getListAssetsMethod = AssetServiceGrpc.getListAssetsMethod) == null) {
          AssetServiceGrpc.getListAssetsMethod = getListAssetsMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.ListAssetsRequest, com.traffic.proto.traffic.v1.ListAssetsResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ListAssets"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.ListAssetsRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.ListAssetsResponse.getDefaultInstance()))
              .setSchemaDescriptor(new AssetServiceMethodDescriptorSupplier("ListAssets"))
              .build();
        }
      }
    }
    return getListAssetsMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.RecordMacIpBindingRequest,
      com.traffic.proto.traffic.v1.RecordMacIpBindingResponse> getRecordMacIpBindingMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "RecordMacIpBinding",
      requestType = com.traffic.proto.traffic.v1.RecordMacIpBindingRequest.class,
      responseType = com.traffic.proto.traffic.v1.RecordMacIpBindingResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.RecordMacIpBindingRequest,
      com.traffic.proto.traffic.v1.RecordMacIpBindingResponse> getRecordMacIpBindingMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.RecordMacIpBindingRequest, com.traffic.proto.traffic.v1.RecordMacIpBindingResponse> getRecordMacIpBindingMethod;
    if ((getRecordMacIpBindingMethod = AssetServiceGrpc.getRecordMacIpBindingMethod) == null) {
      synchronized (AssetServiceGrpc.class) {
        if ((getRecordMacIpBindingMethod = AssetServiceGrpc.getRecordMacIpBindingMethod) == null) {
          AssetServiceGrpc.getRecordMacIpBindingMethod = getRecordMacIpBindingMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.RecordMacIpBindingRequest, com.traffic.proto.traffic.v1.RecordMacIpBindingResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "RecordMacIpBinding"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.RecordMacIpBindingRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.RecordMacIpBindingResponse.getDefaultInstance()))
              .setSchemaDescriptor(new AssetServiceMethodDescriptorSupplier("RecordMacIpBinding"))
              .build();
        }
      }
    }
    return getRecordMacIpBindingMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.GetAssetHistoryRequest,
      com.traffic.proto.traffic.v1.GetAssetHistoryResponse> getGetAssetHistoryMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "GetAssetHistory",
      requestType = com.traffic.proto.traffic.v1.GetAssetHistoryRequest.class,
      responseType = com.traffic.proto.traffic.v1.GetAssetHistoryResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.GetAssetHistoryRequest,
      com.traffic.proto.traffic.v1.GetAssetHistoryResponse> getGetAssetHistoryMethod() {
    io.grpc.MethodDescriptor<com.traffic.proto.traffic.v1.GetAssetHistoryRequest, com.traffic.proto.traffic.v1.GetAssetHistoryResponse> getGetAssetHistoryMethod;
    if ((getGetAssetHistoryMethod = AssetServiceGrpc.getGetAssetHistoryMethod) == null) {
      synchronized (AssetServiceGrpc.class) {
        if ((getGetAssetHistoryMethod = AssetServiceGrpc.getGetAssetHistoryMethod) == null) {
          AssetServiceGrpc.getGetAssetHistoryMethod = getGetAssetHistoryMethod =
              io.grpc.MethodDescriptor.<com.traffic.proto.traffic.v1.GetAssetHistoryRequest, com.traffic.proto.traffic.v1.GetAssetHistoryResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "GetAssetHistory"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.GetAssetHistoryRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.traffic.proto.traffic.v1.GetAssetHistoryResponse.getDefaultInstance()))
              .setSchemaDescriptor(new AssetServiceMethodDescriptorSupplier("GetAssetHistory"))
              .build();
        }
      }
    }
    return getGetAssetHistoryMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static AssetServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<AssetServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<AssetServiceStub>() {
        @java.lang.Override
        public AssetServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new AssetServiceStub(channel, callOptions);
        }
      };
    return AssetServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static AssetServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<AssetServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<AssetServiceBlockingStub>() {
        @java.lang.Override
        public AssetServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new AssetServiceBlockingStub(channel, callOptions);
        }
      };
    return AssetServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static AssetServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<AssetServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<AssetServiceFutureStub>() {
        @java.lang.Override
        public AssetServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new AssetServiceFutureStub(channel, callOptions);
        }
      };
    return AssetServiceFutureStub.newStub(factory, channel);
  }

  /**
   */
  public interface AsyncService {

    /**
     * <pre>
     * RegisterOrUpdate creates or updates an asset.
     * </pre>
     */
    default void upsertAsset(com.traffic.proto.traffic.v1.UpsertAssetRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UpsertAssetResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getUpsertAssetMethod(), responseObserver);
    }

    /**
     * <pre>
     * GetAsset retrieves an asset by ID or MAC.
     * </pre>
     */
    default void getAsset(com.traffic.proto.traffic.v1.GetAssetRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.GetAssetResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getGetAssetMethod(), responseObserver);
    }

    /**
     * <pre>
     * ListAssets returns assets for a tenant, with optional filters.
     * </pre>
     */
    default void listAssets(com.traffic.proto.traffic.v1.ListAssetsRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.ListAssetsResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getListAssetsMethod(), responseObserver);
    }

    /**
     * <pre>
     * RecordMacIpBinding records a MAC→IP binding observed from traffic.
     * </pre>
     */
    default void recordMacIpBinding(com.traffic.proto.traffic.v1.RecordMacIpBindingRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.RecordMacIpBindingResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getRecordMacIpBindingMethod(), responseObserver);
    }

    /**
     * <pre>
     * GetAssetHistory returns the change event history for an asset.
     * </pre>
     */
    default void getAssetHistory(com.traffic.proto.traffic.v1.GetAssetHistoryRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.GetAssetHistoryResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getGetAssetHistoryMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service AssetService.
   */
  public static abstract class AssetServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return AssetServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service AssetService.
   */
  public static final class AssetServiceStub
      extends io.grpc.stub.AbstractAsyncStub<AssetServiceStub> {
    private AssetServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected AssetServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new AssetServiceStub(channel, callOptions);
    }

    /**
     * <pre>
     * RegisterOrUpdate creates or updates an asset.
     * </pre>
     */
    public void upsertAsset(com.traffic.proto.traffic.v1.UpsertAssetRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UpsertAssetResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getUpsertAssetMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * GetAsset retrieves an asset by ID or MAC.
     * </pre>
     */
    public void getAsset(com.traffic.proto.traffic.v1.GetAssetRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.GetAssetResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getGetAssetMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * ListAssets returns assets for a tenant, with optional filters.
     * </pre>
     */
    public void listAssets(com.traffic.proto.traffic.v1.ListAssetsRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.ListAssetsResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getListAssetsMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * RecordMacIpBinding records a MAC→IP binding observed from traffic.
     * </pre>
     */
    public void recordMacIpBinding(com.traffic.proto.traffic.v1.RecordMacIpBindingRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.RecordMacIpBindingResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getRecordMacIpBindingMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     * <pre>
     * GetAssetHistory returns the change event history for an asset.
     * </pre>
     */
    public void getAssetHistory(com.traffic.proto.traffic.v1.GetAssetHistoryRequest request,
        io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.GetAssetHistoryResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getGetAssetHistoryMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service AssetService.
   */
  public static final class AssetServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<AssetServiceBlockingStub> {
    private AssetServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected AssetServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new AssetServiceBlockingStub(channel, callOptions);
    }

    /**
     * <pre>
     * RegisterOrUpdate creates or updates an asset.
     * </pre>
     */
    public com.traffic.proto.traffic.v1.UpsertAssetResponse upsertAsset(com.traffic.proto.traffic.v1.UpsertAssetRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getUpsertAssetMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * GetAsset retrieves an asset by ID or MAC.
     * </pre>
     */
    public com.traffic.proto.traffic.v1.GetAssetResponse getAsset(com.traffic.proto.traffic.v1.GetAssetRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getGetAssetMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * ListAssets returns assets for a tenant, with optional filters.
     * </pre>
     */
    public com.traffic.proto.traffic.v1.ListAssetsResponse listAssets(com.traffic.proto.traffic.v1.ListAssetsRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getListAssetsMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * RecordMacIpBinding records a MAC→IP binding observed from traffic.
     * </pre>
     */
    public com.traffic.proto.traffic.v1.RecordMacIpBindingResponse recordMacIpBinding(com.traffic.proto.traffic.v1.RecordMacIpBindingRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getRecordMacIpBindingMethod(), getCallOptions(), request);
    }

    /**
     * <pre>
     * GetAssetHistory returns the change event history for an asset.
     * </pre>
     */
    public com.traffic.proto.traffic.v1.GetAssetHistoryResponse getAssetHistory(com.traffic.proto.traffic.v1.GetAssetHistoryRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getGetAssetHistoryMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service AssetService.
   */
  public static final class AssetServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<AssetServiceFutureStub> {
    private AssetServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected AssetServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new AssetServiceFutureStub(channel, callOptions);
    }

    /**
     * <pre>
     * RegisterOrUpdate creates or updates an asset.
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.UpsertAssetResponse> upsertAsset(
        com.traffic.proto.traffic.v1.UpsertAssetRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getUpsertAssetMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * GetAsset retrieves an asset by ID or MAC.
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.GetAssetResponse> getAsset(
        com.traffic.proto.traffic.v1.GetAssetRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getGetAssetMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * ListAssets returns assets for a tenant, with optional filters.
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.ListAssetsResponse> listAssets(
        com.traffic.proto.traffic.v1.ListAssetsRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getListAssetsMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * RecordMacIpBinding records a MAC→IP binding observed from traffic.
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.RecordMacIpBindingResponse> recordMacIpBinding(
        com.traffic.proto.traffic.v1.RecordMacIpBindingRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getRecordMacIpBindingMethod(), getCallOptions()), request);
    }

    /**
     * <pre>
     * GetAssetHistory returns the change event history for an asset.
     * </pre>
     */
    public com.google.common.util.concurrent.ListenableFuture<com.traffic.proto.traffic.v1.GetAssetHistoryResponse> getAssetHistory(
        com.traffic.proto.traffic.v1.GetAssetHistoryRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getGetAssetHistoryMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_UPSERT_ASSET = 0;
  private static final int METHODID_GET_ASSET = 1;
  private static final int METHODID_LIST_ASSETS = 2;
  private static final int METHODID_RECORD_MAC_IP_BINDING = 3;
  private static final int METHODID_GET_ASSET_HISTORY = 4;

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
        case METHODID_UPSERT_ASSET:
          serviceImpl.upsertAsset((com.traffic.proto.traffic.v1.UpsertAssetRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.UpsertAssetResponse>) responseObserver);
          break;
        case METHODID_GET_ASSET:
          serviceImpl.getAsset((com.traffic.proto.traffic.v1.GetAssetRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.GetAssetResponse>) responseObserver);
          break;
        case METHODID_LIST_ASSETS:
          serviceImpl.listAssets((com.traffic.proto.traffic.v1.ListAssetsRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.ListAssetsResponse>) responseObserver);
          break;
        case METHODID_RECORD_MAC_IP_BINDING:
          serviceImpl.recordMacIpBinding((com.traffic.proto.traffic.v1.RecordMacIpBindingRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.RecordMacIpBindingResponse>) responseObserver);
          break;
        case METHODID_GET_ASSET_HISTORY:
          serviceImpl.getAssetHistory((com.traffic.proto.traffic.v1.GetAssetHistoryRequest) request,
              (io.grpc.stub.StreamObserver<com.traffic.proto.traffic.v1.GetAssetHistoryResponse>) responseObserver);
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
        default:
          throw new AssertionError();
      }
    }
  }

  public static final io.grpc.ServerServiceDefinition bindService(AsyncService service) {
    return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
        .addMethod(
          getUpsertAssetMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.UpsertAssetRequest,
              com.traffic.proto.traffic.v1.UpsertAssetResponse>(
                service, METHODID_UPSERT_ASSET)))
        .addMethod(
          getGetAssetMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.GetAssetRequest,
              com.traffic.proto.traffic.v1.GetAssetResponse>(
                service, METHODID_GET_ASSET)))
        .addMethod(
          getListAssetsMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.ListAssetsRequest,
              com.traffic.proto.traffic.v1.ListAssetsResponse>(
                service, METHODID_LIST_ASSETS)))
        .addMethod(
          getRecordMacIpBindingMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.RecordMacIpBindingRequest,
              com.traffic.proto.traffic.v1.RecordMacIpBindingResponse>(
                service, METHODID_RECORD_MAC_IP_BINDING)))
        .addMethod(
          getGetAssetHistoryMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.traffic.proto.traffic.v1.GetAssetHistoryRequest,
              com.traffic.proto.traffic.v1.GetAssetHistoryResponse>(
                service, METHODID_GET_ASSET_HISTORY)))
        .build();
  }

  private static abstract class AssetServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    AssetServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return com.traffic.proto.traffic.v1.AssetProto.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("AssetService");
    }
  }

  private static final class AssetServiceFileDescriptorSupplier
      extends AssetServiceBaseDescriptorSupplier {
    AssetServiceFileDescriptorSupplier() {}
  }

  private static final class AssetServiceMethodDescriptorSupplier
      extends AssetServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    AssetServiceMethodDescriptorSupplier(java.lang.String methodName) {
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
      synchronized (AssetServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new AssetServiceFileDescriptorSupplier())
              .addMethod(getUpsertAssetMethod())
              .addMethod(getGetAssetMethod())
              .addMethod(getListAssetsMethod())
              .addMethod(getRecordMacIpBindingMethod())
              .addMethod(getGetAssetHistoryMethod())
              .build();
        }
      }
    }
    return result;
  }
}
