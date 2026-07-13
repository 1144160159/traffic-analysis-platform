#!/usr/bin/env python3
"""Short-lived receiver for Desktop Chrome visual and interaction evidence.

The receiver exposes a one-time smoke token endpoint and a constrained screenshot
upload endpoint. Desktop Chrome currently returns JPEG screenshots, so the
receiver stores either PNG uploads directly or converts JPEG uploads to the
PNG file expected by the UI visual gate. It is intended for local test runs
only; it never writes the token to disk or logs it.
"""

from __future__ import annotations

import argparse
import io
import json
import os
import re
import html
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import unquote

from PIL import Image


TARGET_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,80}$")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Receive Desktop Chrome screenshots and interactions for UI evidence.")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--max-uploads", type=int, default=1)
    parser.add_argument("--max-bytes", type=int, default=12 * 1024 * 1024)
    parser.add_argument("--max-json-bytes", type=int, default=1024 * 1024)
    parser.add_argument("--expected-width", type=int, default=1920)
    parser.add_argument("--expected-height", type=int, default=1080)
    return parser.parse_args()


class ReceiverState:
    def __init__(
        self,
        evidence_dir: Path,
        token: str,
        capture_key: str,
        max_uploads: int,
        max_bytes: int,
        max_json_bytes: int,
        expected_width: int,
        expected_height: int,
    ) -> None:
        self.evidence_dir = evidence_dir
        self.token = token
        self.capture_key = capture_key
        self.max_uploads = max_uploads
        self.max_bytes = max_bytes
        self.max_json_bytes = max_json_bytes
        self.expected_width = expected_width
        self.expected_height = expected_height
        self.uploads = 0
        self.lock = threading.Lock()


