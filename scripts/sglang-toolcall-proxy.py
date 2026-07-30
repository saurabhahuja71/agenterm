#!/usr/bin/env python3
"""Tiny OpenAI-compatible proxy that rewrites Qwen/SGLang text tool markup
into real `tool_calls` so clients like VT Code execute tools instead of
printing raw `<tool_call>{"name":...}` as the final reply.

Compatible with Python 3.6+.

  Upstream (SGLang):  http://127.0.0.1:30000
  This proxy:         http://127.0.0.1:30001

  python3 scripts/sglang-toolcall-proxy.py [upstream] [port]
  # point VT Code at http://127.0.0.1:30001/v1
"""

from __future__ import print_function

import json
import re
import sys
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from socketserver import ThreadingMixIn

try:
    from typing import Any, Dict, List, Tuple
except ImportError:
    pass

UPSTREAM = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:30000"
LISTEN_HOST = "127.0.0.1"
LISTEN_PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 30001

TAGGED_JSON = re.compile(
    r"<tool_call>\s*(\{.*?\})\s*</tool_call>",
    re.DOTALL | re.IGNORECASE,
)
BARE_ARRAY = re.compile(r"^\s*(\[\s*\{.*\"name\".*\}\s*\])\s*$", re.DOTALL)


def _as_args(obj):
    # type: (Dict[str, Any]) -> Dict[str, Any]
    args = obj.get("arguments")
    if isinstance(args, dict):
        return args
    params = obj.get("parameters")
    if isinstance(params, dict):
        return params
    skip = set(["name", "arguments", "parameters"])
    return dict((k, v) for k, v in obj.items() if k not in skip)


def extract_text_tools(content):
    # type: (str) -> List[Tuple[str, Dict[str, Any]]]
    found = []  # type: List[Tuple[str, Dict[str, Any]]]
    if not content:
        return found

    for m in TAGGED_JSON.finditer(content):
        try:
            obj = json.loads(m.group(1))
        except ValueError:
            continue
        if not isinstance(obj, dict):
            continue
        name = obj.get("name")
        if not isinstance(name, str) or not name.strip():
            continue
        found.append((name.strip(), _as_args(obj)))

    if found:
        return found

    m = BARE_ARRAY.match(content.strip())
    if m:
        try:
            arr = json.loads(m.group(1))
        except ValueError:
            arr = None
        if isinstance(arr, list):
            for item in arr:
                if not isinstance(item, dict):
                    continue
                name = item.get("name")
                if isinstance(name, str) and name.strip():
                    found.append((name.strip(), _as_args(item)))
    return found


def strip_tool_markup(content):
    # type: (str) -> str
    cleaned = TAGGED_JSON.sub("", content)
    if BARE_ARRAY.match(cleaned.strip()):
        return ""
    return cleaned.strip()


def rewrite_message(msg):
    # type: (Dict[str, Any]) -> Dict[str, Any]
    if not isinstance(msg, dict) or msg.get("role") != "assistant":
        return msg
    if msg.get("tool_calls"):
        return msg
    content = msg.get("content")
    if not isinstance(content, str) or not content.strip():
        return msg

    tools = extract_text_tools(content)
    if not tools:
        return msg

    tool_calls = []
    for i, (name, args) in enumerate(tools):
        tool_calls.append(
            {
                "id": "call_proxy_%d" % i,
                "type": "function",
                "function": {
                    "name": name,
                    "arguments": json.dumps(args, ensure_ascii=False),
                },
            }
        )
    out = dict(msg)
    out["tool_calls"] = tool_calls
    residual = strip_tool_markup(content)
    out["content"] = residual if residual else None
    return out


def rewrite_response_body(body, content_type):
    # type: (bytes, str) -> bytes
    if "json" not in content_type and not body[:1] == b"{":
        return body
    try:
        data = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, ValueError):
        return body
    if not isinstance(data, dict):
        return body
    choices = data.get("choices")
    if not isinstance(choices, list):
        return body
    changed = False
    for ch in choices:
        if not isinstance(ch, dict):
            continue
        msg = ch.get("message")
        if isinstance(msg, dict):
            new_msg = rewrite_message(msg)
            if new_msg is not msg and new_msg.get("tool_calls"):
                ch["message"] = new_msg
                if ch.get("finish_reason") in (None, "stop"):
                    ch["finish_reason"] = "tool_calls"
                changed = True
    if not changed:
        return body
    return json.dumps(data, ensure_ascii=False).encode("utf-8")


