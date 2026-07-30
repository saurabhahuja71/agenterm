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

def _cli_upstream_port():
    # type: () -> Tuple[str, int]
    args = [a for a in sys.argv[1:] if a not in ("--self-test", "self-test")]
    upstream = args[0] if len(args) > 0 else "http://127.0.0.1:30000"
    port = int(args[1]) if len(args) > 1 else 30001
    return upstream, port


UPSTREAM, LISTEN_PORT = _cli_upstream_port()
LISTEN_HOST = "127.0.0.1"

# Match whole <tool_call>…</tool_call> so we can brace-parse JSON inside
# (str_replace payloads often contain '}' in old_string/new_string).
TAGGED_BLOCK = re.compile(
    r"<tool_call>\s*(.*?)\s*</tool_call>",
    re.DOTALL | re.IGNORECASE,
)
BARE_ARRAY = re.compile(r"^\s*(\[\s*\{.*\"name\".*\}\s*\])\s*$", re.DOTALL)


def _extract_balanced_json(s, start=0):
    # type: (str, int) -> Tuple[str, int]
    """Return (json_text, end_index) for the first balanced {...} or [...]."""
    i = start
    n = len(s)
    while i < n and s[i] not in "{[":
        i += 1
    if i >= n:
        return "", -1
    open_ch = s[i]
    close_ch = "}" if open_ch == "{" else "]"
    depth = 0
    in_str = False
    esc = False
    for j in range(i, n):
        c = s[j]
        if in_str:
            if esc:
                esc = False
            elif c == "\\":
                esc = True
            elif c == '"':
                in_str = False
            continue
        if c == '"':
            in_str = True
            continue
        if c == open_ch:
            depth += 1
        elif c == close_ch:
            depth -= 1
            if depth == 0:
                return s[i : j + 1], j + 1
    return "", -1


def _as_args(obj):
    # type: (Dict[str, Any]) -> Dict[str, Any]
    args = obj.get("arguments")
    if isinstance(args, dict):
        return args
    if isinstance(args, str) and args.strip():
        try:
            parsed = json.loads(args)
            if isinstance(parsed, dict):
                return parsed
        except ValueError:
            pass
    params = obj.get("parameters")
    if isinstance(params, dict):
        return params
    if isinstance(params, str) and params.strip():
        try:
            parsed = json.loads(params)
            if isinstance(parsed, dict):
                return parsed
        except ValueError:
            pass
    skip = set(["name", "arguments", "parameters", "tool", "function"])
    return dict((k, v) for k, v in obj.items() if k not in skip)


def _tool_from_obj(obj):
    # type: (Any) -> Tuple[str, Dict[str, Any]]
    if not isinstance(obj, dict):
        return "", {}
    name = obj.get("name") or obj.get("tool") or obj.get("function")
    if isinstance(name, dict):
        # OpenAI-ish nested {"function":{"name":...,"arguments":...}}
        inner = name
        name = inner.get("name")
        if not obj.get("arguments") and inner.get("arguments") is not None:
            obj = dict(obj)
            obj["arguments"] = inner.get("arguments")
    if not isinstance(name, str) or not name.strip():
        return "", {}
    return name.strip(), _as_args(obj)


def extract_text_tools(content):
    # type: (str) -> List[Tuple[str, Dict[str, Any]]]
    found = []  # type: List[Tuple[str, Dict[str, Any]]]
    if not content:
        return found

    for m in TAGGED_BLOCK.finditer(content):
        inner = (m.group(1) or "").strip()
        if not inner:
            continue
        # Qwen sometimes: <tool_call>str_replace\n{...}</tool_call>
        name_hint = ""
        json_text = ""
        if inner[0] not in "{[":
            lines = inner.split("\n", 1)
            first = lines[0].strip()
            if first and first[0] not in "{[" and re.match(r"^[\w\.\-]+$", first):
                name_hint = first
                rest = lines[1] if len(lines) > 1 else ""
                json_text, _ = _extract_balanced_json(rest, 0)
            else:
                json_text, _ = _extract_balanced_json(inner, 0)
        else:
            json_text, _ = _extract_balanced_json(inner, 0)
        if not json_text:
            continue
        try:
            obj = json.loads(json_text)
        except ValueError:
            continue
        name, args = _tool_from_obj(obj)
        if not name and name_hint:
            name = name_hint
            if isinstance(obj, dict):
                args = obj
            else:
                args = {}
        if name:
            found.append((name, args))

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
                name, args = _tool_from_obj(item)
                if name:
                    found.append((name, args))
    return found


def strip_tool_markup(content):
    # type: (str) -> str
    cleaned = TAGGED_BLOCK.sub("", content)
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