def make_handler(state: ReceiverState):
    class Handler(BaseHTTPRequestHandler):
        server_version = "TrafficUiCaptureReceiver/1.0"

        def log_message(self, fmt: str, *args) -> None:  # noqa: A003
            safe_args = tuple("<redacted>" if isinstance(arg, str) and "Bearer" in arg else arg for arg in args)
            super().log_message(fmt, *safe_args)

        def _send(self, code: int, body: bytes, content_type: str = "text/plain; charset=utf-8") -> None:
            self.send_response(code)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _authorized(self) -> bool:
            return self.headers.get("X-Codex-Capture-Key", "") == state.capture_key

        def do_GET(self) -> None:  # noqa: N802
            if self.path == "/health":
                self._send(200, b"ok\n")
                return
            if self.path == "/viewport-probe":
                self._send(200, self._viewport_probe_page().encode("utf-8"), "text/html; charset=utf-8")
                return
            if self.path == "/token":
                if not self._authorized():
                    self._send(403, b"forbidden\n")
                    return
                self._send(200, state.token.encode("utf-8"))
                return
            self._send(404, b"not found\n")

        def do_POST(self) -> None:  # noqa: N802
            upload_prefix = "/upload/"
            interaction_screenshot_prefix = "/interaction-screenshot/"
            interaction_prefix = "/interaction/"
            if self.path.startswith(upload_prefix):
                self._receive_screenshot(upload_prefix)
                return
            if self.path.startswith(interaction_screenshot_prefix):
                self._receive_interaction_screenshot(interaction_screenshot_prefix)
                return
            if self.path.startswith(interaction_prefix):
                self._receive_interaction(interaction_prefix)
                return
            if self.path == "/viewport-report":
                self._receive_viewport_report()
                return
            if self.path == "/bridge-result":
                self._receive_bridge_result()
                return
            self._send(404, b"not found\n")

        def _viewport_probe_page(self) -> str:
            expected_width = state.expected_width
            expected_height = state.expected_height
            return f"""<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Desktop Chrome Viewport Probe</title>
  <style>
    html, body {{ margin: 0; width: 100%; height: 100%; background: #071015; color: #e7f7ff; font: 16px/1.45 system-ui, sans-serif; }}
    main {{ box-sizing: border-box; min-height: 100vh; display: grid; place-content: center; gap: 12px; padding: 32px; }}
    strong {{ color: #7cf5c8; }}
    code {{ color: #ffd36a; }}
  </style>
</head>
<body>
  <main>
    <h1>Desktop Chrome Viewport Probe</h1>
    <p id="status">measuring viewport...</p>
    <pre id="payload"></pre>
  </main>
  <script>
    const expected = {{ width: {expected_width}, height: {expected_height} }};
    function measure() {{
      const payload = {{
        status: (window.innerWidth === expected.width && window.innerHeight === expected.height) ? 'pass' : 'blocked',
        expected_size: expected,
        viewport: {{ width: window.innerWidth, height: window.innerHeight }},
        outer_window: {{ width: window.outerWidth, height: window.outerHeight }},
        screen: {{ width: window.screen.width, height: window.screen.height, availWidth: window.screen.availWidth, availHeight: window.screen.availHeight }},
        device_pixel_ratio: window.devicePixelRatio,
        user_agent: navigator.userAgent,
        generated_at: new Date().toISOString(),
        acceptance_effect: 'A blocked probe means visual screenshot upload will be rejected before diff metrics can pass.'
      }};
      document.getElementById('status').innerHTML = payload.status === 'pass'
        ? '<strong>pass</strong>: viewport is <code>' + payload.viewport.width + 'x' + payload.viewport.height + '</code>'
        : '<strong>blocked</strong>: viewport is <code>' + payload.viewport.width + 'x' + payload.viewport.height + '</code>, expected <code>' + expected.width + 'x' + expected.height + '</code>';
      document.getElementById('payload').textContent = JSON.stringify(payload, null, 2);
      fetch('/viewport-report', {{
        method: 'POST',
        headers: {{ 'Content-Type': 'application/json' }},
        body: JSON.stringify(payload)
      }}).catch(() => undefined);
    }}
    window.addEventListener('resize', measure);
    measure();
  </script>
</body>
</html>
"""

        def _read_length(self, max_bytes: int) -> int | None:
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                self._send(411, b"invalid content length\n")
                return None
            if length <= 0 or length > max_bytes:
                self._send(413, b"invalid upload size\n")
                return None
            return length

        def _target_id(self, prefix: str) -> str | None:
            target_id = unquote(self.path[len(prefix) :]).strip("/")
            if not TARGET_RE.fullmatch(target_id):
                self._send(400, b"invalid target id\n")
                return None
            return target_id

        def _receive_screenshot(self, prefix: str) -> None:
            self._receive_image(prefix, "actual-1920.png", "capture-meta.json", "visual")

        def _receive_interaction_screenshot(self, prefix: str) -> None:
            self._receive_image(prefix, "interaction.png", "interaction-capture-meta.json", "interaction")

        def _receive_image(self, prefix: str, output_name: str, meta_name: str, artifact_type: str) -> None:
            if not self._authorized():
                self._send(403, b"forbidden\n")
                return

            target_id = self._target_id(prefix)
            if target_id is None:
                return

            length = self._read_length(state.max_bytes)
            if length is None:
                return

            body = self.rfile.read(length)
            is_png = body.startswith(b"\x89PNG\r\n\x1a\n")
            is_jpeg = body.startswith(b"\xff\xd8\xff")
            if not is_png and not is_jpeg:
                self._send(415, b"expected png or jpeg\n")
                return

            target_dir = state.evidence_dir / target_id
            target_dir.mkdir(parents=True, exist_ok=True)
            output = target_dir / output_name
            with Image.open(io.BytesIO(body)) as image:
                uploaded_width, uploaded_height = image.size
                image_format = "png" if is_png else "jpeg"
                if is_png:
                    output.write_bytes(body)
                else:
                    image.convert("RGBA").save(output, format="PNG")

            with Image.open(output) as stored_image:
                stored_width, stored_height = stored_image.size

            viewport_width = self.headers.get("X-Codex-Desktop-Viewport-Width", "")
            viewport_height = self.headers.get("X-Codex-Desktop-Viewport-Height", "")
            viewport_size_ok = (
                viewport_width == str(state.expected_width)
                and viewport_height == str(state.expected_height)
            )
            size_ok = (
                uploaded_width == state.expected_width
                and uploaded_height == state.expected_height
                and stored_width == state.expected_width
                and stored_height == state.expected_height
                and viewport_size_ok
            )
            capture_meta = {
                "status": "pass" if size_ok else "blocked",
                "target_id": target_id,
                "artifact_type": artifact_type,
                "backend": "codex-desktop-chrome-extension",
                "uploaded_format": image_format,
                "uploaded_size": {"width": uploaded_width, "height": uploaded_height},
                "stored_file": str(output),
                "stored_format": "png",
                "stored_size": {"width": stored_width, "height": stored_height},
                "expected_size": {"width": state.expected_width, "height": state.expected_height},
                "receiver_conversion": "jpeg-to-png" if is_jpeg else "none",
                "post_capture_resize": False,
                "desktop_viewport": {
                    "width": viewport_width,
                    "height": viewport_height,
                    "device_pixel_ratio": self.headers.get("X-Codex-Desktop-Device-Pixel-Ratio", ""),
                },
                "desktop_viewport_size_ok": viewport_size_ok,
                "required_for_visual_gate": artifact_type == "visual",
                "required_for_interaction_gate": artifact_type == "interaction",
            }
            (target_dir / meta_name).write_text(
                json.dumps(capture_meta, ensure_ascii=False, indent=2) + "\n",
                encoding="utf-8",
            )

            with state.lock:
                state.uploads += 1
                should_stop = state.uploads >= state.max_uploads

            self._send(201, f"stored {artifact_type} {target_id}\n".encode("utf-8"))
            if should_stop:
                threading.Thread(target=self.server.shutdown, daemon=True).start()

        def _receive_interaction(self, prefix: str) -> None:
            if not self._authorized():
                self._send(403, b"forbidden\n")
                return

            target_id = self._target_id(prefix)
            if target_id is None:
                return

            length = self._read_length(state.max_json_bytes)
            if length is None:
                return

            body = self.rfile.read(length)
            if re.search(rb"(Bearer\s+[A-Za-z0-9._-]+|codex_smoke_token=|refresh_token)", body, re.IGNORECASE):
                self._send(400, b"interaction contains sensitive token material\n")
                return

            try:
                data = json.loads(body.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError):
                self._send(400, b"invalid interaction json\n")
                return
            if not isinstance(data, dict):
                self._send(400, b"interaction json must be an object\n")
                return

            target_dir = state.evidence_dir / target_id
            target_dir.mkdir(parents=True, exist_ok=True)
            output = target_dir / "interaction.json"
            output.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            self._send(201, f"stored interaction {target_id}\n".encode("utf-8"))

        def _receive_bridge_result(self) -> None:
            if not self._authorized():
                self._send(403, b"forbidden\n")
                return

            length = self._read_length(state.max_json_bytes)
            if length is None:
                return

            body = self.rfile.read(length)
            if re.search(rb"(Bearer\s+[A-Za-z0-9._-]+|codex_smoke_token=|refresh_token)", body, re.IGNORECASE):
                self._send(400, b"bridge result contains sensitive token material\n")
                return

            try:
                data = json.loads(body.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError):
                self._send(400, b"invalid bridge result json\n")
                return
            if not isinstance(data, dict):
                self._send(400, b"bridge result json must be an object\n")
                return

            status = "pass" if data.get("ok") is True else "fail"
            summary = {
                "package_id": "desktop_chrome_bridge_run_result",
                "status": status,
                "result": status,
                "generated_by": "ui_desktop_capture_receiver.py",
                "backend": data.get("backend"),
                "expected_viewport": data.get("expected_viewport"),
                "visual_count": len(data.get("visual") or []),
                "interaction_count": len(data.get("interactions") or []),
                "targets": data.get("targets") or [],
                "raw_result": data,
                "acceptance_effect": "Documents the Desktop Chrome payload run summary. It does not replace per-route screenshots, capture metadata, visual diff metrics, or interaction evidence.",
            }
            output_dir = state.evidence_dir.parent
            output_dir.mkdir(parents=True, exist_ok=True)
            output_json = output_dir / "desktop-chrome-bridge-run-latest.json"
            output_md = output_dir / "desktop-chrome-bridge-run-latest.md"
            output_json.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            output_md.write_text(
                "\n".join(
                    [
                        "# Desktop Chrome Bridge Run",
                        "",
                        f"- Result: `{status}`",
                        f"- Backend: `{html.escape(str(data.get('backend', '')) )}`",
                        f"- Visual captures: `{summary['visual_count']}`",
                        f"- Interaction captures: `{summary['interaction_count']}`",
                        "",
                        "This run summary is generated from the Desktop Chrome payload. The formal gate still depends on per-target screenshot, capture metadata, metrics, and interaction evidence.",
                        "",
                    ]
                ),
                encoding="utf-8",
            )

            with state.lock:
                state.uploads += 1
                should_stop = state.uploads >= state.max_uploads

            self._send(201, f"stored bridge result {status}\n".encode("utf-8"))
            if should_stop:
                threading.Thread(target=self.server.shutdown, daemon=True).start()

        def _receive_viewport_report(self) -> None:
            length = self._read_length(state.max_json_bytes)
            if length is None:
                return

            body = self.rfile.read(length)
            try:
                data = json.loads(body.decode("utf-8"))
            except (UnicodeDecodeError, json.JSONDecodeError):
                self._send(400, b"invalid viewport json\n")
                return
            if not isinstance(data, dict):
                self._send(400, b"viewport json must be an object\n")
                return
            if re.search(r"(Bearer\s+[A-Za-z0-9._-]+|codex_smoke_token=|refresh_token)", json.dumps(data), re.IGNORECASE):
                self._send(400, b"viewport report contains sensitive token material\n")
                return

            viewport = data.get("viewport") if isinstance(data.get("viewport"), dict) else {}
            width = int(viewport.get("width") or 0)
            height = int(viewport.get("height") or 0)
            status = "pass" if width == state.expected_width and height == state.expected_height else "blocked"
            report = {
                "result": status,
                "status": status,
                "package_id": "desktop_chrome_viewport_probe",
                "generated_by": "ui_desktop_capture_receiver.py",
                "expected_size": {"width": state.expected_width, "height": state.expected_height},
                "viewport": {"width": width, "height": height},
                "outer_window": data.get("outer_window") if isinstance(data.get("outer_window"), dict) else {},
                "screen": data.get("screen") if isinstance(data.get("screen"), dict) else {},
                "device_pixel_ratio": data.get("device_pixel_ratio"),
                "user_agent": data.get("user_agent", ""),
                "acceptance_effect": "pass is required before collecting 1920x1080 visual screenshots; this probe does not replace capture-meta.json",
                "raw_report": data,
            }
            output_dir = state.evidence_dir.parent
            output_dir.mkdir(parents=True, exist_ok=True)
            output_json = output_dir / "desktop-chrome-viewport-probe-latest.json"
            output_md = output_dir / "desktop-chrome-viewport-probe-latest.md"
            output_json.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            output_md.write_text(
                "\n".join(
                    [
                        "# Desktop Chrome Viewport Probe",
                        "",
                        f"- Result: `{status}`",
                        f"- Viewport: `{width}x{height}`",
                        f"- Expected: `{state.expected_width}x{state.expected_height}`",
                        f"- Device pixel ratio: `{html.escape(str(data.get('device_pixel_ratio', '')) )}`",
                        "",
                        "This probe is a pre-capture calibration check only. It does not replace `capture-meta.json`, `actual-1920.png`, `diff-1920.png`, `metrics.json`, or route `interaction.json` evidence.",
                        "",
                    ]
                ),
                encoding="utf-8",
            )
            self._send(201, f"stored viewport probe {status}\n".encode("utf-8"))

    return Handler


def main() -> int:
    args = parse_args()
    token = os.environ.get("DESKTOP_SMOKE_TOKEN", "")
    if not token:
        raise SystemExit("DESKTOP_SMOKE_TOKEN is required")
    capture_key = os.environ.get("CODEX_CAPTURE_KEY", "")
    if not capture_key:
        raise SystemExit("CODEX_CAPTURE_KEY is required")

    state = ReceiverState(
        args.evidence_dir,
        token,
        capture_key,
        args.max_uploads,
        args.max_bytes,
        args.max_json_bytes,
        args.expected_width,
        args.expected_height,
    )
    server = ThreadingHTTPServer((args.host, args.port), make_handler(state))
    print(f"ui-desktop-capture-receiver listening host={args.host} port={args.port} max_uploads={args.max_uploads}", flush=True)
    try:
        server.serve_forever()
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
