"""Run an isolated new-api routing fault demo using only Python's standard library.

python -B tools/routing-fault-demo/run.py --binary <built gateway>
"""

import argparse
from http.client import HTTPException
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
import math
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from urllib.error import URLError
from urllib.parse import urlsplit

BASE = Path(__file__).resolve().parent
ROOT = BASE.parents[1]
MODEL = "route-demo-model"
USERNAME = "monitorroot"
SCENARIOS = {
    "normal": ("正常运行", "A / B / C 均健康，低折扣 A 优先承接流量。", 20),
    "slow_ttft": ("A 首字延迟", "A 首个 Token 延迟 900 ms，超过 350 ms 门槛。", 25),
    "slow_tpot": ("A 出字变慢", "A 每个 Token 间隔 300 ms，超过 100 ms 门槛。", 25),
    "http429": ("A 返回 429", "A 持续返回上游限流；网关按实际错误结果调整。", 20),
    "http502": ("A 返回 502", "A 持续返回上游 502；观察熔断与备用渠道承接。", 20),
    "disconnect": ("A 流式断连", "A 返回部分内容后意外断开 TCP 连接。", 25),
    "dual_failure": ("A + B 同时异常", "A、B 均返回 502，观察 C 承接真实流量。", 25),
    "recovery": ("全部恢复", "全部上游恢复正常，等待真实探测与逐步回流。", 70),
}
CYCLE = [
    "normal",
    "slow_ttft",
    "recovery",
    "slow_tpot",
    "recovery",
    "http429",
    "recovery",
    "http502",
    "recovery",
    "disconnect",
    "recovery",
    "dual_failure",
    "recovery",
]
NAMES = ["A · 低折扣主渠道（5折）", "B · 备用渠道（6折）", "C · 兜底渠道（8折）"]
NETWORK_ERRORS = (OSError, URLError, HTTPException, TimeoutError)

# Reuse the real management/relay client from the existing local E2E fixture.
# Disable bytecode output so imports do not dirty the source checkout.
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "capacity_fixture", ROOT / "tools/channel-capacity-e2e/run.py"
)
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)
Client = fixture.Client