def completion_to_sse(data):
    # type: (Dict[str, Any]) -> bytes
    """Turn a full chat.completion into OpenAI-style SSE *chunks with delta*.

    Clients like agenterm only accumulate tool_calls from ``delta``, not from
    a non-stream ``message`` stuffed into a single SSE frame. Emitting the raw
    completion as ``data: {...message...}`` drops str_replace/etc. tool runs.
    """
    if not isinstance(data, dict):
        payload = json.dumps(data, ensure_ascii=False)
        return ("data: " + payload + "\n\ndata: [DONE]\n\n").encode("utf-8")

    base = {
        "id": data.get("id") or "proxy-rewritten",
        "object": "chat.completion.chunk",
        "created": data.get("created"),
        "model": data.get("model"),
    }
    chunks = []  # type: List[str]

    choices = data.get("choices")
    if not isinstance(choices, list) or not choices:
        chunks.append(json.dumps(dict(base, choices=[]), ensure_ascii=False))
    else:
        for i, ch in enumerate(choices):
            if not isinstance(ch, dict):
                continue
            msg = ch.get("message") or {}
            if not isinstance(msg, dict):
                msg = {}
            role = msg.get("role") or "assistant"
            content = msg.get("content")
            if content is None:
                content = ""
            tool_calls = msg.get("tool_calls")
            finish = ch.get("finish_reason")
            if tool_calls and finish in (None, "stop"):
                finish = "tool_calls"

            # Role (+ optional content) first, matching OpenAI stream shape.
            delta0 = {"role": role}  # type: Dict[str, Any]
            if content:
                delta0["content"] = content
            elif not tool_calls:
                delta0["content"] = content
            chunks.append(
                json.dumps(
                    dict(
                        base,
                        choices=[
                            {
                                "index": ch.get("index", i),
                                "delta": delta0,
                                "finish_reason": None,
                            }
                        ],
                    ),
                    ensure_ascii=False,
                )
            )

            if tool_calls:
                # One chunk with full tool_calls (agenterm concatenates args).
                # Normalize arguments to JSON *strings* (OpenAI wire format).
                norm = []
                for j, tc in enumerate(tool_calls):
                    if not isinstance(tc, dict):
                        continue
                    fn = tc.get("function") or {}
                    if not isinstance(fn, dict):
                        fn = {}
                    args = fn.get("arguments")
                    if isinstance(args, (dict, list)):
                        args = json.dumps(args, ensure_ascii=False)
                    elif args is None:
                        args = "{}"
                    else:
                        args = str(args)
                    norm.append(
                        {
                            "id": tc.get("id") or ("call_proxy_%d" % j),
                            "index": tc.get("index", j),
                            "type": tc.get("type") or "function",
                            "function": {
                                "name": fn.get("name") or "",
                                "arguments": args,
                            },
                        }
                    )
                chunks.append(
                    json.dumps(
                        dict(
                            base,
                            choices=[
                                {
                                    "index": ch.get("index", i),
                                    "delta": {"tool_calls": norm},
                                    "finish_reason": None,
                                }
                            ],
                        ),
                        ensure_ascii=False,
                    )
                )

            chunks.append(
                json.dumps(
                    dict(
                        base,
                        choices=[
                            {
                                "index": ch.get("index", i),
                                "delta": {},
                                "finish_reason": finish or "stop",
                            }
                        ],
                    ),
                    ensure_ascii=False,
                )
            )

    out = "".join("data: %s\n\n" % c for c in chunks) + "data: [DONE]\n\n"
    return out.encode("utf-8")


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
                    # Re-emit as OpenAI delta SSE so stream clients see tool_calls.
                    try:
                        data = json.loads(resp_body.decode("utf-8"))
                    except ValueError:
                        data = None
                    if isinstance(data, dict):
                        sse = completion_to_sse(data)
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

    # str_replace with braces inside strings must still parse
    sr = (
        '<tool_call>\n{"name": "str_replace", "arguments": '
        '{"path": "a.go", "old_string": "func() {}", '
        '"new_string": "func() { return 1 }"}}\n</tool_call>'
    )
    got_sr = extract_text_tools(sr)
    assert len(got_sr) == 1 and got_sr[0][0] == "str_replace", got_sr
    assert got_sr[0][1].get("path") == "a.go", got_sr
    assert "{}" in got_sr[0][1].get("old_string", ""), got_sr

    # Name outside JSON (Qwen dialect)
    named = '<tool_call>str_replace\n{"path": "x", "old_string": "a", "new_string": "b"}\n</tool_call>'
    got_n = extract_text_tools(named)
    assert got_n == [("str_replace", {"path": "x", "old_string": "a", "new_string": "b"})], got_n

    # SSE rewrite uses delta, not message
    full = {
        "id": "t1",
        "created": 1,
        "model": "m",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [
                        {
                            "id": "call_1",
                            "type": "function",
                            "function": {
                                "name": "str_replace",
                                "arguments": '{"path":"README.md","old_string":"OLD","new_string":"NEW"}',
                            },
                        }
                    ],
                },
                "finish_reason": "tool_calls",
            }
        ],
    }
    sse = completion_to_sse(full).decode("utf-8")
    assert "chat.completion.chunk" in sse, sse
    assert '"delta"' in sse, sse
    assert "str_replace" in sse, sse
    assert '"message"' not in sse.split("data: [DONE]")[0] or '"delta"' in sse
    # agenterm-critical: tool_calls under delta
    assert any(
        '"tool_calls"' in line and '"delta"' in line
        for line in sse.split("\n")
        if line.startswith("data: ") and line != "data: [DONE]"
    ), sse

    if len(sys.argv) > 1 and sys.argv[1] in ("--self-test", "self-test"):
        print("sglang-toolcall-proxy self-test OK", flush=True)
        return

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
