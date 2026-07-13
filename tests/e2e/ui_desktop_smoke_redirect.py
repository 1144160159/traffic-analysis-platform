#!/usr/bin/env python3
"""One-time Desktop Chrome smoke-token redirect helper.

The Codex Desktop Chrome wrapper can open a URL, but its tool output echoes the
requested URL. This helper keeps the short-lived JWT out of that requested URL:
the wrapper opens a nonce-only `/start` URL, and this local helper responds with
a redirect to the Web UI hash token path that the app already knows how to
consume when DESKTOP_SMOKE_TOKEN_ENABLED is enabled for an acceptance window.
"""

from __future__ import annotations

import argparse
import os
import re
import threading
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, quote, urlencode, urlparse


ROUTE_RE = re.compile(r"^/[A-Za-z0-9/_:.-]{0,160}$")
QUERY_RE = re.compile(r"^[A-Za-z0-9_=&%:.,@+/-]{0,320}$")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Redirect Desktop Chrome to a hash smoke-token URL without exposing the token in the wrapper URL.")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--app-base-url", default="http://10.0.5.8:30180")
    parser.add_argument("--default-route", default="/alerts")
    parser.add_argument("--max-redirects", type=int, default=1)
    return parser.parse_args()


def normalize_base_url(value: str) -> str:
    return value.rstrip("/")


def normalize_route(value: str) -> str:
    route = value.strip() or "/alerts"
    if not route.startswith("/"):
        route = f"/{route}"
    if not ROUTE_RE.fullmatch(route):
        raise ValueError("route must be an absolute app path without query, hash, scheme, or unsafe characters")
    if route.startswith("//") or "://" in route or "#" in route or "?" in route:
        raise ValueError("route must not include scheme, query, or hash")
    return route


def normalize_query(value: str) -> str:
    query = value.strip().lstrip("?")
    if not query:
        return ""
    if "#" in query or "://" in query or not QUERY_RE.fullmatch(query):
        raise ValueError("query must not include hash, scheme, or unsafe characters")
    return query


class RedirectState:
    def __init__(self, app_base_url: str, token: str, nonce: str, default_route: str, max_redirects: int) -> None:
        self.app_base_url = normalize_base_url(app_base_url)
        self.token = token
        self.nonce = nonce
        self.default_route = normalize_route(default_route)
        self.max_redirects = max_redirects
        self.redirects = 0
        self.lock = threading.Lock()


def make_handler(state: RedirectState):
    class Handler(BaseHTTPRequestHandler):
        server_version = "TrafficUiSmokeRedirect/1.0"

        def log_message(self, fmt: str, *args) -> None:  # noqa: A003
            safe_args = []
            for arg in args:
                text = str(arg)
                if "codex_smoke_token" in text or state.token in text:
                    text = "<redacted>"
                safe_args.append(text)
            super().log_message(fmt, *safe_args)

        def _send_text(self, code: int, body: str) -> None:
            encoded = body.encode("utf-8")
            self.send_response(code)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        def do_GET(self) -> None:  # noqa: N802
            parsed = urlparse(self.path)
            if parsed.path == "/health":
                self._send_text(HTTPStatus.OK, "ok\n")
                return
            if parsed.path != "/start":
                self._send_text(HTTPStatus.NOT_FOUND, "not found\n")
                return

            params = parse_qs(parsed.query, keep_blank_values=True)
            nonce = params.get("nonce", [""])[0]
            if nonce != state.nonce:
                self._send_text(HTTPStatus.FORBIDDEN, "forbidden\n")
                return

            route_param = params.get("route", [state.default_route])[0]
            query_param = params.get("query", [""])[0]
            try:
                route = normalize_route(route_param)
                query = normalize_query(query_param)
            except ValueError as error:
                self._send_text(HTTPStatus.BAD_REQUEST, f"invalid route/query: {error}\n")
                return

            with state.lock:
                state.redirects += 1
                should_stop = state.redirects >= state.max_redirects

            hash_params = urlencode({"codex_smoke_token": state.token})
            query_suffix = f"?{query}" if query else ""
            location = f"{state.app_base_url}{quote(route, safe='/:-._')}{query_suffix}#{hash_params}"
            self.send_response(HTTPStatus.FOUND)
            self.send_header("Location", location)
            self.send_header("Cache-Control", "no-store")
            self.end_headers()

            if should_stop:
                threading.Thread(target=self.server.shutdown, daemon=True).start()

    return Handler


def main() -> int:
    args = parse_args()
    token = os.environ.get("DESKTOP_SMOKE_TOKEN", "")
    if not token:
        raise SystemExit("DESKTOP_SMOKE_TOKEN is required")
    nonce = os.environ.get("CODEX_SMOKE_NONCE", "")
    if not nonce:
        raise SystemExit("CODEX_SMOKE_NONCE is required")

    state = RedirectState(args.app_base_url, token, nonce, args.default_route, args.max_redirects)
    server = ThreadingHTTPServer((args.host, args.port), make_handler(state))
    print(
        f"ui-desktop-smoke-redirect listening host={args.host} port={args.port} max_redirects={args.max_redirects}",
        flush=True,
    )
    try:
        server.serve_forever()
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