def gateway_environment(output, port):
    """Use a fresh environment so inherited DB/cache/telemetry secrets stay out."""
    inherited = {
        "PATH",
        "SYSTEMROOT",
        "WINDIR",
        "COMSPEC",
        "PATHEXT",
        "TEMP",
        "TMP",
        "TMPDIR",
        "HOME",
        "USERPROFILE",
        "LANG",
        "LC_ALL",
        "LD_LIBRARY_PATH",
        "DYLD_LIBRARY_PATH",
    }
    env = {key: value for key, value in os.environ.items() if key.upper() in inherited}
    env.update(
        {
            "PORT": str(port),
            "SQL_DSN": "",
            "LOG_SQL_DSN": "",
            "SQLITE_PATH": "file:"
            + (output / "gateway.db").as_posix()
            + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)",
            "SQL_MAX_OPEN_CONNS": "1",
            "REDIS_CONN_STRING": "",
            "SESSION_SECRET": secrets.token_hex(32),
            "NODE_NAME": "local-fault-demo",
            "NODE_TYPE": "master",
            "MEMORY_CACHE_ENABLED": "true",
            "GIN_MODE": "release",
            "CountToken": "false",
            "GLOBAL_API_RATE_LIMIT_ENABLE": "false",
            "GLOBAL_WEB_RATE_LIMIT_ENABLE": "false",
            "CRITICAL_RATE_LIMIT_ENABLE": "false",
            "SEARCH_RATE_LIMIT_ENABLE": "false",
            "HTTP_PROXY": "",
            "HTTPS_PROXY": "",
            "ALL_PROXY": "",
            "NO_PROXY": "127.0.0.1,localhost",
            "SHUTDOWN_TIMEOUT_SECONDS": "5",
        }
    )
    return env


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class Demo:
    def __init__(self, output, rate=16, workers=10):
        self.run = output
        self.rate, self.workers = rate, workers
        self.password = secrets.token_urlsafe(18)
        self.lock = threading.Lock()
        self.stop = threading.Event()
        self.scenario, self.auto = "normal", False
        self.cycle_index, self.deadline = 0, 0
        self.events = [
            {
                "at": int(time.time() * 1000),
                "label": "启动本地故障演示 · 自动轮播已关闭",
            }
        ]
        self.traffic = {"sent": 0, "success": 0, "failure": 0, "inflight": 0}
        self.stats = [{"success": 0, "failure": 0} for _ in range(3)]
        self.next_request = time.monotonic()
        self.channel_ids = []
        self.cached_monitor, self.cache_at, self.monitor_stale = None, 0, True
        self.monitor_lock = threading.Lock()
        self.gateway_url, self.control_url, self.admin_token = "", "", ""
        self.process = None

    def behavior(self, index):
        with self.lock:
            scene = self.scenario
        if scene == "dual_failure" and index < 2:
            return "http502"
        if index == 0 and scene not in ["normal", "recovery", "dual_failure"]:
            return scene
        return "normal"

    def set_scenario(self, scenario, automatic=False):
        if scenario not in SCENARIOS:
            raise ValueError("Unknown scenario")
        with self.lock:
            self.scenario = scenario
            if not automatic:
                self.auto = False
            self.events.insert(
                0, {"at": int(time.time() * 1000), "label": SCENARIOS[scenario][0]}
            )
            self.events = self.events[:12]
        print("SCENARIO " + scenario, flush=True)

    def set_auto(self, enabled):
        with self.lock:
            self.auto = enabled
            self.cycle_index = 0
            self.deadline = time.monotonic() + SCENARIOS["normal"][2]
        if enabled:
            self.set_scenario("normal", automatic=True)

    def wait_for(self, function, timeout=60):
        deadline = time.monotonic() + timeout
        while not self.stop.is_set():
            if self.process and self.process.poll() is not None:
                raise RuntimeError("Gateway exited; see gateway.log")
            try:
                value = function()
                if value:
                    return value
            except NETWORK_ERRORS:
                pass
            if time.monotonic() >= deadline:
                raise RuntimeError("Local gateway timeout; see gateway.log")
            self.stop.wait(0.2)
        raise InterruptedError("Demo stopped")

    def snapshot(self):
        with self.monitor_lock:
            if time.monotonic() - self.cache_at >= 1:
                self.cache_at = time.monotonic()
                try:
                    self.cached_monitor = Client(
                        self.gateway_url, self.admin_token
                    ).api("GET", "/api/channel/routing-monitor")
                    self.monitor_stale = False
                except (*NETWORK_ERRORS, AssertionError):
                    self.monitor_stale = True
            monitor, stale = self.cached_monitor, self.monitor_stale
        route = next(
            (
                route
                for node in (monitor or {}).get("nodes", [])
                for route in node["routes"]
                if route["model"] == MODEL
                and route["group"] == "default"
                and route["policy"] == ""
            ),
            None,
        )
        channel_data = {
            row["channel_id"]: row for row in (route or {}).get("channels", [])
        }
        with self.lock:
            scene = self.scenario
            result = {
                "at": int(time.time() * 1000),
                "scenario": scene,
                "label": SCENARIOS[scene][0],
                "description": SCENARIOS[scene][1],
                "auto": self.auto,
                "remaining_seconds": max(0, int(self.deadline - time.monotonic())),
                "next_label": SCENARIOS[CYCLE[(self.cycle_index + 1) % len(CYCLE)]][0],
                "gateway_url": self.gateway_url,
                "control_url": self.control_url,
                "traffic": dict(self.traffic),
                "rate": self.rate,
                "workers": self.workers,
                "events": list(self.events),
                "channels": [],
                "monitor": monitor,
                "monitor_stale": stale,
            }
            stats = [dict(row) for row in self.stats]
        behavior_names = {
            "normal": "正常",
            "slow_ttft": "首字延迟 900 ms",
            "slow_tpot": "Token 间隔 300 ms",
            "http429": "HTTP 429 限流",
            "http502": "HTTP 502 异常",
            "disconnect": "流式意外断连",
        }
        for index, channel_id in enumerate(self.channel_ids):
            row = channel_data.get(channel_id, {})
            result["channels"].append(
                {
                    "channel_id": channel_id,
                    "name": NAMES[index],
                    "behavior": behavior_names[self.behavior(index)],
                    **stats[index],
                    "target_share": row.get("target_share"),
                    "ttft_ms": row.get("ttft_ms"),
                    "tpot_ms": row.get("tpot_ms"),
                }
            )
        return result

    def pump(self, token):
        client = Client(self.gateway_url, token)
        while not self.stop.is_set():
            with self.lock:
                due = max(time.monotonic(), self.next_request)
                self.next_request = due + 1 / self.rate
            if self.stop.wait(max(0, due - time.monotonic())):
                break
            with self.lock:
                self.traffic["sent"] += 1
                self.traffic["inflight"] += 1
            success = False
            try:
                status, _, body = client.chat(MODEL, stream=True, max_tokens=None)
                success = status == 200 and isinstance(body, str) and "[DONE]" in body
            except NETWORK_ERRORS:
                pass
            finally:
                with self.lock:
                    self.traffic["inflight"] -= 1
                    self.traffic["success" if success else "failure"] += 1


