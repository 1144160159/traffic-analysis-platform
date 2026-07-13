#!/usr/bin/env python3
"""Self-test the Desktop Chrome capture receiver without creating acceptance evidence.

The test starts the receiver on localhost with a temporary evidence directory,
exercises the viewport probe/report endpoints plus upload/interaction guards,
and writes a small stable summary. It proves the receiver is ready to accept
real Desktop Chrome evidence; it does not create or substitute any visual diff
or route interaction acceptance files.
"""

from __future__ import annotations

import argparse
import io
import json
import secrets
import tempfile
import threading
from datetime import datetime, timezone
from http.server import ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.error import HTTPError
from urllib.request import ProxyHandler, Request, build_opener

from PIL import Image

from ui_desktop_capture_receiver import ReceiverState, make_handler


DEFAULT_OUTPUT_JSON = Path("doc/02_acceptance/02-regression/ui-visual-interaction/receiver-selftest-latest.json")
DEFAULT_OUTPUT_MD = Path("doc/02_acceptance/02-regression/ui-visual-interaction/receiver-selftest-latest.md")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Self-test UI Desktop capture receiver endpoints.")
    parser.add_argument("--output-json", type=Path, default=DEFAULT_OUTPUT_JSON)
    parser.add_argument("--output-md", type=Path, default=DEFAULT_OUTPUT_MD)
    parser.add_argument("--expected-width", type=int, default=1920)
    parser.add_argument("--expected-height", type=int, default=1080)
    return parser.parse_args()


def now_iso() -> str:
    return datetime.now(timezone.utc).astimezone().isoformat()


def repo_rel(path: Path) -> str:
    try:
        return path.resolve().relative_to(Path.cwd().resolve()).as_posix()
    except ValueError:
        return path.as_posix()


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_md(path: Path, summary: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "# UI Desktop Capture Receiver Self-test",
        "",
        f"- Result: `{summary['result']}`",
        f"- Generated: `{summary['generated_at']}`",
        f"- Expected viewport: `{summary['expected_size']['width']}x{summary['expected_size']['height']}`",
        f"- Checks passed: `{summary['passed']}/{summary['total']}`",
        "",
        "This self-test uses a temporary evidence directory. It does not replace Desktop Chrome screenshots, `capture-meta.json`, visual diff metrics, or route `interaction.json` evidence.",
        "",
        "## Checks",
        "",
    ]
    for check in summary["checks"]:
        lines.append(f"- `{check['status']}` {check['name']}: {check['detail']}")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def request(base_url: str, method: str, path: str, body: bytes | None = None, headers: dict[str, str] | None = None) -> tuple[int, bytes]:
    req = Request(f"{base_url}{path}", data=body, method=method, headers=headers or {})
    opener = build_opener(ProxyHandler({}))
    try:
        with opener.open(req, timeout=5) as response:  # noqa: S310 - local self-test server only.
            return response.status, response.read()
    except HTTPError as error:
        return error.code, error.read()


def make_png(width: int, height: int) -> bytes:
    buf = io.BytesIO()
    Image.new("RGB", (width, height), color=(7, 16, 21)).save(buf, format="PNG")
    return buf.getvalue()


def check(name: str, passed: bool, detail: str, artifact: str = "") -> dict[str, Any]:
    return {
        "name": name,
        "passed": passed,
        "status": "pass" if passed else "fail",
        "detail": detail,
        "artifact": artifact,
    }


