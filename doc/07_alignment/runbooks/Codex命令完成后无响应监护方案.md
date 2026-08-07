# Codex 命令完成后无响应监护方案

## 目标

只处理一种情况：命令工具已经返回终态，但 Codex 没有解释结果或继续当前任务。

本方案不监控模型思考时长，不启动子代理，不重跑命令，也不启动第二个 Codex 进程。

## 机制

项目级 `.codex/hooks.json` 注册 `PostToolUse` hook，只匹配 Codex 官方定义的 canonical 工具名 `Bash`。`exec_command` 的长命令在中间轮询时不会触发完成 hook；后续 `write_stdin` 取得终态时，Codex 才为原命令触发一次 `PostToolUse(Bash)`。

因此 `PostToolUse(Bash)` 事件本身就是完成信号。`scripts/codex_hooks/command_completion_guard.py` 会尽力从响应中提取 `exit_code` 用于日志和提示，但不会因为某种工具响应格式没有数值退出码而漏掉连续响应提醒。

若上层工具返回 `session_id`、`cell_id` 或运行中状态，代理仍须轮询到终态。若终态只给出 `Script completed` 且没有可继续轮询的 ID，则记录“退出码缺失”并检查产物，不允许为补退出码重复执行原命令。

命令结束后 hook 执行两项动作：

1. 向下一次模型采样注入短上下文，要求立即解释退出结果、向用户报告并继续任务。
2. 在 `${CODEX_COMMAND_COMPLETION_GUARD_DIR}` 或 `/tmp/codex-command-completion-guard` 下追加 metadata-only JSONL 事件。

日志只记录 session、turn、tool use、工具名、时间和退出码，不记录命令、stdout 或 stderr。

## 启用

Codex 在新会话发现项目 hook 时会要求审核信任。确认 `.codex/hooks.json` 和脚本内容后选择信任；hook 在后续工具调用中生效。当前已经运行的会话不会热加载新 hook。

本隔离整改 worktree 中的实现已经可测试。要在目标项目会话中自动生效，应先评审并合入该 checkout，再新开或重启 Codex 会话，并在 `/hooks` 中完成项目 hook 的信任确认。不同 Codex 表面对 worktree 的加载细节可能不同，以新会话 `/hooks` 显示的实际来源为准。

不要使用 `--dangerously-bypass-hook-trust` 作为日常启动参数。

## 验证

```bash
python3 -m unittest discover -s tests/codex_hooks -p 'test_*.py' -v
```

手工模拟成功命令：

```bash
printf '%s' '{"session_id":"smoke","turn_id":"turn-1","tool_use_id":"tool-1","tool_name":"Bash","tool_response":{"exit_code":0}}' \
  | python3 scripts/codex_hooks/command_completion_guard.py post-tool
```

检查完成事件：

```bash
python3 scripts/codex_hooks/command_completion_guard.py status --session-id smoke
```

## 边界

这个 hook 能处理“工具结果已经交还给 Codex，但模型没有主动继续”的路径。若 Codex 后端、网络连接或 app-server 本身冻结，hook 无法从仓库进程中重启模型推理；那类故障必须由 Codex 产品侧线程心跳或人工重新发送消息恢复。它是连续响应护栏，不是进程级 watchdog。

为了避免重复副作用，本方案刻意不自动执行 `codex exec resume`、`turn/interrupt` 或 `turn/start`。