class Upstream(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        state, index = self.server.demo, self.server.index
        mode = state.behavior(index)
        with state.lock:
            state.stats[index][
                "failure" if mode in ["http429", "http502", "disconnect"] else "success"
            ] += 1
        if mode in ["http429", "http502"]:
            status = 429 if mode == "http429" else 502
            code = "rate_limit_exceeded" if status == 429 else "upstream_failed"
            data = json.dumps(
                {
                    "error": {
                        "message": "Synthetic local upstream " + str(status),
                        "type": "server_error",
                        "code": code,
                    }
                }
            ).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Connection", "close")
            if status == 429:
                self.send_header("Retry-After", "1")
            self.end_headers()
            self.wfile.write(data)
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        if mode == "disconnect":
            self.send_header("Content-Length", "99999")
        self.end_headers()
        try:
            if state.stop.wait(0.9 if mode == "slow_ttft" else 0.035):
                return
            for part, content in enumerate(["本地", "故障", "演示", "完成"]):
                payload = {
                    "id": "synthetic-" + str(index),
                    "object": "chat.completion.chunk",
                    "model": MODEL,
                    "choices": [
                        {
                            "index": 0,
                            "delta": {"role": "assistant", "content": content},
                            "finish_reason": None,
                        }
                    ],
                }
                self.wfile.write(("data: " + json.dumps(payload) + "\n\n").encode())
                self.wfile.flush()
                if mode == "disconnect" and part == 1:
                    self.connection.shutdown(socket.SHUT_RDWR)
                    self.connection.close()
                    return
                if part < 3 and state.stop.wait(0.3 if mode == "slow_tpot" else 0.03):
                    return
            final = {
                "id": "synthetic-" + str(index),
                "object": "chat.completion.chunk",
                "model": MODEL,
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                "usage": {
                    "prompt_tokens": 1,
                    "completion_tokens": 4,
                    "total_tokens": 5,
                },
            }
            self.wfile.write(
                ("data: " + json.dumps(final) + "\n\ndata: [DONE]\n\n").encode()
            )
            self.wfile.flush()
        except NETWORK_ERRORS:
            pass


class Control(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def reply(self, status, data, kind="application/json; charset=utf-8"):
        payload = (
            data
            if isinstance(data, bytes)
            else json.dumps(data, ensure_ascii=False).encode()
        )
        self.send_response(status)
        self.send_header("Content-Type", kind)
        self.send_header("Content-Length", str(len(payload)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self):
        if self.headers.get("Host") != urlsplit(self.server.demo.control_url).netloc:
            return self.reply(403, {"error": "Invalid local host"})
        if self.path == "/":
            self.reply(
                200, (BASE / "control.html").read_bytes(), "text/html; charset=utf-8"
            )
        elif self.path == "/api/state":
            self.reply(200, self.server.demo.snapshot())
        else:
            self.reply(404, {"error": "Not found"})

    def do_POST(self):
        state = self.server.demo
        if self.path != "/api/scenario":
            return self.reply(404, {"error": "Not found"})
        if (
            self.headers.get("Host") != urlsplit(state.control_url).netloc
            or self.headers.get("Origin", state.control_url) != state.control_url
            or self.headers.get("Content-Type", "").split(";")[0].strip()
            != "application/json"
        ):
            return self.reply(
                403, {"error": "Only this local control page may change scenarios"}
            )
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if not 0 < size < 512:
                raise ValueError("Invalid body size")
            body = json.loads(self.rfile.read(size))
            if not isinstance(body, dict):
                raise ValueError("Invalid command")
            if (
                set(body) == {"scenario"}
                and isinstance(body["scenario"], str)
                and body["scenario"] in SCENARIOS
            ):
                state.set_scenario(body["scenario"])
            elif set(body) == {"auto"} and type(body["auto"]) is bool:
                state.set_auto(body["auto"])
            else:
                raise ValueError("Invalid command")
        except (ValueError, KeyError, TypeError):
            return self.reply(400, {"error": "Invalid command"})
        self.reply(200, {"success": True})


def create_token(dashboard, name):
    dashboard.api(
        "POST",
        "/api/token/",
        {"name": name, "unlimited_quota": True, "expired_time": -1, "group": "default"},
    )
    token_id = next(
        row["id"]
        for row in dashboard.api("GET", "/api/token/?page_size=100")["items"]
        if row["name"] == name
    )
    key = dashboard.api("POST", f"/api/token/{token_id}/key")["key"]
    return key if key.startswith("sk-") else "sk-" + key


def seed_gateway(state, upstreams):
    anonymous = Client(state.gateway_url)
    state.wait_for(lambda: anonymous.api("GET", "/api/setup"))
    anonymous.api(
        "POST",
        "/api/setup",
        {
            "username": USERNAME,
            "password": state.password,
            "confirmPassword": state.password,
        },
    )
    login = anonymous.api(
        "POST", "/api/user/login", {"username": USERNAME, "password": state.password}
    )
    state.admin_token = login["access_token"]
    admin = Client(state.gateway_url, state.admin_token)
    for key, value in {
        "ModelRatio": json.dumps({MODEL: 0}),
        "RetryTimes": "2",
        "ModelRequestRateLimitEnabled": "false",
        "performance_setting.monitor_enabled": "false",
        "GroupRatio": '{"default":1}',
        "UserUsableGroups": '{"default":"本地演示分组"}',
        "AutomaticDisableChannelEnabled": "false",
        "AutomaticEnableChannelEnabled": "false",
    }.items():
        admin.api("PUT", "/api/option/", {"key": key, "value": value})
    settings = {
        "enabled": True,
        "max_samples": 15,
        "max_age_seconds": 90,
        "min_samples": 3,
        "probe_fraction": 0.015,
        "degradation_threshold": 1.3,
        "recovery_threshold": 1.1,
        "critical_threshold": 1.9,
        "candidate_advantage": 1.1,
        "aggressiveness": 0.9,
        "recovery_step": 0.15,
        "cooldown_seconds": 2,
        "hard_failure_threshold": 1,
        "hard_failure_cooldown_seconds": 8,
    }
    admin.api("PUT", "/api/option/dynamic_routing", settings)
    root_token = create_token(admin, "local-fault-demo")
    for index, upstream in enumerate(upstreams):
        admin.api(
            "POST",
            "/api/channel/",
            {
                "mode": "single",
                "channel": {
                    "name": NAMES[index],
                    "type": 1,
                    "key": "synthetic-local-only",
                    "status": 1,
                    "models": MODEL,
                    "group": "default",
                    "priority": 100 - index * 10,
                    "weight": 1,
                    "cost_discount": [0.5, 0.6, 0.8][index],
                    "base_url": f"http://127.0.0.1:{upstream.server_port}",
                },
            },
        )
        channel_id = next(
            row["id"]
            for row in admin.api("GET", "/api/channel/?page_size=100")["items"]
            if row["name"] == NAMES[index]
        )
        state.channel_ids.append(channel_id)
        admin.api(
            "PATCH",
            f"/api/channel/{channel_id}/model-routing-overrides",
            {
                "overrides": [
                    {"model": MODEL, "rpm_override": None, "tpm_override": None}
                ],
            },
        )
    admin.api(
        "PUT",
        "/api/channel/routing-pool",
        {
            "routing_group": "default",
            "model": MODEL,
            "mode": "discount",
            "revision": "",
            "max_ttft_ms": 350,
            "max_tpot_ms": 100,
            "channels": [
                {
                    "channel_id": channel_id,
                    "priority": 100 - index * 10,
                    "weight": 1,
                    "enabled": True,
                }
                for index, channel_id in enumerate(state.channel_ids)
            ],
        },
    )
    # Real pinned relay requests warm samples; no controller state is injected.
    for channel_id in state.channel_ids:
        pinned = Client(state.gateway_url, root_token + "-" + str(channel_id))
        for _ in range(4):
            status, _, body = pinned.chat(MODEL, stream=True, max_tokens=None)
            if status != 200 or not isinstance(body, str) or "[DONE]" not in body:
                raise RuntimeError("Local upstream warmup failed; see gateway.log")
    user_tokens, user_ids = [], {}
    for username in ["alice", "bob"]:
        admin.api(
            "POST",
            "/api/user/",
            {"username": username, "password": state.password, "role": 1},
        )
        auth = anonymous.api(
            "POST",
            "/api/user/login",
            {"username": username, "password": state.password},
        )
        user_id = auth["user"]["id"]
        user_ids[username] = user_id
        admin.api(
            "POST",
            "/api/user/manage",
            {
                "id": user_id,
                "action": "add_quota",
                "mode": "override",
                "value": 1000000,
            },
        )
        dashboard = Client(state.gateway_url, auth["access_token"])
        user_tokens.append(create_token(dashboard, "local-demo"))
        if username == "alice":
            config = admin.api(
                "GET",
                f"/api/user/{user_id}/channel-routing-overrides?view=pool&model={MODEL}&routing_group=default",
            )
            admin.api(
                "PATCH",
                f"/api/user/{user_id}/channel-routing-overrides",
                {
                    "model": MODEL,
                    "routing_group": "default",
                    "revision": config["revision"],
                    "overrides": [
                        {
                            "channel_id": channel_id,
                            "priority_override": 100 - index * 10,
                            "weight_override": 1,
                            "enabled": index > 0,
                        }
                        for index, channel_id in enumerate(state.channel_ids)
                    ],
                },
            )
    return root_token, user_tokens, user_ids


def arguments(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--binary",
        required=True,
        type=Path,
        help="Built gateway with routing monitor support",
    )
    parser.add_argument(
        "--output",
        type=Path,
        help="New run directory (default: unique system temp directory)",
    )
    parser.add_argument(
        "--rate",
        type=float,
        default=16,
        help="Total client requests/sec upper bound (default: 16)",
    )
    parser.add_argument(
        "--workers",
        type=int,
        default=10,
        help="Maximum concurrent requests, 3–32 (default: 10)",
    )
    parser.add_argument(
        "--gateway-port",
        type=int,
        default=0,
        help="Gateway port, 0 chooses an available port",
    )
    parser.add_argument(
        "--control-port",
        type=int,
        default=0,
        help="Control page loopback port, 0 chooses an available port",
    )
    parser.add_argument(
        "--duration",
        type=float,
        default=0,
        help="Stop N seconds after READY; 0 waits for Ctrl+C or output/stop",
    )
    parser.add_argument(
        "--auto",
        action="store_true",
        help="Start automatic scenario cycling (default: manual)",
    )
    args = parser.parse_args(argv)
    if not args.binary.resolve().is_file():
        parser.error("--binary must point to an existing built gateway")
    if (
        not math.isfinite(args.rate)
        or not 0 < args.rate <= 100
        or not 3 <= args.workers <= 32
    ):
        parser.error("--rate must be in (0,100] and --workers in [3,32]")
    if not math.isfinite(args.duration) or args.duration < 0:
        parser.error("--duration must be finite and >= 0")
    if any(not 0 <= port <= 65535 for port in [args.gateway_port, args.control_port]):
        parser.error("Ports must be in [0,65535]")
    return args


def main(argv=None):
    args = arguments(argv)
    if args.output:
        output = args.output.resolve()
        output.mkdir(parents=True, exist_ok=False)
    else:
        output = Path(tempfile.mkdtemp(prefix="new-api-routing-demo-")).resolve()
    state = Demo(output, args.rate, args.workers)
    upstreams, threads = [], []
    control, log = None, None
    control_started = False
    previous_handlers = {
        kind: signal.getsignal(kind) for kind in [signal.SIGINT, signal.SIGTERM]
    }
    for kind in previous_handlers:
        signal.signal(kind, lambda *_: state.stop.set())
    try:
        for index in range(3):
            upstream = ThreadingHTTPServer(("127.0.0.1", 0), Upstream)
            upstream.demo, upstream.index = state, index
            threading.Thread(target=upstream.serve_forever, daemon=True).start()
            upstreams.append(upstream)
        control = ThreadingHTTPServer(("127.0.0.1", args.control_port), Control)
        control.demo = state
        state.control_url = f"http://127.0.0.1:{control.server_port}"
        state.gateway_url = f"http://127.0.0.1:{args.gateway_port or free_port()}"
        log = (output / "gateway.log").open("w", encoding="utf-8")
        print("OUTPUT " + str(output), flush=True)
        state.process = subprocess.Popen(
            [str(args.binary.resolve())],
            cwd=output,
            env=gateway_environment(output, urlsplit(state.gateway_url).port),
            stdout=log,
            stderr=subprocess.STDOUT,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
        )
        root_token, user_tokens, user_ids = seed_gateway(state, upstreams)
        threading.Thread(target=control.serve_forever, daemon=True).start()
        control_started = True
        # Most requests target the base pool; Alice exercises B/C-only isolation.
        tokens = [root_token] * (args.workers - 2) + user_tokens
        for token in tokens:
            thread = threading.Thread(target=state.pump, args=(token,), daemon=True)
            thread.start()
            threads.append(thread)
        admin = Client(state.gateway_url, state.admin_token)
        state.wait_for(
            lambda: any(
                route["selection_count"] > 0
                for node in admin.api("GET", "/api/channel/routing-monitor")["nodes"]
                for route in node["routes"]
                if route["model"] == MODEL
            ),
            timeout=30,
        )
        private = {
            "gateway_url": state.gateway_url,
            "control_url": state.control_url,
            "username": USERNAME,
            "password": state.password,
            "access_token": state.admin_token,
            "root_token": root_token,
            "user_ids": user_ids,
            "channel_ids": state.channel_ids,
            "upstream_ports": [upstream.server_port for upstream in upstreams],
            "model": MODEL,
            "pid": state.process.pid,
            "stop_file": str(output / "stop"),
        }
        with (output / "credentials.json").open("x", encoding="utf-8") as credentials:
            if os.name != "nt":
                os.chmod(credentials.name, 0o600)
            credentials.write(json.dumps(private, ensure_ascii=False, indent=2))
        if args.auto:
            state.set_auto(True)
        (output / "state.json").write_text(
            json.dumps(state.snapshot(), ensure_ascii=False, indent=2), encoding="utf-8"
        )
        print("GATEWAY " + state.gateway_url, flush=True)
        print("CONTROL " + state.control_url, flush=True)
        print(
            "LOGIN "
            + USERNAME
            + " (password in "
            + str(output / "credentials.json")
            + ")",
            flush=True,
        )
        print(
            "READY real requests / isolated SQLite / "
            + ("auto ON" if args.auto else "auto OFF"),
            flush=True,
        )
        until = time.monotonic() + args.duration if args.duration else None
        while not state.stop.wait(0.2):
            if (output / "stop").exists() or (
                until is not None and time.monotonic() >= until
            ):
                break
            if state.process.poll() is not None:
                raise RuntimeError("Gateway exited; see gateway.log")
            with state.lock:
                advance = state.auto and time.monotonic() >= state.deadline
                if advance:
                    state.cycle_index = (state.cycle_index + 1) % len(CYCLE)
                    next_scene = CYCLE[state.cycle_index]
                    state.deadline = time.monotonic() + SCENARIOS[next_scene][2]
            if advance:
                state.set_scenario(next_scene, automatic=True)
    finally:
        state.stop.set()
        # Close only the process and servers created by this run.
        if state.process and state.process.poll() is None:
            state.process.terminate()
            try:
                state.process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                state.process.kill()
                state.process.wait(timeout=5)
        for thread in threads:
            thread.join(timeout=1)
        if control:
            # shutdown() blocks if serve_forever() never started (seed failure).
            if control_started:
                control.shutdown()
            control.server_close()
        for upstream in upstreams:
            upstream.shutdown()
            upstream.server_close()
        if log:
            log.close()
        for kind, handler in previous_handlers.items():
            signal.signal(kind, handler)
        print("STOPPED " + str(output), flush=True)


if __name__ == "__main__":
    try:
        main()
    except InterruptedError:
        pass