class ThreadingHTTPServer(ThreadingMixIn, HTTPServer):
    daemon_threads = True


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        sys.stderr.write("[sglang-toolcall-proxy] " + (fmt % args) + "\n")

    def _proxy(self):
        length = int(self.headers.get("Content-Length", "0") or 0)
        req_body = self.rfile.read(length) if length else b""
        client_wants_stream = False
        path = self.path.rstrip("/")
        is_chat = path.endswith("/chat/completions")

        # Force non-stream upstream for chat so we can rewrite text tools once.
        if is_chat and req_body:
            try:
                payload = json.loads(req_body.decode("utf-8"))
                if isinstance(payload, dict):
                    client_wants_stream = bool(payload.get("stream"))
                    if client_wants_stream:
                        payload["stream"] = False
                        req_body = json.dumps(payload).encode("utf-8")
            except ValueError:
                pass

        url = UPSTREAM.rstrip("/") + self.path
        headers = {}
        for k, v in self.headers.items():
            if k.lower() in ("host", "content-length", "transfer-encoding", "connection"):
                continue
            headers[k] = v
        from urllib.parse import urlparse

        headers["Host"] = urlparse(UPSTREAM).netloc
        # Python 3.6 urllib Request supports method=
        req = urllib.request.Request(
            url, data=req_body if req_body else None, headers=headers, method=self.command
        )
        try:
            resp = urllib.request.urlopen(req, timeout=600)
            try:
                resp_body = resp.read()
                ctype = resp.headers.get("Content-Type", "application/json")
                if is_chat and not resp_body.lstrip().startswith(b"data:"):
                    resp_body = rewrite_response_body(resp_body, ctype)

                if is_chat and client_wants_stream:
                    # Re-emit as a minimal SSE stream so stream clients still work.
                    try:
                        data = json.loads(resp_body.decode("utf-8"))
                    except ValueError:
                        data = None
                    if isinstance(data, dict):
                        sse = (
                            "data: "
                            + json.dumps(data, ensure_ascii=False)
                            + "\n\ndata: [DONE]\n\n"
                        ).encode("utf-8")
                        self.send_response(200)
                        self.send_header("Content-Type", "text/event-stream")
                        self.send_header("Cache-Control", "no-cache")
                        self.send_header("Content-Length", str(len(sse)))
                        self.send_header("Connection", "close")
                        self.end_headers()
                        self.wfile.write(sse)
                        return

                self.send_response(resp.status)
                for k, v in resp.headers.items():
                    if k.lower() in ("transfer-encoding", "content-length", "connection"):
                        continue
                    self.send_header(k, v)
                self.send_header("Content-Length", str(len(resp_body)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(resp_body)
            finally:
                resp.close()
        except urllib.error.HTTPError as e:
            body = e.read()
            self.send_response(e.code)
            self.send_header("Content-Type", e.headers.get("Content-Type", "text/plain"))
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(body)
        except Exception as e:
            msg = json.dumps({"error": {"message": str(e), "type": "proxy_error"}}).encode("utf-8")
            self.send_response(502)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(msg)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(msg)

    def do_GET(self):
        self._proxy()

    def do_POST(self):
        self._proxy()

    def do_OPTIONS(self):
        self._proxy()


def main():
    sample = '<tool_call>\n{"name": "exec_command", "arguments": {"cmd": "ls"}}\n</tool_call>'
    got = extract_text_tools(sample)
    assert got == [("exec_command", {"cmd": "ls"})], got

    httpd = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), Handler)
    print(
        "sglang-toolcall-proxy listening on http://%s:%d -> %s"
        % (LISTEN_HOST, LISTEN_PORT, UPSTREAM),
        flush=True,
    )
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nbye", flush=True)


if __name__ == "__main__":
    main()
