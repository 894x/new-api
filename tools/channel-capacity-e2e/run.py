"""Exercise a real gateway process against local simulated OpenAI upstreams.

Build web/dist and the gateway first, then run:
  python tools/channel-capacity-e2e/run.py --binary <gateway> --output <new directory>
Only the upstream provider is mocked. Setup, authentication, configuration,
SQLite migration/cache, routing, admission, and response handling are real.
"""
import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError, URLError
from urllib.request import Request, build_opener, ProxyHandler


class Upstream(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        # Record only synthetic request data; never retain authorization headers.
        with self.server.lock:
            self.server.calls.append({'path': self.path, 'body': body,
                                      'organization': self.headers.get('OpenAI-Organization')})
        payload = {'id': self.server.label, 'object': 'chat.completion',
                   'model': body['model'], 'choices': [{'index': 0,
                   'message': {'role': 'assistant', 'content': self.server.label},
                   'finish_reason': 'stop'}],
                   'usage': {'prompt_tokens': 1, 'completion_tokens': 1, 'total_tokens': 2}}
        if body.get('stream'):
            chunk = {'id': self.server.label, 'object': 'chat.completion.chunk',
                     'model': body['model'], 'choices': [{'index': 0,
                     'delta': {'role': 'assistant', 'content': self.server.label},
                     'finish_reason': 'stop'}], 'usage': payload['usage']}
            data = ('data: ' + json.dumps(chunk) + '\n\ndata: [DONE]\n\n').encode()
            content_type = 'text/event-stream'
        else:
            data = json.dumps(payload).encode()
            content_type = 'application/json'
        self.send_response(200)
        self.send_header('Content-Type', content_type)
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)


class Client:
    def __init__(self, base, credential=None):
        self.base, self.credential = base, credential
        self.opener = build_opener(ProxyHandler({}))

    def request(self, method, path, body=None):
        headers = {'Content-Type': 'application/json'}
        if self.credential:
            headers['Authorization'] = 'Bearer ' + self.credential
        request = Request(self.base + path, data=None if body is None else json.dumps(body).encode(),
                          headers=headers, method=method)
        try:
            response = self.opener.open(request, timeout=20)
        except HTTPError as error:
            response = error
        with response:
            raw = response.read().decode()
            try:
                data = json.loads(raw)
            except ValueError:
                data = raw
            return response.status, dict(response.headers), data

    def api(self, method, path, body=None):
        status, _, data = self.request(method, path, body)
        assert status == 200 and data.get('success'), (method, path, status, data)
        return data.get('data')

    def chat(self, model, **params):
        body = {'model': model, 'messages': [{'role': 'user', 'content': 'hi'}], 'max_tokens': 1}
        body.update(params)
        if body.get('max_tokens') is None:
            del body['max_tokens']
        return self.request('POST', '/v1/chat/completions', body)


def capacity_denied(response):
    status, headers, body = response
    assert status == 429, (status, body)
    assert body['error']['code'] == 'channel_model_capacity_exhausted', body
    assert 1 <= int(headers['Retry-After']) <= 60, headers
    return int(headers['Retry-After'])


