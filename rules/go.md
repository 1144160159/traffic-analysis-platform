# Go 开发规范

基于 [Effective Go](https://go.dev/doc/effective_go)、[Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)、[Uber Go Style Guide](https://github.com/uber-go/guide)。

## 1. 命名

```go
// 包名：小写、单数、简短、无下划线
package ingest      // ✓
package ingest_gateway  // ✗

// 导出标识符：PascalCase
type FlowEvent struct { ... }
func NewIngestGateway() *IngestGateway { ... }

// 非导出：camelCase
var defaultTimeout = 30 * time.Second
func parseFlow(data []byte) (*Flow, error) { ... }

// 接口：单方法接口以 -er 结尾
type Reader interface { Read(p []byte) (n int, err error) }
type Validator interface { Validate() error }

// 缩写：全大写或全小写
type HTTPClient struct { ... }   // ✓
type HttpClient struct { ... }   // ✗
var userID string                // ✓
var userId string                // ✗

// getter：不加 Get 前缀
func (s *Server) Addr() string { return s.addr }  // ✓
func (s *Server) GetAddr() string { ... }          // ✗
```

## 2. 错误处理

```go
// 必须处理 error
data, err := os.ReadFile(path)
if err != nil {
    return fmt.Errorf("read config %s: %w", path, err)
}

// 禁止忽略 error
_ = os.Remove(tmpFile)  // ✗ 必须注释说明为什么忽略

// 错误包装：使用 %w 保留错误链
if err := validate(req); err != nil {
    return errors.NewInvalidArgument("validate failed: %w", err)
}

// 错误类型：定义 sentinel errors + custom types
var ErrTenantNotFound = errors.New("tenant not found")

type ValidationError struct {
    Field   string
    Message string
}
func (e *ValidationError) Error() string {
    return fmt.Sprintf("field %s: %s", e.Field, e.Message)
}

// panic 仅仅用于不可恢复的初始化错误
config := mustLoadConfig()  // os.Exit(1) if failed
```

## 3. 并发

```go
// goroutine 生命周期必须明确
ctx, cancel := context.WithTimeout(parent, 30*time.Second)
defer cancel()

go func() {
    <-ctx.Done()
    // cleanup
}()

// 使用 errgroup 管理并发
g, ctx := errgroup.WithContext(ctx)
g.Go(func() error { return fetchA(ctx) })
g.Go(func() error { return fetchB(ctx) })
return g.Wait()

// channel：明确方向、容量
var events chan<- Event   // write-only
var quit <-chan struct{}  // read-only

// sync.Pool 用于频繁分配的对象
var bufPool = sync.Pool{New: func() any { return make([]byte, 4096) }}

// 禁止：
//   - goroutine 泄漏（没有退出机制）
//   - channel 未关闭导致死锁
//   - 对未初始化的 sync.Mutex 进行复制
```

## 4. 性能

```go
// 预分配 slice 容量
ids := make([]string, 0, estimatedSize)

// strings.Builder 代替 + 拼接
var b strings.Builder
b.WriteString("prefix_")
b.WriteString(id)

// 避免在 hot path 使用 defer
// 如必须，确认性能影响 < 1%

// 用 io.Reader/io.Writer 接口而非具体类型
func process(r io.Reader) error { ... }

// []byte 与 string 转换有内存拷贝
// 频繁转换时考虑 unsafe 或 bytes 包
```

## 5. 测试

```go
// 表格驱动测试
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        input   Request
        wantErr bool
    }{
        {"valid", validReq, false},
        {"missing_tenant", invalidReq, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := Validate(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

// 使用 testify 断言库
assert.Equal(t, expected, actual)
require.NoError(t, err)

// mock：接口 + 手写 mock 或用 mockgen
type Store interface {
    Get(ctx context.Context, key string) (string, error)
}
```

## 6. 项目结构

```
cmd/<service>/main.go          # 入口，只做初始化
internal/<domain>/
  api/handler.go               # HTTP/gRPC handler
  service/service.go           # 业务逻辑
  repository/postgres.go       # 数据访问
  config/config.go             # 配置
internal/common/
  errors/  httpx/  logging/    # 跨服务公共模块
  kafka/   otel/   storage/    # 基础设施封装
  utils/   validation/         # 工具函数
```