def read_json_if_exists(path: Path) -> dict[str, Any] | None:
    if not path.is_file():
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def run_selftest(expected_width: int, expected_height: int) -> dict[str, Any]:
    checks: list[dict[str, Any]] = []
    token = f"selftest-token-{secrets.token_hex(8)}"
    capture_key = f"selftest-key-{secrets.token_hex(8)}"

    with tempfile.TemporaryDirectory(prefix="taf-capture-receiver-selftest-") as tmp:
        temp_root = Path(tmp)
        evidence_dir = temp_root / "latest"
        state = ReceiverState(
            evidence_dir=evidence_dir,
            token=token,
            capture_key=capture_key,
            max_uploads=20,
            max_bytes=12 * 1024 * 1024,
            max_json_bytes=1024 * 1024,
            expected_width=expected_width,
            expected_height=expected_height,
        )
        server = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(state))
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        base_url = f"http://127.0.0.1:{server.server_address[1]}"
        try:
            status, body = request(base_url, "GET", "/health")
            checks.append(check("health endpoint responds", status == 200 and body == b"ok\n", f"status={status}"))

            status, body = request(base_url, "GET", "/viewport-probe")
            body_text = body.decode("utf-8", errors="replace")
            checks.append(
                check(
                    "viewport probe page is served",
                    status == 200 and "/viewport-report" in body_text and str(expected_width) in body_text,
                    f"status={status} contains_report_endpoint={'/viewport-report' in body_text}",
                )
            )

            status, _ = request(base_url, "GET", "/token")
            checks.append(check("token endpoint rejects unauthenticated reads", status == 403, f"status={status}"))

            status, body = request(base_url, "GET", "/token", headers={"X-Codex-Capture-Key": capture_key})
            checks.append(check("token endpoint accepts capture key", status == 200 and body.decode() == token, f"status={status}"))

            pass_payload = {
                "status": "pass",
                "viewport": {"width": expected_width, "height": expected_height},
                "expected_size": {"width": expected_width, "height": expected_height},
                "device_pixel_ratio": 1,
                "user_agent": "receiver-selftest",
            }
            status, _ = request(
                base_url,
                "POST",
                "/viewport-report",
                json.dumps(pass_payload).encode("utf-8"),
                {"Content-Type": "application/json"},
            )
            viewport_report_path = temp_root / "desktop-chrome-viewport-probe-latest.json"
            viewport_report = read_json_if_exists(viewport_report_path)
            checks.append(
                check(
                    "viewport report pass is stored",
                    status == 201 and viewport_report is not None and viewport_report.get("result") == "pass",
                    f"status={status} result={viewport_report.get('result') if viewport_report else 'missing-file'}",
                    str(viewport_report_path),
                )
            )

            blocked_payload = {
                "status": "blocked",
                "viewport": {"width": expected_width + 640, "height": expected_height + 191},
                "expected_size": {"width": expected_width, "height": expected_height},
                "device_pixel_ratio": 1.5,
                "user_agent": "receiver-selftest",
            }
            status, _ = request(
                base_url,
                "POST",
                "/viewport-report",
                json.dumps(blocked_payload).encode("utf-8"),
                {"Content-Type": "application/json"},
            )
            viewport_report = read_json_if_exists(viewport_report_path)
            checks.append(
                check(
                    "viewport report blocked is stored",
                    status == 201 and viewport_report is not None and viewport_report.get("result") == "blocked" and viewport_report.get("viewport", {}).get("width") == expected_width + 640,
                    f"status={status} result={viewport_report.get('result') if viewport_report else 'missing-file'} viewport={viewport_report.get('viewport') if viewport_report else None}",
                    str(viewport_report_path),
                )
            )

            sensitive_payload = {"viewport": {"width": expected_width, "height": expected_height}, "leak": "Bearer should-not-pass"}
            status, _ = request(
                base_url,
                "POST",
                "/viewport-report",
                json.dumps(sensitive_payload).encode("utf-8"),
                {"Content-Type": "application/json"},
            )
            viewport_report_after_reject = read_json_if_exists(viewport_report_path)
            checks.append(
                check(
                    "viewport report rejects sensitive material",
                    status == 400 and viewport_report_after_reject is not None and viewport_report_after_reject.get("result") == "blocked",
                    f"status={status} preserved_result={viewport_report_after_reject.get('result') if viewport_report_after_reject else 'missing-file'}",
                )
            )

            screenshot_headers = {
                "X-Codex-Capture-Key": capture_key,
                "X-Codex-Desktop-Viewport-Width": str(expected_width),
                "X-Codex-Desktop-Viewport-Height": str(expected_height),
                "X-Codex-Desktop-Device-Pixel-Ratio": "1",
                "Content-Type": "image/png",
            }
            status, _ = request(base_url, "POST", "/upload/selftest", make_png(expected_width, expected_height), screenshot_headers)
            capture_meta_path = evidence_dir / "selftest" / "capture-meta.json"
            capture_meta = read_json_if_exists(capture_meta_path)
            checks.append(
                check(
                    "screenshot upload writes passing capture metadata",
                    status == 201 and capture_meta is not None and capture_meta.get("status") == "pass" and capture_meta.get("post_capture_resize") is False,
                    f"status={status} meta_status={capture_meta.get('status') if capture_meta else 'missing-file'}",
                    str(capture_meta_path),
                )
            )

            status, _ = request(
                base_url,
                "POST",
                "/interaction-screenshot/selftest",
                make_png(expected_width, expected_height),
                screenshot_headers,
            )
            interaction_screenshot_path = evidence_dir / "selftest" / "interaction.png"
            interaction_meta_path = evidence_dir / "selftest" / "interaction-capture-meta.json"
            interaction_meta = read_json_if_exists(interaction_meta_path)
            checks.append(
                check(
                    "interaction screenshot upload writes passing metadata",
                    status == 201
                    and interaction_screenshot_path.is_file()
                    and interaction_meta is not None
                    and interaction_meta.get("status") == "pass"
                    and interaction_meta.get("artifact_type") == "interaction",
                    f"status={status} exists={interaction_screenshot_path.is_file()} meta_status={interaction_meta.get('status') if interaction_meta else 'missing-file'}",
                    str(interaction_screenshot_path),
                )
            )

            interaction_payload = {
                "status": "pass",
                "route_id": "selftest",
                "business_action": "receiver selftest interaction write",
                "desktop_chrome_backend_status": "pass",
                "no_4xx_5xx": True,
                "no_requestfailed": True,
                "no_pageerror": True,
                "no_console_error": True,
            }
            status, _ = request(
                base_url,
                "POST",
                "/interaction/selftest",
                json.dumps(interaction_payload).encode("utf-8"),
                {"X-Codex-Capture-Key": capture_key, "Content-Type": "application/json"},
            )
            interaction_path = evidence_dir / "selftest" / "interaction.json"
            checks.append(
                check(
                    "interaction upload writes JSON",
                    status == 201 and interaction_path.is_file(),
                    f"status={status} exists={interaction_path.is_file()}",
                    str(interaction_path),
                )
            )

            status, _ = request(
                base_url,
                "POST",
                "/interaction/sensitive",
                json.dumps({"leak": "codex_smoke_token=should-not-pass"}).encode("utf-8"),
                {"X-Codex-Capture-Key": capture_key, "Content-Type": "application/json"},
            )
            checks.append(check("interaction upload rejects sensitive material", status == 400, f"status={status}"))

            bridge_result_payload = {
                "ok": True,
                "backend": "codex-desktop-chrome-extension",
                "expected_viewport": {"width": expected_width, "height": expected_height},
                "targets": [{"name": "Chrome", "type": "extension"}],
                "visual": [{"target_id": "selftest", "ok": True}],
                "interactions": [{"route_id": "selftest", "ok": True}],
            }
            status, _ = request(
                base_url,
                "POST",
                "/bridge-result",
                json.dumps(bridge_result_payload).encode("utf-8"),
                {"X-Codex-Capture-Key": capture_key, "Content-Type": "application/json"},
            )
            bridge_result_path = temp_root / "desktop-chrome-bridge-run-latest.json"
            bridge_result = read_json_if_exists(bridge_result_path)
            checks.append(
                check(
                    "bridge result upload writes run summary",
                    status == 201
                    and bridge_result is not None
                    and bridge_result.get("result") == "pass"
                    and bridge_result.get("visual_count") == 1
                    and bridge_result.get("interaction_count") == 1,
                    f"status={status} result={bridge_result.get('result') if bridge_result else 'missing-file'}",
                    str(bridge_result_path),
                )
            )

            status, _ = request(
                base_url,
                "POST",
                "/bridge-result",
                json.dumps({"ok": True, "final_url": "http://local/#codex_smoke_token=should-not-pass"}).encode("utf-8"),
                {"X-Codex-Capture-Key": capture_key, "Content-Type": "application/json"},
            )
            checks.append(check("bridge result upload rejects sensitive material", status == 400, f"status={status}"))
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)

    passed = sum(1 for item in checks if item["passed"])
    result = "pass" if passed == len(checks) else "fail"
    return {
        "package_id": "ui_desktop_capture_receiver_selftest",
        "run_id": "ui-desktop-capture-receiver-selftest-latest",
        "result": result,
        "generated_at": now_iso(),
        "expected_size": {"width": expected_width, "height": expected_height},
        "temporary_evidence_only": True,
        "acceptance_effect": "Proves the receiver endpoint behavior only; does not replace Desktop Chrome visual or interaction evidence.",
        "passed": passed,
        "total": len(checks),
        "checks": checks,
    }


def main() -> int:
    args = parse_args()
    summary = run_selftest(args.expected_width, args.expected_height)
    write_json(args.output_json, summary)
    write_md(args.output_md, summary)
    print(
        f"ui-desktop-capture-receiver-selftest result={summary['result']} "
        f"passed={summary['passed']}/{summary['total']} json={repo_rel(args.output_json)} md={repo_rel(args.output_md)}"
    )
    return 0 if summary["result"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
