"""Verify group/model channel intersections through a real local gateway.

Only upstream providers are simulated. Each run uses a new SQLite database,
real setup/login, public configuration APIs, real Keys, relay and billing.
Build the gateway first; run with --binary PATH --output NEW_DIRECTORY.
Use --memory-cache false for the database selection path; --hold keeps the
isolated server available for browser verification until a stop file appears.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import secrets
import socket
import sqlite3
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import URLError


# Reuse the repository's real-process HTTP client; it never logs credentials.
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    'capacity_e2e', Path(__file__).resolve().parents[1] / 'channel-capacity-e2e' / 'run.py')
capacity_e2e = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capacity_e2e)
Client = capacity_e2e.Client


class Upstream(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        marker = body['messages'][-1]['content']
        fail = marker == 'fail-all' or (marker == 'fail-x' and self.server.label == 'X')
        with self.server.lock:
            self.server.calls.append({'path': self.path, 'model': body['model'],
                                      'marker': marker, 'status': 500 if fail else 200})
        payload = {'id': self.server.label, 'object': 'chat.completion',
                   'model': body['model'], 'choices': [{'index': 0,
                   'message': {'role': 'assistant', 'content': self.server.label},
                   'finish_reason': 'stop'}],
                   'usage': {'prompt_tokens': 1, 'completion_tokens': 1, 'total_tokens': 2}}
        if fail:
            payload = {'error': {'message': 'synthetic upstream failure', 'type': 'server_error'}}
        data = json.dumps(payload).encode()
        content_type = 'application/json'
        if body.get('stream') and not fail:
            payload['object'] = 'chat.completion.chunk'
            payload['choices'][0]['delta'] = payload['choices'][0].pop('message')
            data = ('data: ' + json.dumps(payload) + '\n\ndata: [DONE]\n\n').encode()
            content_type = 'text/event-stream'
        self.send_response(500 if fail else 200)
        self.send_header('Content-Type', content_type)
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--memory-cache', choices=['true', 'false'], default='true')
    parser.add_argument('--hold', action='store_true')
    args = parser.parse_args()
    binary, output = args.binary.resolve(), args.output.resolve()
    assert binary.is_file(), binary
    output.mkdir(parents=True, exist_ok=False)
    report = {'mode': 'real gateway / isolated SQLite / simulated upstream HTTP',
              'memory_cache': args.memory_cache, 'tests': []}
    upstreams, process, log = [], None, None
    try:
        for label in ['X', 'Y', 'Z', 'W']:
            server = ThreadingHTTPServer(('127.0.0.1', 0), Upstream)
            server.label, server.calls, server.lock = label, [], threading.Lock()
            threading.Thread(target=server.serve_forever, daemon=True).start()
            upstreams.append(server)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        database = output / 'gateway.db'
        env = os.environ.copy()
        env.update({'PORT': str(port), 'SQL_DSN': '', 'LOG_SQL_DSN': '',
                    'SQLITE_PATH': 'file:' + database.as_posix() + '?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)',
                    'SQL_MAX_OPEN_CONNS': '1', 'REDIS_CONN_STRING': '',
                    'SESSION_SECRET': secrets.token_hex(32), 'NODE_TYPE': 'master',
                    'MEMORY_CACHE_ENABLED': args.memory_cache, 'GIN_MODE': 'release',
                    'CountToken': 'false', 'GLOBAL_API_RATE_LIMIT_ENABLE': 'false',
                    'GLOBAL_WEB_RATE_LIMIT_ENABLE': 'false', 'CRITICAL_RATE_LIMIT_ENABLE': 'false',
                    'SEARCH_RATE_LIMIT_ENABLE': 'false', 'HTTP_PROXY': '', 'HTTPS_PROXY': '',
                    'ALL_PROXY': '', 'NO_PROXY': '127.0.0.1,localhost'})
        log = (output / 'gateway.log').open('w', encoding='utf-8')
        anonymous = Client(f'http://127.0.0.1:{port}')

        def start():
            child = subprocess.Popen([str(binary)], cwd=output, env=env, stdout=log,
                                     stderr=subprocess.STDOUT,
                                     creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
            deadline = time.monotonic() + 60
            while True:
                assert child.poll() is None, 'Gateway exited; see gateway.log'
                try:
                    anonymous.api('GET', '/api/setup')
                    return child
                except (URLError, TimeoutError):
                    assert time.monotonic() < deadline, 'Gateway startup timeout'
                    time.sleep(0.2)

        def mark(name, evidence):
            report['tests'].append({'name': name, 'status': 'passed', 'evidence': evidence})
            print('PASS ' + name, flush=True)

        process = start()
        password = secrets.token_urlsafe(12)
        anonymous.api('POST', '/api/setup', {'username': 'e2eroot', 'password': password,
                                            'confirmPassword': password})
        auth = anonymous.api('POST', '/api/user/login', {'username': 'e2eroot', 'password': password})
        admin = Client(anonymous.base, auth['access_token'])

        def option(key, value):
            admin.api('PUT', '/api/option/', {'key': key,
                      'value': value if isinstance(value, str) else json.dumps(value)})

        models = ['gpt-policy', 'gpt-open', 'gpt-denied', 'gpt-nointersection',
                  'gpt-alias', 'gpt-retry', 'gpt-stream', 'gpt-capacity', 'gpt-auto', 'gpt-monthly']
        for key, value in {'ModelPrice': dict.fromkeys(models, 0.02), 'RetryTimes': '0',
                           'ModelRequestRateLimitEnabled': 'false', 'LogConsumeEnabled': 'true',
                           'performance_setting.monitor_enabled': 'false',
                           'error_setting.hide_error_details': 'true',
                           'GroupRatio': {'default': 1, 'vip': 0.5, 'pool': 9},
                           'UserUsableGroups': {'default': 'default', 'vip': 'vip', 'auto': 'auto'}}.items():
            option(key, value)

        def new_user(name, group='default'):
            admin.api('POST', '/api/user/', {'username': name, 'password': password, 'role': 1, 'group': group})
            session = anonymous.api('POST', '/api/user/login', {'username': name, 'password': password})
            uid = session['user']['id']
            if group != 'default':
                admin.api('PUT', '/api/user/', {'id': uid, 'username': name, 'group': group})
                session = anonymous.api('POST', '/api/user/login', {'username': name, 'password': password})
            assert session['user']['group'] == group, session['user']['group']
            admin.api('POST', '/api/user/manage', {'id': uid, 'action': 'add_quota',
                      'mode': 'override', 'value': 10000000})
            return Client(anonymous.base, session['access_token']), uid

        def token_for(dashboard, name, group='vip', **extra):
            dashboard.api('POST', '/api/token/', {'name': name, 'unlimited_quota': False,
                          'remain_quota': 10000000, 'expired_time': -1, 'group': group, **extra})
            rows = dashboard.api('GET', '/api/token/?page_size=100')['items']
            tid = next(row['id'] for row in rows if row['name'] == name)
            key = dashboard.api('POST', f'/api/token/{tid}/key')['key']
            return Client(anonymous.base, key if key.startswith('sk-') else 'sk-' + key), tid

        dashboard, uid = new_user('e2ealice')
        other_dashboard, other_uid = new_user('e2ebob', 'vip')
        alice, tid = token_for(dashboard, 'vip-key')
        bob, _ = token_for(other_dashboard, 'other-user')
        auto, auto_tid = token_for(dashboard, 'auto-key', 'auto',
                                  auto_groups=['default', 'vip'], cross_group_retry=True)
        limited, _ = token_for(dashboard, 'limited-key', model_limits_enabled=True, model_limits='gpt-policy')
        root, _ = token_for(admin, 'root-vip')
        admin.api('POST', '/api/user/manage', {'id': auth['user']['id'], 'action': 'add_quota',
                  'mode': 'override', 'value': 10000000})

        def channel(name, model_names, upstream, group, priority, **extra):
            payload = {'name': name, 'type': 1, 'key': 'synthetic-upstream-key', 'status': 1,
                       'models': ','.join(model_names), 'group': group, 'priority': priority, 'weight': 0,
                       'base_url': f'http://127.0.0.1:{upstreams[upstream].server_port}', **extra}
            admin.api('POST', '/api/channel/', {'mode': 'single', 'channel': payload})
            rows = admin.api('GET', '/api/channel/?page_size=100')['items']
            cid = next(row['id'] for row in rows if row['name'] == name)
            admin.api('PATCH', f'/api/channel/{cid}/model-routing-overrides',
                      {'overrides': [{'model': model_names[0], 'rpm_override': None, 'tpm_override': None}]})
            return cid

        shared_models = [m for m in models if m != 'gpt-nointersection']
        x = channel('X-shared', shared_models, 0, 'vip,pool', 50,
                    model_mapping=json.dumps({'gpt-alias': 'private-upstream-name'}))
        y = channel('Y-key-only', models, 1, 'vip', 100)
        z = channel('Z-pool-only', models, 2, 'pool', 200)
        w = channel('W-retry-shared', ['gpt-retry'], 3, 'vip,pool', 10)
        channel('default-auto-outside', ['gpt-auto'], 1, 'default', 100)

        def snapshot(user_id=uid, token_id=tid):
            with sqlite3.connect(database, timeout=10) as db:
                user = db.execute('select quota, used_quota, request_count from users where id=?', (user_id,)).fetchone()
                token = db.execute('select remain_quota, used_quota from tokens where id=?', (token_id,)).fetchone()
                logs = db.execute('select count(*), coalesce(sum(quota),0) from logs where type=2 and token_id=?', (token_id,)).fetchone()
                return user + token + logs

        def checked_chat(client, model, expected, token_id=tid, user_id=uid, delta=5000, **params):
            before = snapshot(user_id, token_id)
            calls_before = [len(s.calls) for s in upstreams]
            response = client.chat(model, **params)
            status, _, body = response
            if expected is None:
                assert status >= 400, (status, body)
            else:
                assert status == 200, (status, body)
                if params.get('stream'):
                    assert expected in body and '[DONE]' in body, body
                else:
                    assert body['choices'][0]['message']['content'] == expected, body
            charge = delta if expected is not None else 0
            want = (before[0] - charge, before[1] + charge, before[2] + int(charge > 0),
                    before[3] - charge, before[4] + charge, before[5] + int(charge > 0), before[6] + charge)
            deadline = time.monotonic() + 5
            while snapshot(user_id, token_id) != want:
                assert time.monotonic() < deadline, ('billing mismatch', before, want, snapshot(user_id, token_id), status)
                time.sleep(0.02)
            called = {s.label: s.calls[n:] for s, n in zip(upstreams, calls_before) if len(s.calls) > n}
            if expected is not None:
                with sqlite3.connect(database, timeout=10) as db:
                    row = db.execute('select channel_id, "group", quota from logs where type=2 and token_id=? order by id desc limit 1', (token_id,)).fetchone()
                assert row[1:] == ('vip', delta), row
            return status, called

        policy = {'default': {m: ['pool'] for m in models if m != 'gpt-open'}}
        policy['default']['gpt-denied'] = []
        dynamic_settings = {row['key'].split('.', 1)[1]: json.loads(row['value'])
                            for row in admin.api('GET', '/api/option/')
                            if row['key'].startswith('dynamic_routing_setting.')}
        for dynamic in [False, True]:
            dynamic_settings['enabled'] = dynamic
            admin.api('PUT', '/api/option/dynamic_routing', dynamic_settings)
            prefix = 'dynamic=' + str(dynamic).lower() + ': '
            option('GroupModelChannelGroups', {})
            _, calls = checked_chat(alice, 'gpt-policy', 'Y')
            assert set(calls) == {'Y'}, calls
            mark(prefix + 'unconfigured baseline', 'Key vip chooses its highest priority Y; exact user/Key/log debit 5000')
            option('GroupModelChannelGroups', policy)
            _, calls = checked_chat(alice, 'gpt-policy', 'X')
            assert set(calls) == {'X'}, calls
            mark(prefix + 'underlying channel intersection', 'vip={X,Y}, pool={X,Z}; only X receives HTTP; billing group remains vip')
            checked_chat(alice, 'gpt-open', 'Y')
            mark(prefix + 'unconfigured model unchanged', 'same user and Key still select Y for a model without a rule')
            for model in ['gpt-denied', 'gpt-nointersection']:
                status, calls = checked_chat(alice, model, None)
                assert not calls, calls
                mark(prefix + model, {'http': status, 'upstream_calls': 0, 'charge': 0})
            status, _, listed = alice.request('GET', '/v1/models')
            assert status == 200, listed
            names = {row['id'] for row in listed['data']}
            assert {'gpt-policy', 'gpt-open'} <= names, names
            assert not {'gpt-denied', 'gpt-nointersection'} & names, names
            dashboard_models = set(dashboard.api('GET', '/api/user/models'))
            assert 'gpt-policy' in dashboard_models and 'gpt-denied' not in dashboard_models, dashboard_models
            mark(prefix + 'model discovery', 'relay and dashboard hide denied models and retain usable models')
            _, calls = checked_chat(alice, 'gpt-alias', 'X')
            assert calls['X'][0]['model'] == 'private-upstream-name', calls
            mark(prefix + 'public model policy precedes alias mapping', 'gpt-alias policy applied; upstream receives private-upstream-name')
            checked_chat(auto, 'gpt-auto', 'X', token_id=auto_tid)
            mark(prefix + 'Auto group and pricing', 'default has no allowed channel; vip selects X and charges vip ratio')
            checked_chat(alice, 'gpt-stream', 'X', stream=True)
            mark(prefix + 'stream relay', 'SSE completes with DONE; one exact debit and consume log')
            option('RetryTimes', '2')
            _, calls = checked_chat(alice, 'gpt-retry', 'W', messages=[{'role': 'user', 'content': 'fail-x'}])
            assert set(calls) == {'X', 'W'}, calls
            mark(prefix + 'upstream retry stays in intersection', 'X fails then W succeeds; Y/Z receive no request; one charge')
            _, calls = checked_chat(alice, 'gpt-retry', None, messages=[{'role': 'user', 'content': 'fail-all'}])
            assert calls and set(calls) <= {'X', 'W'}, calls
            mark(prefix + 'failed retry refunds reservation', 'only allowed upstreams called; user/Key restored; no consume log')
            option('RetryTimes', '0')
            expanded = {'default': {'gpt-policy': ['pool', 'vip']}}
            option('GroupModelChannelGroups', expanded)
            checked_chat(alice, 'gpt-policy', 'Y')
            mark(prefix + 'multiple pools union', 'pool plus vip admits Y immediately')
            option('GroupModelChannelGroups', {})
            checked_chat(alice, 'gpt-policy', 'Y')
            mark(prefix + 'remove policy restores routing', 'clearing the saved rule restores original selection and billing')

        option('GroupModelChannelGroups', policy)
        original_special = next(row['value'] for row in admin.api('GET', '/api/option/')
                                if row['key'] == 'GroupGroupRatio')
        option('GroupGroupRatio', {'default': {'vip': 0.25}})
        checked_chat(alice, 'gpt-policy', 'X', delta=2500)
        option('GroupGroupRatio', original_special)
        checked_chat(alice, 'gpt-policy', 'X')
        mark('special user-group pricing preserved', 'pool ratio 9 ignored; special vip ratio 0.25 charges 2500; restored base ratio charges 5000')

        option('group_ratio_setting.model_tiered_ratios', {'vip': {'gpt-monthly': {
            'enabled': True, 'effective_from': 0, 'effective_until': None, 'timezone': 'UTC',
            'tiers': [{'min_monthly_original_quota': 0, 'ratio': 0.4},
                      {'min_monthly_original_quota': 15000, 'ratio': 0.2}]}}})
        checked_chat(alice, 'gpt-monthly', 'X', delta=4000)
        checked_chat(alice, 'gpt-monthly', 'X', delta=3000)
        checked_chat(alice, 'gpt-monthly', None, messages=[{'role': 'user', 'content': 'fail-all'}])
        with sqlite3.connect(database, timeout=10) as db:
            usage = db.execute('select original_quota, charged_quota from user_group_model_monthly_usages where user_id=? and using_group=? and origin_model=?',
                               (uid, 'vip', 'gpt-monthly')).fetchone()
        assert usage == (20000, 7000), usage
        option('group_ratio_setting.model_tiered_ratios', {})
        mark('monthly tiered pricing and refund preserved', 'X selected; crossing 15000 original quota charges 4000 then 3000; failed relay refunds and does not advance monthly usage')

        # Dynamic routing deliberately bypasses affinity; exercise its static path.
        dynamic_settings['enabled'] = False
        admin.api('PUT', '/api/option/dynamic_routing', dynamic_settings)
        option('channel_affinity_setting.rules', [{'name': 'pool-e2e', 'model_regex': ['^gpt-policy$'],
               'path_regex': ['^/v1/chat/completions$'], 'user_agent_include': [],
               'value_regex': '', 'ttl_seconds': 60, 'param_override_template': {}, 'skip_retry_on_failure': False,
               'key_sources': [{'type': 'request_header', 'key': 'X-Session'}],
               'include_rule_name': True, 'include_model_name': True, 'include_using_group': True}])
        alice.opener.addheaders = [('X-Session', 'pool-e2e-session')]
        option('GroupModelChannelGroups', {})
        checked_chat(alice, 'gpt-policy', 'Y', prompt_cache_key='pool-e2e-session')
        deadline = time.monotonic() + 5
        while admin.api('GET', '/api/option/channel_affinity_cache')['by_rule_name'].get('pool-e2e', 0) == 0:
            assert time.monotonic() < deadline, 'Affinity entry was not recorded after the successful relay'
            time.sleep(0.02)
        checked_chat(alice, 'gpt-policy', 'Y', prompt_cache_key='pool-e2e-session')
        with sqlite3.connect(database, timeout=10) as db:
            other = db.execute('select other from logs where type=2 and token_id=? order by id desc limit 1', (tid,)).fetchone()[0]
        assert json.loads(other)['admin_info']['channel_affinity']['channel_id'] == y, other
        option('GroupModelChannelGroups', policy)
        _, calls = checked_chat(alice, 'gpt-policy', 'X', prompt_cache_key='pool-e2e-session')
        assert set(calls) == {'X'}, calls
        alice.opener.addheaders = []
        mark('existing affinity cache cannot bypass a new policy', 'consume log proves cached Y was used; policy update replaces it with X for the same session')

        status, _, body = bob.chat('gpt-policy')
        assert status == 200 and body['choices'][0]['message']['content'] == 'Y', body
        mark('other user group unaffected', 'vip user is outside default policy; routes to Y')
        before_calls = sum(len(s.calls) for s in upstreams)
        status, _, _ = limited.chat('gpt-open')
        assert status == 403 and before_calls == sum(len(s.calls) for s in upstreams), status
        mark('Key model allowlist preserved', 'Key rejects model before upstream despite missing group rule')

        # Pinning is an admin-only public Key suffix; it must not escape either set.
        for cid, expected in [(x, 'X'), (y, None), (z, None)]:
            before_calls = sum(len(s.calls) for s in upstreams)
            status, _, body = Client(anonymous.base, root.credential + '-' + str(cid)).chat('gpt-policy')
            if expected:
                assert status == 200 and body['choices'][0]['message']['content'] == expected, body
            else:
                assert status == 403 and before_calls == sum(len(s.calls) for s in upstreams), (status, body)
        mark('admin pinned channels enforce both memberships', 'pin X=200; Key-only Y and pool-only Z=403, no upstream')

        admin.api('PATCH', f'/api/channel/{x}/model-routing-overrides',
                  {'overrides': [{'model': 'gpt-capacity', 'rpm_override': 1, 'tpm_override': None}]})
        checked_chat(alice, 'gpt-capacity', 'X')
        status, calls = checked_chat(alice, 'gpt-capacity', None)
        assert status == 429 and not calls, (status, calls)
        mark('capacity exhaustion cannot spill outside intersection', 'X RPM exhausted; Y/Z excluded; 429 without charge')

        status, _, rejected = admin.request('PUT', '/api/option/',
            {'key': 'GroupModelChannelGroups', 'value': '{"default":{"gpt-policy":null}}'})
        assert status >= 400 or rejected.get('success') is False, rejected
        checked_chat(alice, 'gpt-policy', 'X')
        mark('invalid configuration is atomic', 'null rejected and previous policy still chooses X')

        process.terminate()
        process.wait(timeout=10)
        process = start()
        options = admin.api('GET', '/api/option/')
        saved = next(row['value'] for row in options if row['key'] == 'GroupModelChannelGroups')
        assert json.loads(saved) == policy, saved
        checked_chat(alice, 'gpt-policy', 'X')
        mark('process restart persists and reloads policy', 'same SQLite, Key and admin session; X still selected and exact billing retained')
        report['status'] = 'passed'
        report['upstream_requests'] = {s.label: s.calls for s in upstreams}
        (output / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
        if args.hold:
            (output / 'ui-session.json').write_text(json.dumps({'base_url': anonymous.base,
                'username': 'e2eroot', 'password': password, 'pid': process.pid,
                'alice_key': alice.credential, 'admin_token': admin.credential}), encoding='utf-8')
            print('READY ' + anonymous.base, flush=True)
            while not (output / 'stop').exists():
                assert process.poll() is None, 'Gateway exited while awaiting browser verification'
                time.sleep(0.5)
    except Exception as error:
        report['status'], report['error'] = 'failed', str(error)
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
        report['upstream_requests'] = {s.label: s.calls for s in upstreams}
        (output / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
        print('Report: ' + str(output / 'report.json'), flush=True)


if __name__ == '__main__':
    main()