def success(response, label=None):
    status, _, body = response
    assert status == 200, (status, body)
    if label:
        assert body['choices'][0]['message']['content'] == label, body


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--memory-cache', choices=['true', 'false'], default='true')
    args = parser.parse_args()
    binary, output = args.binary.resolve(), args.output.resolve()
    assert binary.is_file(), binary
    output.mkdir(parents=True, exist_ok=False)
    results, upstreams = [], []
    process = None
    log = None
    report = {'mode': 'real process / isolated SQLite / simulated upstream HTTP',
              'memory_cache': args.memory_cache, 'tests': results}
    try:
        for label in ['upstream-high', 'upstream-low']:
            server = ThreadingHTTPServer(('127.0.0.1', 0), Upstream)
            server.label, server.calls, server.lock = label, [], threading.Lock()
            threading.Thread(target=server.serve_forever, daemon=True).start()
            upstreams.append(server)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        env = os.environ.copy()
        env.update({'PORT': str(port), 'SQL_DSN': '', 'LOG_SQL_DSN': '',
                    'SQLITE_PATH': 'file:' + (output / 'gateway.db').as_posix() + '?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)',
                    'SQL_MAX_OPEN_CONNS': '1', 'REDIS_CONN_STRING': '',
                    'SESSION_SECRET': secrets.token_hex(32), 'NODE_TYPE': 'master',
                    'MEMORY_CACHE_ENABLED': args.memory_cache, 'GIN_MODE': 'release',
                    'CountToken': 'false', 'GLOBAL_API_RATE_LIMIT_ENABLE': 'false',
                    'GLOBAL_WEB_RATE_LIMIT_ENABLE': 'false', 'CRITICAL_RATE_LIMIT_ENABLE': 'false',
                    'SEARCH_RATE_LIMIT_ENABLE': 'false', 'HTTP_PROXY': '', 'HTTPS_PROXY': '',
                    'ALL_PROXY': '', 'NO_PROXY': '127.0.0.1,localhost'})
        log = (output / 'gateway.log').open('w', encoding='utf-8')
        process = subprocess.Popen([str(binary)], cwd=output, env=env, stdout=log,
                                   stderr=subprocess.STDOUT,
                                   creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        anonymous = Client(f'http://127.0.0.1:{port}')
        deadline = time.monotonic() + 60
        while True:
            assert process.poll() is None, 'Gateway exited; see gateway.log'
            try:
                anonymous.api('GET', '/api/setup')
                break
            except (URLError, TimeoutError):
                assert time.monotonic() < deadline, 'Gateway startup timeout'
                time.sleep(0.2)
        password = secrets.token_urlsafe(12)
        anonymous.api('POST', '/api/setup', {'username': 'e2eroot', 'password': password,
                                            'confirmPassword': password})
        auth = anonymous.api('POST', '/api/user/login', {'username': 'e2eroot', 'password': password})
        admin = Client(anonymous.base, auth['access_token'])
        models = ['rpm-shared', 'alias-a', 'alias-b', 'forced', 'tpm-atomic', 'default-output',
                  'multi-choice', 'final-policy', 'system-prompt', 'concurrent', 'streaming',
                  'user-tpm', 'reset-window', 'override-config', 'auto-group']
        for key, value in {'ModelRatio': json.dumps(dict.fromkeys(models, 0)), 'RetryTimes': '0',
                           'ModelRequestRateLimitEnabled': 'false',
                           'performance_setting.monitor_enabled': 'false',
                           'error_setting.hide_error_details': 'true',
                           'GroupRatio': '{"default":1,"vip":1}',
                           'UserUsableGroups': '{"default":"default","vip":"vip","auto":"auto"}'}.items():
            admin.api('PUT', '/api/option/', {'key': key, 'value': value})

        def token_for(dashboard, name, group='default', **extra):
            dashboard.api('POST', '/api/token/', {'name': name, 'unlimited_quota': True,
                          'expired_time': -1, 'group': group, **extra})
            tokens = dashboard.api('GET', '/api/token/?page_size=100')['items']
            token_id = next(row['id'] for row in tokens if row['name'] == name)
            key = dashboard.api('POST', f'/api/token/{token_id}/key')['key']
            return Client(anonymous.base, key if key.startswith('sk-') else 'sk-' + key)

        users = []
        dashboards = []
        for username in ['e2ealice', 'e2ebob']:
            admin.api('POST', '/api/user/', {'username': username, 'password': password, 'role': 1})
            session = anonymous.api('POST', '/api/user/login', {'username': username, 'password': password})
            admin.api('POST', '/api/user/manage', {'id': session['user']['id'],
                      'action': 'add_quota', 'mode': 'override', 'value': 1000000})
            dashboard = Client(anonymous.base, session['access_token'])
            dashboards.append(dashboard)
            users.append(token_for(dashboard, 'default'))
        alice_second = token_for(dashboards[0], 'second')
        bob_vip = token_for(dashboards[1], 'vip', 'vip')
        root_token = token_for(admin, 'root')
        auto_token = token_for(dashboards[0], 'auto', 'auto', auto_groups=['default', 'vip'], cross_group_retry=True)

        def channel(name, model_names, upstream=0, rpm=0, tpm=0, priority=100, **extra):
            names = [model_names] if isinstance(model_names, str) else model_names
            payload = {'name': name, 'type': 1, 'key': 'synthetic-upstream-key', 'status': 1,
                       'models': ','.join(names), 'group': 'default,vip', 'priority': priority,
                       'rpm': rpm, 'tpm': tpm, 'weight': 0,
                       'base_url': f'http://127.0.0.1:{upstreams[upstream].server_port}',
                       'model_mapping': json.dumps(dict.fromkeys(names, 'mapped-' + name))}
            payload.update(extra)
            admin.api('POST', '/api/channel/', {'mode': 'single', 'channel': payload})
            rows = admin.api('GET', '/api/channel/?page_size=100')['items']
            channel_id = next(row['id'] for row in rows if row['name'] == name)
            # Existing channel creation relies on periodic cache sync; this public
            # override endpoint explicitly refreshes the cache without a fixed wait.
            admin.api('PATCH', f'/api/channel/{channel_id}/model-routing-overrides',
                      {'overrides': [{'model': names[0], 'rpm_override': None, 'tpm_override': None}]})
            return channel_id

        def mark(name, evidence):
            results.append({'name': name, 'status': 'passed', 'evidence': evidence})
            print('PASS ' + name, flush=True)

        # Run the short capacity sequences away from a fixed-window boundary.
        if time.time() % 60 > 45:
            time.sleep(60 - time.time() % 60 + 0.1)
        channel('shared-high', 'rpm-shared', rpm=2, openai_organization='org-high')
        channel('shared-low', 'rpm-shared', upstream=1, rpm=1, priority=10)
        success(users[0].chat('rpm-shared'), 'upstream-high')
        success(alice_second.chat('rpm-shared'), 'upstream-high')
        success(bob_vip.chat('rpm-shared'), 'upstream-low')
        capacity_denied(users[1].chat('rpm-shared'))
        assert upstreams[0].calls[-1]['organization'] == 'org-high'
        assert upstreams[1].calls[-1]['organization'] is None
        assert upstreams[1].calls[-1]['body']['model'] == 'mapped-shared-low'
        mark('shared RPM across users, keys and groups; priority spillover', '200 high, 200 high, 200 low, 429; RetryTimes=0; organization cleared')

        channel('aliases', ['alias-a', 'alias-b'], rpm=1)
        success(users[0].chat('alias-a'))
        success(users[0].chat('alias-b'))
        capacity_denied(users[0].chat('alias-a'))
        mark('public aliases have independent capacity', 'same upstream mapped model; A 200, B 200, A 429')

        forced_id = channel('forced-high', 'forced', rpm=1)
        channel('forced-low', 'forced', upstream=1, rpm=1, priority=1)
        forced = Client(anonymous.base, root_token.credential + '-' + str(forced_id))
        success(forced.chat('forced'), 'upstream-high')
        capacity_denied(forced.chat('forced'))
        success(users[0].chat('forced'), 'upstream-low')
        mark('forced channel cannot spill over', 'forced 200 then 429; automatic request still reaches low')

        channel('tpm', 'tpm-atomic', rpm=1, tpm=100)
        capacity_denied(users[0].chat('tpm-atomic', max_tokens=1000))
        success(users[0].chat('tpm-atomic', max_tokens=1))
        capacity_denied(users[0].chat('tpm-atomic', max_tokens=1))
        mark('TPM rejection does not consume RPM', 'oversized 429, small 200, RPM exhausted 429')

        channel('output-default', 'default-output', tpm=100)
        capacity_denied(users[0].chat('default-output', max_tokens=None))
        success(users[0].chat('default-output', max_tokens=0))
        mark('default output reservation and explicit zero', 'omitted output budget 429; explicit zero 200')

        channel('choices', 'multi-choice', tpm=100)
        capacity_denied(users[0].chat('multi-choice', max_tokens=60, n=2))
        success(users[0].chat('multi-choice', max_tokens=60, n=1))
        mark('multiple completion reservation', 'two choices 429; one choice 200')

        before = sum(len(s.calls) for s in upstreams)
        channel('policy', 'final-policy', tpm=100, param_override='{"max_tokens":1000}')
        capacity_denied(users[0].chat('final-policy', max_tokens=1))
        channel('system', 'system-prompt', tpm=100,
                setting=json.dumps({'system_prompt': 'capacity ' * 300}))
        capacity_denied(users[0].chat('system-prompt', max_tokens=1))
        assert sum(len(s.calls) for s in upstreams) == before
        mark('final transformed payload admission', 'parameter override and injected system prompt rejected before upstream dispatch')

        config_id = channel('config', 'override-config', rpm=1, tpm=100)
        path = f'/api/channel/{config_id}/model-routing-overrides'
        rows = admin.api('GET', path)
        assert rows[0]['effective_rpm'] == 1 and rows[0]['effective_tpm'] == 100
        rows = admin.api('PATCH', path, {'overrides': [{'model': 'override-config', 'rpm_override': 0, 'tpm_override': 0}]})
        assert rows[0]['effective_rpm'] == rows[0]['effective_tpm'] == 0
        rows = admin.api('PATCH', path, {'overrides': [{'model': 'override-config', 'priority_override': 42}]})
        assert rows[0]['effective_rpm'] == rows[0]['effective_tpm'] == 0
        success(users[0].chat('override-config', max_tokens=1000))
        success(users[0].chat('override-config', max_tokens=1000))
        _, _, invalid = admin.request('PATCH', path, {'overrides': [{'model': 'override-config', 'rpm_override': -1}]})
        assert not invalid['success']
        assert admin.api('GET', path)[0]['effective_rpm'] == 0
        rows = admin.api('PATCH', path, {'overrides': [{'model': 'override-config', 'rpm_override': None, 'tpm_override': None}]})
        assert rows[0]['effective_rpm'] == 1 and rows[0]['effective_tpm'] == 100
        capacity_denied(users[0].chat('override-config', max_tokens=1000))
        mark('configuration API inheritance, explicit zero, legacy patch and validation', 'persisted round trip; invalid patch atomic; null restores channel defaults')

        channel('parallel', 'concurrent', rpm=2)
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            responses = list(pool.map(lambda _: users[0].chat('concurrent'), range(4)))
        assert sorted(r[0] for r in responses) == [200, 200, 429, 429]
        for response in responses:
            if response[0] == 429:
                capacity_denied(response)
        mark('concurrent admission is atomic', 'four concurrent requests admit exactly two')

        channel('stream-high', 'streaming', rpm=1)
        channel('stream-low', 'streaming', upstream=1, rpm=1, priority=1)
        success(users[0].chat('streaming'))
        streamed = users[0].chat('streaming', stream=True)
        success(streamed)
        assert 'upstream-low' in streamed[2] and '[DONE]' in streamed[2]
        capacity_denied(users[0].chat('streaming', stream=True))
        mark('streaming spillover before response headers', 'normal 200 high, SSE 200 low, exhausted 429 JSON')

        channel('user-limit', 'user-tpm', rpm=1)
        admin.api('PUT', '/api/option/', {'key': 'ModelRequestRateLimitTPM', 'value': '100'})
        admin.api('PUT', '/api/option/', {'key': 'ModelRequestRateLimitEnabled', 'value': 'true'})
        oversized = users[0].chat('user-tpm', messages=[{'role': 'user', 'content': 'token ' * 300}])
        assert oversized[0] == 429, oversized
        success(users[0].chat('user-tpm'))
        admin.api('PUT', '/api/option/', {'key': 'ModelRequestRateLimitEnabled', 'value': 'false'})
        mark('existing user TPM coexists with upstream capacity', 'user TPM rejects large prompt before channel RPM reservation; small request succeeds')

        channel('auto-default-high', 'auto-group', rpm=1, group='default')
        channel('auto-default-low', 'auto-group', rpm=1, priority=10, group='default')
        channel('auto-vip', 'auto-group', upstream=1, rpm=1, group='vip')
        success(auto_token.chat('auto-group'), 'upstream-high')
        success(auto_token.chat('auto-group'), 'upstream-high')
        success(auto_token.chat('auto-group'), 'upstream-low')
        capacity_denied(auto_token.chat('auto-group'))
        mark('automatic group cursor survives capacity spillover', 'two default priorities before vip, then 429; cross-group retry on, RetryTimes=0')

        channel('reset', 'reset-window', rpm=1)
        success(users[0].chat('reset-window'))
        delay = capacity_denied(users[0].chat('reset-window'))
        print(f'Waiting {delay}s for the real capacity window reset', flush=True)
        # Real E2E clock boundary, using the server response rather than an arbitrary test delay.
        time.sleep(delay + 0.1)
        success(users[0].chat('reset-window'))
        mark('Retry-After recovers at the real minute boundary', '429 followed by 200 after the advertised interval')
        report['status'] = 'passed'
    except Exception as error:
        report['status'] = 'failed'
        report['error'] = str(error)
        raise
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)
        if log is not None:
            log.close()
        for server in upstreams:
            server.shutdown()
            server.server_close()
        report['upstream_requests'] = {server.label: server.calls for server in upstreams}
        (output / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
        print(f'Report: {output / "report.json"}', flush=True)


if __name__ == '__main__':
    main()
