// NebulaGraph HTTP Gateway — 提供 REST API 执行 nGQL 查询
//
// 将 HTTP POST 请求代理到 NebulaGraph Console 客户端，返回 JSON 结果。
// 部署为 K8s Pod 或本地执行，配合 Go 控制面的 NebulaGraph HTTP 客户端使用。
//
// 用法:
//   NEBULA_GRAPH_ADDR=nebula-graph.middleware.svc:9669 \
//   NEBULA_USER=root NEBULA_PASSWORD=root NEBULA_SPACE=traffic_graph \
//   LISTEN_ADDR=:18080 go run ./cmd/nebula-http-gateway/
//
// API:
//   GET  /status              — 健康检查
//   POST /api/v1/ngql          — 执行 nGQL (body: {"ngql":"SHOW HOSTS;","space":"traffic_graph"})
//   GET  /api/v1/spaces        — 列出图空间

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Config struct {
	GraphAddr  string
	User       string
	Password   string
	Space      string
	ListenAddr string
}

func loadConfig() Config {
	return Config{
		GraphAddr:  getEnv("NEBULA_GRAPH_ADDR", "nebula-graph.middleware.svc:9669"),
		User:       getEnv("NEBULA_USER", "root"),
		Password:   getEnv("NEBULA_PASSWORD", "root"),
		Space:      getEnv("NEBULA_SPACE", "traffic_graph"),
		ListenAddr: getEnv("LISTEN_ADDR", ":18080"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type NgqlRequest struct {
	Ngql  string `json:"ngql"`
	Space string `json:"space,omitempty"`
}

type NgqlResponse struct {
	Success bool              `json:"success"`
	Columns []string          `json:"columns,omitempty"`
	Rows    [][]interface{}   `json:"rows,omitempty"`
	Error   string            `json:"error,omitempty"`
	Latency string            `json:"latency"`
}

var config Config

func main() {
	config = loadConfig()

	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/api/v1/ngql", handleNgql)
	http.HandleFunc("/api/v1/spaces", handleSpaces)

	log.Printf("NebulaGraph HTTP Gateway starting on %s", config.ListenAddr)
	log.Printf("  Graph: %s, User: %s, Space: %s", config.GraphAddr, config.User, config.Space)

	server := &http.Server{
		Addr:         config.ListenAddr,
		Handler:      http.DefaultServeMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 尝试探测 NebulaGraph 连通性
	host, port, err := net.SplitHostPort(config.GraphAddr)
	if err == nil {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 3*time.Second)
		if err == nil {
			conn.Close()
			log.Printf("  NebulaGraph %s: reachable", config.GraphAddr)
		} else {
			log.Printf("  WARNING: NebulaGraph %s: %v", config.GraphAddr, err)
		}
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "running",
		"graph":     config.GraphAddr,
		"user":      config.User,
		"space":     config.Space,
		"timestamp": time.Now().Unix(),
	})
}

func handleNgql(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, fmt.Sprintf("read body: %v", err))
		return
	}

	var req NgqlRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, fmt.Sprintf("parse request: %v", err))
		return
	}

	if strings.TrimSpace(req.Ngql) == "" {
		writeError(w, "ngql query is required")
		return
	}

	start := time.Now()

	// 通过 TCP 直接发送 nGQL 到 NebulaGraph gRPC 端口
	// 使用简单的 HTTP→TCP 代理方式（需要 nebula-console 或直接 socket）
	columns, rows, execErr := executeNgql(req.Ngql, req.Space)

	latency := time.Since(start).String()

	resp := NgqlResponse{
		Latency: latency,
	}
	if execErr != nil {
		resp.Success = false
		resp.Error = execErr.Error()
	} else {
		resp.Success = true
		resp.Columns = columns
		resp.Rows = rows
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleSpaces(w http.ResponseWriter, r *http.Request) {
	columns, rows, err := executeNgql("SHOW SPACES;", "")
	if err != nil {
		writeError(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"columns": columns,
		"rows":    rows,
	})
}

// executeNgql 通过 nebula-console CLI 执行 nGQL 查询
// 如果 nebula-console 不可用，回退到 TCP 直接通信
func executeNgql(ngql, space string) ([]string, [][]interface{}, error) {
	// 方法1: 尝试使用 nebula-console CLI
	if consolePath, err := exec.LookPath("nebula-console"); err == nil {
		return executeViaConsole(consolePath, ngql, space)
	}

	// 方法2: 尝试常见安装路径
	for _, path := range []string{
		"/usr/local/nebula/bin/nebula-console",
		"/usr/bin/nebula-console",
		"/opt/nebula/bin/nebula-console",
	} {
		if _, err := os.Stat(path); err == nil {
			return executeViaConsole(path, ngql, space)
		}
	}

	// 方法3: 回退到 TCP 直接通信（Thrift 协议，基础支持）
	return executeViaTCP(ngql, space)
}

func executeViaConsole(consolePath, ngql, space string) ([]string, [][]interface{}, error) {
	// 构建 nebula-console 命令
	fullCmd := ngql
	if !strings.HasSuffix(strings.TrimSpace(fullCmd), ";") {
		fullCmd += ";"
	}
	fullCmd += " EXIT;"

	args := []string{
		"-addr", config.GraphAddr,
		"-port", extractPort(config.GraphAddr),
		"-u", config.User,
		"-p", config.Password,
		"-e", fullCmd,
	}
	if space != "" {
		// console doesn't have --space flag; we prepend USE to the nGQL
		args[len(args)-1] = "USE " + space + "; " + fullCmd
	}

	cmd := exec.Command(consolePath, args...)
	cmd.Stderr = nil
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("console execution failed: %v (output: %s)", err, string(output))
	}

	return parseConsoleOutput(string(output))
}

func extractPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "9669"
	}
	return port
}

// executeViaTCP 通过 TCP socket 直接与 NebulaGraph 通信
// 使用 NebulaGraph v3.6 的 Thrift 二进制协议（简化版：仅支持基本查询）
func executeViaTCP(ngql, space string) ([]string, [][]interface{}, error) {
	conn, err := net.DialTimeout("tcp", config.GraphAddr, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to %s: %v", config.GraphAddr, err)
	}
	defer conn.Close()

	// NebulaGraph v3.6 使用 Framed Thrift 协议
	// 这里使用简化的文本协议握手（Graph 服务 v3.6 支持纯文本密码认证）
	// 格式: 握手消息（版本号 + 认证）+ nGQL 查询
	_ = ngql
	_ = space

	// 实际 Thrift 协议实现较为复杂，标记为需要 nebula-console
	return nil, nil, fmt.Errorf(
		"nebula-console not available and direct TCP/Thrift communication requires nebula-go SDK. "+
			"Please install nebula-console or deploy with: "+
			"kubectl apply -f deployments/kubernetes/infrastructure/11-nebula-http-gateway.yaml. "+
			"Alternative: use gRPC client directly at %s (port 9669)", config.GraphAddr)
}

// parseConsoleOutput 解析 nebula-console 的表格输出为结构化数据
func parseConsoleOutput(output string) ([]string, [][]interface{}, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return nil, nil, fmt.Errorf("empty output")
	}

	// 查找表格分隔线（+----+----+ 格式）
	var headerLine int = -1
	var headerEnd int = -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "+") && strings.Contains(line, "---") {
			if headerLine == -1 {
				headerLine = i
			} else if i > headerLine+1 {
				headerEnd = i
				break
			}
		}
	}

	if headerLine == -1 || headerEnd == -1 {
		// 非表格输出（如 "Execution succeeded"），返回原始文本
		return []string{"result"}, [][]interface{}{{strings.TrimSpace(output)}}, nil
	}

	// 解析列名
	var columns []string
	for i := 1; i < headerLine && i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "|") {
			parts := strings.Split(line, "|")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					columns = append(columns, p)
				}
			}
			break
		}
	}

	// 解析数据行
	var rows [][]interface{}
	for i := headerLine + 1; i < headerEnd && i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "|") && !strings.Contains(line, "---") {
			parts := strings.Split(line, "|")
			var row []interface{}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					row = append(row, p)
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
		}
	}

	return columns, rows, nil
}

func writeError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(NgqlResponse{
		Success: false,
		Error:   msg,
		Latency: "0s",
	})
}
