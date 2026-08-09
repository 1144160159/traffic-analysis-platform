from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "scripts" / "codex_hooks" / "command_completion_guard.py"
SPEC = importlib.util.spec_from_file_location("command_completion_guard", SCRIPT)
assert SPEC and SPEC.loader
guard = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(guard)


class CommandCompletionGuardTest(unittest.TestCase):
    def test_explicit_exit_code_is_completed(self) -> None:
        outcome, exit_code = guard.classify_response(
            "Bash", {"output": "done", "exit_code": 0, "session_id": 42}
        )
        self.assertEqual((outcome, exit_code), ("completed", 0))

    def test_internal_transport_aliases_are_not_matched(self) -> None:
        for tool_name in ("exec", "wait", "write_stdin", "functions.exec"):
            with self.subTest(tool_name=tool_name):
                outcome, exit_code = guard.classify_response(tool_name, {"exit_code": 0})
                self.assertEqual((outcome, exit_code), ("ignored", None))

    def test_post_tool_use_without_exit_code_is_still_completed(self) -> None:
        outcome, exit_code = guard.classify_response(
            "Bash", {"output": "tool-specific response without numeric status"}
        )
        self.assertEqual((outcome, exit_code), ("completed", None))

    def test_nested_text_exit_code_is_completed(self) -> None:
        outcome, exit_code = guard.classify_response(
            "Bash",
            'Script completed\nOutput: {"exit_code":3,"output":"failed"}',
        )
        self.assertEqual((outcome, exit_code), ("completed", 3))

    def test_unrelated_tool_is_ignored(self) -> None:
        outcome, exit_code = guard.classify_response("apply_patch", {"exit_code": 0})
        self.assertEqual((outcome, exit_code), ("ignored", None))

    def test_hook_writes_metadata_only_and_injects_context(self) -> None:
        secret_command = "curl -H Authorization:secret-token https://example.invalid"
        payload = {
            "session_id": "session-1",
            "turn_id": "turn-1",
            "tool_use_id": "tool-1",
            "tool_name": "Bash",
            "tool_input": {"command": secret_command},
            "tool_response": {"exit_code": 0, "output": "private-output"},
        }
        with tempfile.TemporaryDirectory() as temp_dir:
            previous = os.environ.get("CODEX_COMMAND_COMPLETION_GUARD_DIR")
            os.environ["CODEX_COMMAND_COMPLETION_GUARD_DIR"] = temp_dir
            try:
                output = guard.post_tool(payload)
            finally:
                if previous is None:
                    os.environ.pop("CODEX_COMMAND_COMPLETION_GUARD_DIR", None)
                else:
                    os.environ["CODEX_COMMAND_COMPLETION_GUARD_DIR"] = previous

            context = output["hookSpecificOutput"]["additionalContext"]
            self.assertIn("exit_code=0", context)
            journal = (Path(temp_dir) / "session-1.jsonl").read_text(encoding="utf-8")
            event = json.loads(journal)
            self.assertEqual(event["exit_code"], 0)
            self.assertNotIn(secret_command, journal)
            self.assertNotIn("private-output", journal)

    def test_cli_fails_open_on_malformed_input(self) -> None:
        completed = subprocess.run(
            [sys.executable, str(SCRIPT), "post-tool"],
            input="not-json",
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        self.assertEqual(completed.returncode, 0)
        output = json.loads(completed.stdout)
        self.assertTrue(output["continue"])
        self.assertIn("failed open", output["systemMessage"])

    def test_project_hook_points_to_guard(self) -> None:
        hooks = json.loads((REPO_ROOT / ".codex" / "hooks.json").read_text(encoding="utf-8"))
        post_tool_use = hooks["hooks"]["PostToolUse"]
        self.assertEqual(len(post_tool_use), 1)
        handler = post_tool_use[0]["hooks"][0]
        self.assertEqual(post_tool_use[0]["matcher"], "^Bash$")
        self.assertEqual(handler["type"], "command")
        self.assertIn("command_completion_guard.py", handler["command"])
        self.assertEqual(handler["timeout"], 5)
        self.assertEqual(handler["additionalContextLimit"], 500)


if __name__ == "__main__":
    unittest.main()
