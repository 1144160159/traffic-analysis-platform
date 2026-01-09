////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/httpx/middleware.go
// 修复版：使用正确的 logger 类型
////////////////////////////////////////////////////////////////////////////////

package httpx

import (
	"net/http"

	"go.uber.org/zap"
)

// Middleware HTTP中间件类型
type Middleware func(http.Handler) http.Handler

// Chain 中间件链
type Chain struct {
	middlewares []Middleware
}

// NewChain 创建中间件链
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// Append 追加中间件
func (c *Chain) Append(middlewares ...Middleware) *Chain {
	newMiddlewares := make([]Middleware, 0, len(c.middlewares)+len(middlewares))
	newMiddlewares = append(newMiddlewares, c.middlewares...)
	newMiddlewares = append(newMiddlewares, middlewares...)
	return &Chain{middlewares: newMiddlewares}
}

// Prepend 前置中间件
func (c *Chain) Prepend(middlewares ...Middleware) *Chain {
	newMiddlewares := make([]Middleware, 0, len(c.middlewares)+len(middlewares))
	newMiddlewares = append(newMiddlewares, middlewares...)
	newMiddlewares = append(newMiddlewares, c.middlewares...)
	return &Chain{middlewares: newMiddlewares}
}

// Then 应用中间件链到handler
func (c *Chain) Then(h http.Handler) http.Handler {
	if h == nil {
		h = http.DefaultServeMux
	}

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}

	return h
}

// ThenFunc 应用中间件链到handler函数
func (c *Chain) ThenFunc(fn http.HandlerFunc) http.Handler {
	if fn == nil {
		return c.Then(nil)
	}
	return c.Then(fn)
}

// Handler 返回应用中间件后的handler
func (c *Chain) Handler(h http.Handler) http.Handler {
	return c.Then(h)
}

// Config 中间件配置
type Config struct {
	ServiceName    string
	Logger         *zap.Logger
	CORSConfig     *CORSConfig
	RequestTimeout int // 秒
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ServiceName:    "unknown",
		RequestTimeout: 30,
		CORSConfig:     DefaultCORSConfig(),
	}
}

// DefaultChain 默认中间件链（推荐顺序）
func DefaultChain(cfg *Config) *Chain {
	return NewChain(
		Recovery(cfg.Logger),
		RequestID(),
		Logging(cfg.Logger),
		CORS(cfg.CORSConfig),
		Metrics(cfg.ServiceName),
		TimeoutWithConfig(cfg.RequestTimeout, nil),
	)
}

// AuthenticatedChain 带认证的中间件链
func AuthenticatedChain(cfg *Config, authMiddleware Middleware) *Chain {
	return DefaultChain(cfg).Append(authMiddleware)
}

// DefaultChainWithLogger 使用 logger 创建默认中间件链
func DefaultChainWithLogger(serviceName string, logger *zap.Logger) *Chain {
	cfg := &Config{
		ServiceName:    serviceName,
		Logger:         logger,
		RequestTimeout: 30,
		CORSConfig:     DefaultCORSConfig(),
	}
	return DefaultChain(cfg)
}
