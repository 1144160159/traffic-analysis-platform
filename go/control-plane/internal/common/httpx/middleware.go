package httpx

import (
	"net/http"

	"go.uber.org/zap"
)

type Middleware func(http.Handler) http.Handler

type Chain struct {
	middlewares []Middleware
}

func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

func (c *Chain) Append(middlewares ...Middleware) *Chain {
	newMiddlewares := make([]Middleware, 0, len(c.middlewares)+len(middlewares))
	newMiddlewares = append(newMiddlewares, c.middlewares...)
	newMiddlewares = append(newMiddlewares, middlewares...)
	return &Chain{middlewares: newMiddlewares}
}

func (c *Chain) Prepend(middlewares ...Middleware) *Chain {
	newMiddlewares := make([]Middleware, 0, len(c.middlewares)+len(middlewares))
	newMiddlewares = append(newMiddlewares, middlewares...)
	newMiddlewares = append(newMiddlewares, c.middlewares...)
	return &Chain{middlewares: newMiddlewares}
}

func (c *Chain) Then(h http.Handler) http.Handler {
	if h == nil {
		h = http.DefaultServeMux
	}

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}

	return h
}

func (c *Chain) ThenFunc(fn http.HandlerFunc) http.Handler {
	if fn == nil {
		return c.Then(nil)
	}
	return c.Then(fn)
}

func (c *Chain) Handler(h http.Handler) http.Handler {
	return c.Then(h)
}

type Config struct {
	ServiceName    string
	Logger         *zap.Logger
	CORSConfig     *CORSConfig
	RequestTimeout int
}

func DefaultConfig() *Config {
	return &Config{
		ServiceName:    "unknown",
		RequestTimeout: 30,
		CORSConfig:     DefaultCORSConfig(),
	}
}

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

func AuthenticatedChain(cfg *Config, authMiddleware Middleware) *Chain {
	return DefaultChain(cfg).Append(authMiddleware)
}

func DefaultChainWithLogger(serviceName string, logger *zap.Logger) *Chain {
	cfg := &Config{
		ServiceName:    serviceName,
		Logger:         logger,
		RequestTimeout: 30,
		CORSConfig:     DefaultCORSConfig(),
	}
	return DefaultChain(cfg)
}
