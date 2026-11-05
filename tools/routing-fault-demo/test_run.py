"""HTTP fault/control contracts and run isolation; no gateway or paid upstream."""

from http.client import IncompleteRead
from http.server import ThreadingHTTPServer
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch
from urllib.error import HTTPError
from urllib.request import Request, ProxyHandler, build_opener

spec = importlib.util.spec_from_file_location(
    "routing_fault_demo", Path(__file__).with_name("run.py")
)
demo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(demo)


class DemoTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.state = demo.Demo(Path(self.temp.name))
        self.opener = build_opener(ProxyHandler({}))
        self.servers = []
        self.addCleanup(self.close_servers)

    def close_servers(self):
        self.state.stop.set()
        for server in self.servers:
            server.shutdown()
            server.server_close()

    def server(self, handler, index=0):
        server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
        server.demo, server.index = self.state, index
        self.servers.append(server)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        return f"http://127.0.0.1:{server.server_port}"

    def request(self, url, body=None, **headers):
        request = Request(
            url,
            data=None if body is None else json.dumps(body).encode(),
            headers={"Content-Type": "application/json", **headers},
        )
        try:
            return self.opener.open(request, timeout=5)
        except HTTPError as error:
            return error

    def test_upstream_errors_and_dual_failure_leave_fallback_healthy(self):
        urls = [self.server(demo.Upstream, index) for index in range(3)]
        for scenario, statuses in [
            ("http429", [429, 200, 200]),
            ("http502", [502, 200, 200]),
            ("dual_failure", [502, 502, 200]),
            ("recovery", [200, 200, 200]),
        ]:
            self.state.set_scenario(scenario)
            for url, status in zip(urls, statuses):
                with self.subTest(scenario=scenario, url=url), self.request(
                    url, {}
                ) as response:
                    self.assertEqual(response.status, status)
                    raw = response.read().decode()
                    if status == 200:
                        self.assertIn("data: [DONE]", raw)
                    else:
                        self.assertIn(
                            "Synthetic local upstream",
                            json.loads(raw)["error"]["message"],
                        )

    def test_disconnect_sends_partial_stream_without_done_marker(self):
        self.state.set_scenario("disconnect")
        with self.request(self.server(demo.Upstream), {}) as response:
            with self.assertRaises(IncompleteRead) as caught:
                response.read()
        self.assertIn(b"data: ", caught.exception.partial)
        self.assertNotIn(b"[DONE]", caught.exception.partial)

    def test_local_controls_switch_real_scenario_and_disable_auto(self):
        self.state.control_url = self.server(demo.Control)
        self.state.set_auto(True)
        with self.request(
            self.state.control_url + "/api/scenario",
            {"scenario": "slow_ttft"},
            Origin=self.state.control_url,
        ) as response:
            self.assertEqual(response.status, 200)
        self.assertEqual(self.state.behavior(0), "slow_ttft")
        self.assertEqual(self.state.behavior(1), "normal")
        self.assertFalse(self.state.auto)

    def test_control_rejects_foreign_origin_host_and_invalid_commands(self):
        self.state.control_url = self.server(demo.Control)
        cases = [
            ({"scenario": "http502"}, {"Origin": "https://foreign.example"}, 403),
            ({"scenario": "http502"}, {"Host": "foreign.example"}, 403),
            ({"scenario": "unknown"}, {}, 400),
            ({"scenario": []}, {}, 400),
            ({"auto": 1}, {}, 400),
            ([], {}, 400),
        ]
        for body, headers, status in cases:
            with self.subTest(body=body, headers=headers), self.request(
                self.state.control_url + "/api/scenario", body, **headers
            ) as response:
                self.assertEqual(response.status, status)
            self.assertEqual(self.state.scenario, "normal")
        with self.request(self.state.control_url + "/credentials.json") as response:
            self.assertEqual(response.status, 404)

    def test_child_environment_drops_inherited_service_configuration(self):
        with patch.dict(
            demo.os.environ,
            {
                "SQL_DSN": "remote-db-secret",
                "LOG_SQL_DSN": "remote-logs",
                "REDIS_CONN_STRING": "remote-cache",
                "ENABLE_PPROF": "true",
                "PYROSCOPE_SERVER_ADDRESS": "https://foreign.example",
                "HTTPS_PROXY": "remote-proxy",
            },
        ):
            env = demo.gateway_environment(Path(self.temp.name), 45678)
        self.assertEqual(env["SQL_DSN"], "")
        self.assertEqual(env["LOG_SQL_DSN"], "")
        self.assertEqual(env["REDIS_CONN_STRING"], "")
        self.assertEqual(env["HTTPS_PROXY"], "")
        self.assertNotIn("ENABLE_PPROF", env)
        self.assertNotIn("PYROSCOPE_SERVER_ADDRESS", env)
        self.assertIn(Path(self.temp.name).as_posix(), env["SQLITE_PATH"])
        self.assertEqual(env["PORT"], "45678")

    def test_existing_output_is_rejected_before_process_start(self):
        marker = Path(self.temp.name) / "keep.txt"
        marker.write_text("keep", encoding="utf-8")
        with patch.object(demo.subprocess, "Popen") as spawn:
            with self.assertRaises(FileExistsError):
                demo.main(["--binary", sys.executable, "--output", self.temp.name])
        spawn.assert_not_called()
        self.assertEqual(marker.read_text(encoding="utf-8"), "keep")


if __name__ == "__main__":
    unittest.main()
