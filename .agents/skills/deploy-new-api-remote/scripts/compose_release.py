import argparse
import copy
import datetime
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import time
from urllib.parse import urlparse, unquote, parse_qs
from decimal import Decimal

# Config and credentials are discovered at runtime; this module does not store either.
SHA = IMAGE = ''
ROOT = BACKUP_ROOT = Path('.')
TARGETS = {}
PROJECT_DIRS = {}
CONFIG = {}

def container_name(project, service='new-api'):
    ids = run(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project='+project,
               '--filter', 'label=com.docker.compose.service='+service]).decode().split()
    assert len(ids) == 1, f'Expected exactly one {project}/{service} container'
    c = inspect(ids[0])
    assert c['State']['Status'] == 'running', f'{project}/{service} is not running'
    return c['Name'].lstrip('/')

def file_hash(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def initialize(config_path):
    global SHA, IMAGE, ROOT, BACKUP_ROOT, TARGETS, PROJECT_DIRS, CONFIG
    ROOT = config_path.resolve().parent
    CONFIG = json.loads(config_path.read_text(encoding='utf-8'))
    SHA, IMAGE = CONFIG['commit'], CONFIG['image']
    assert re.fullmatch(r'[0-9a-f]{40}', SHA), 'A full commit SHA is required'
    assert re.fullmatch(r'[0-9a-f]{64}', CONFIG['sha256']), 'A binary SHA-256 is required'
    assert re.fullmatch(r'sha256:[0-9a-f]{64}', CONFIG['image_id']), 'An immutable image ID is required'
    assert re.fullmatch(r'static/js/[A-Za-z0-9_.-]+\.js', CONFIG['frontend_asset']), 'Invalid frontend asset'
    assert IMAGE and not IMAGE.endswith(':latest'), 'Use a unique release image tag'
    BACKUP_ROOT = Path(CONFIG['backup_root'])
    assert BACKUP_ROOT.is_absolute(), 'Backup root must be absolute'
    BACKUP_ROOT = BACKUP_ROOT.resolve()
    TARGETS = {}
    for target in CONFIG['targets']:
        project, host, port = target['project'], target['hostname'], target['port']
        assert re.fullmatch(r'[a-z0-9][a-z0-9_-]{0,62}', project), 'Invalid Compose project'
        assert re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]*', host), 'Invalid hostname'
        assert isinstance(port, int) and not isinstance(port, bool) and 1 <= port <= 65535, 'Invalid port'
        assert project not in TARGETS, 'Duplicate target project'
        TARGETS[project] = (host, port)
    assert TARGETS, 'At least one target is required'
    for target in CONFIG['targets']:
        assert set(target.get('requires', [])) <= set(TARGETS) - {target['project']}, 'Invalid prerequisites'
    PROJECT_DIRS = {}
    for project in TARGETS:
        c = inspect(container_name(project))
        directory = Path(c['Config']['Labels']['com.docker.compose.project.working_dir'])
        assert directory.is_absolute() and directory.resolve() == directory and directory.is_dir(), 'Unexpected Compose directory'
        assert not BACKUP_ROOT.is_relative_to(directory), 'Backups must be outside the instance checkout'
        assert not ROOT.is_relative_to(directory), 'Release artifacts must be outside the instance checkout'
        PROJECT_DIRS[project] = directory


def run(args, **kwargs):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
    if result.returncode:
        raise RuntimeError(f'Command failed: {args[0]} {args[1] if len(args)>1 else ""}; exit={result.returncode}')
    return result.stdout

def inspect(name):
    return json.loads(run(['docker', 'inspect', name]))[0]

def identity(c):
    return {'id': c['Id'], 'image_id': c['Image'], 'image': c['Config']['Image'],
            'restarts': c['RestartCount'], 'started': c['State']['StartedAt'],
            'status': c['State']['Status'], 'health': c['State'].get('Health', {}).get('Status'),
            'revision': (c['Config'].get('Labels') or {}).get('org.opencontainers.image.revision')}

def snapshot():
    ids = run(['docker', 'ps', '-aq']).decode().split()
    return {c['Name'].lstrip('/'): identity(c) for c in json.loads(run(['docker', 'inspect', *ids]))}

def protected_check(expected, excluded=()):
    current = snapshot()
    assert set(current) == set(expected), 'Container inventory changed'
    for name, old in expected.items():
        if name not in excluded:
            assert current[name] == old, f'Protected container changed: {name}'
    return current

def compose(project, files):
    args = ['docker', 'compose', '--project-directory', str(PROJECT_DIRS[project]), '-p', project]
    for file in files:
        args += ['-f', str(file)]
    return args

def counts(project):
    sql = "SELECT json_build_object('users',(SELECT count(*) FROM users),'channels',(SELECT count(*) FROM channels),'tokens',(SELECT count(*) FROM tokens),'options',(SELECT count(*) FROM options));"
    command = ['docker', 'exec', '-i', container_name(project, 'postgres'), 'sh', '-c', 'exec psql -X -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"']
    return json.loads(run(command, input=sql.encode()))

def nginx_directives(source):
    lexer = shlex.shlex(source, posix=True, punctuation_chars='{};')
    lexer.whitespace_split = True
    roots, stack, header = [], [], []
    current = roots
    for token in lexer:
        tokens = list(token) if token and set(token) <= set('{};') else [token]
        for item in tokens:
            if item == '{':
                assert header, 'Unsupported Nginx block syntax'
                children = []
                current.append((header, children))
                stack.append(current)
                current, header = children, []
            elif item == '}':
                assert stack and not header, 'Unsupported Nginx closing syntax'
                current = stack.pop()
            elif item == ';':
                assert header, 'Unsupported Nginx directive syntax'
                current.append((header, None))
                header = []
            else:
                header.append(item)
    assert not stack and not header, 'Incomplete Nginx configuration'
    return roots

def nginx_check():
    proc = subprocess.run(['sudo', '-n', 'nginx', '-T'], capture_output=True)
    assert proc.returncode == 0, 'Nginx validation failed'
    config = proc.stdout.decode()
    directives = nginx_directives(config)
    servers, pending = [], list(directives)
    while pending:
        header, children = pending.pop()
        if header == ['server']:
            servers.append(children)
        if children:
            pending.extend(children)
    file_parts = re.split(r'^# configuration file (.+):\s*$', config, flags=re.M)
    included_files = dict(zip(file_parts[1::2], file_parts[2::2]))
    server_fields = {'server_name', 'listen', 'location', 'include', 'client_max_body_size', 'access_log', 'error_log'}
    proxy_fields = {'proxy_pass', 'proxy_http_version', 'proxy_set_header', 'proxy_buffering', 'proxy_cache',
                    'proxy_read_timeout', 'proxy_send_timeout', 'proxy_connect_timeout', 'client_max_body_size',
                    'access_log', 'error_log'}
    for project, (host, port) in TARGETS.items():
        candidates = [server for server in servers
                      if any(h[0] == 'server_name' and host in h[1:] for h, _ in server)
                      and any(h[0] == 'listen' and 'ssl' in h and (h[1] == '443' or h[1].endswith(':443')) for h, _ in server)]
        assert len(candidates) == 1, 'Expected one HTTPS server for '+project
        locations = []
        for header, children in candidates[0]:
            directive = header[0]
            assert directive in server_fields or directive.startswith('ssl_'), 'Unsupported Nginx server routing directive'
            if directive == 'location':
                locations.append((header, children))
            else:
                assert children is None, 'Unsupported Nginx server block'
            if directive == 'include':
                assert len(header) == 2 and header[1] in included_files, 'Cannot verify Nginx include'
                included = nginx_directives(included_files[header[1]])
                assert all(h[0].startswith('ssl_') and nested is None for h, nested in included), 'Only TLS-only Nginx includes are supported'
        assert len(locations) == 1 and locations[0][0] == ['location', '/'], 'Expected a single root proxy location'
        route = locations[0][1]
        assert all(h[0] in proxy_fields and nested is None for h, nested in route), 'Unsupported Nginx location routing directive'
        proxy_passes = [h for h, _ in route if h[0] == 'proxy_pass']
        assert proxy_passes == [['proxy_pass', 'http://127.0.0.1:'+str(port)]], 'Nginx root proxy mapping changed: '+project
    return hashlib.sha256(proc.stdout).hexdigest()

def curl(url, resolve=None):
    args = ['curl', '--noproxy', '*', '--connect-timeout', '5', '--max-time', '15', '-sS', '-w', '\n%{http_code}', url]
    if resolve:
        args += ['--resolve', resolve]
    output = run(args)
    body, code = output.rsplit(b'\n', 1)
    return int(code), body

def verify_runtime_binding(project, app):
    expected = {'3000/tcp': [{'HostIp': '127.0.0.1', 'HostPort': str(TARGETS[project][1])}]}
    assert app['HostConfig']['PortBindings'] == expected, 'Target port does not belong to the selected application'

def verify_runtime_config(project, app, config):
    # Compose config emits escaped dollars for re-use as a Compose input;
    # inspect contains the literal values passed to Docker.
    service = json.loads(json.dumps(config['services']['new-api']).replace('$$', '$'))
    supported = {'command', 'depends_on', 'entrypoint', 'environment', 'healthcheck', 'image',
                 'labels', 'networks', 'ports', 'pull_policy', 'restart', 'volumes', 'working_dir', 'user'}
    assert not set(service) - supported, 'Unsupported Compose fields require manual runtime verification'
    image = json.loads(run(['docker', 'image', 'inspect', service['image']]))[0]
    assert image['Id'] == app['Image'], 'Configured image differs from the running application'
    inherited = image['Config']
    expected_env = dict(item.split('=', 1) for item in inherited.get('Env') or [])
    expected_env.update(service.get('environment') or {})
    actual_env = dict(item.split('=', 1) for item in app['Config'].get('Env') or [])
    assert expected_env == actual_env, 'Compose environment differs from the running application'
    for key, runtime_key in [('command', 'Cmd'), ('entrypoint', 'Entrypoint'), ('working_dir', 'WorkingDir'), ('user', 'User')]:
        expected_value = service.get(key)
        if expected_value is None:
            expected_value = inherited.get(runtime_key)
        if runtime_key in ['User', 'WorkingDir']:
            expected_value = expected_value or ''
        assert expected_value == app['Config'].get(runtime_key), 'Compose process configuration differs: '+key
    expected_labels = dict(inherited.get('Labels') or {})
    expected_labels.update(service.get('labels') or {})
    actual_labels = {key: value for key, value in app['Config']['Labels'].items() if not key.startswith('com.docker.compose.')}
    assert expected_labels == actual_labels, 'Compose application labels differ'
    mounts = service.get('volumes') or []
    expected_mounts = {}
    for mount in mounts:
        assert set(mount) <= {'type', 'source', 'target', 'read_only', 'bind'}, 'Unsupported mount options'
        assert mount['type'] == 'bind' and mount['target'] in ['/data', '/app/logs'], 'Unsupported application mount'
        assert set(mount.get('bind') or {}) <= {'create_host_path'}, 'Unsupported bind mount options'
        assert Path(mount['source']).is_absolute(), 'Mount source must be absolute'
        expected_mounts[mount['target']] = (mount['type'], str(Path(mount['source']).resolve()), not mount.get('read_only', False))
    assert '/data' in expected_mounts and len(expected_mounts) == len(mounts), 'A unique persistent /data mount is required'
    actual_mounts = {m['Destination']: (m['Type'], m['Source'], m['RW']) for m in app['Mounts']}
    assert expected_mounts == actual_mounts, 'Compose mounts differ from the running application'
    expected_ports = {}
    for port in service.get('ports') or []:
        assert set(port) <= {'mode', 'host_ip', 'target', 'published', 'protocol'}, 'Unsupported port options'
        assert port.get('mode', 'ingress') == 'ingress', 'Unsupported port mode'
        key = str(port['target'])+'/'+port.get('protocol', 'tcp')
        expected_ports.setdefault(key, []).append({'HostIp': port.get('host_ip', ''), 'HostPort': str(port['published'])})
    assert expected_ports == app['HostConfig']['PortBindings'], 'Compose ports differ from the running application'
    verify_runtime_binding(project, app)
    expected_networks = set()
    for key, settings in (service.get('networks') or {}).items():
        assert not settings, 'Custom application network settings require manual verification'
        expected_networks.add(config['networks'][key]['name'])
    assert expected_networks == set(app['NetworkSettings']['Networks']), 'Compose networks differ from the running application'
    restart = service.get('restart', 'no').split(':')
    assert app['HostConfig']['RestartPolicy'] == {'Name': restart[0], 'MaximumRetryCount': int(restart[1]) if len(restart) == 2 else 0}, 'Compose restart policy differs'
    health = dict(inherited.get('Healthcheck') or {})
    units = {'h': 3600000000000, 'm': 60000000000, 's': 1000000000, 'ms': 1000000, 'us': 1000, 'µs': 1000, 'ns': 1}
    for key, value in (service.get('healthcheck') or {}).items():
        if key in ['test', 'retries']:
            health[key.title()] = value
        else:
            runtime_key = {'interval': 'Interval', 'timeout': 'Timeout', 'start_period': 'StartPeriod', 'start_interval': 'StartInterval'}.get(key)
            assert runtime_key, 'Unsupported healthcheck option'
            parts = re.findall(r'(\d+(?:\.\d+)?)(ms|us|µs|ns|h|m|s)', value)
            assert ''.join(number+unit for number, unit in parts) == value and parts, 'Unsupported healthcheck duration'
            health[runtime_key] = int(sum(Decimal(number)*units[unit] for number, unit in parts))
    assert health == (app['Config'].get('Healthcheck') or {}), 'Compose healthcheck differs'

def accept(project, image_id, binary_hash, asset, revision=''):
    revision = SHA if revision == '' else revision
    name = container_name(project)
    host, port = TARGETS[project]
    deadline = time.monotonic()+150
    while True:
        c = inspect(name)
        if c['State'].get('Health', {}).get('Status') == 'healthy':
            break
        assert c['State']['Status'] == 'running', 'Application stopped'
        assert time.monotonic() < deadline, 'Healthcheck timeout'
        time.sleep(2)
    assert c['Image'] == image_id, 'Wrong application image'
    assert c['Config']['Labels'].get('org.opencontainers.image.revision') == revision, 'Wrong runtime revision'
    assert c['RestartCount'] == 0, 'Application restarted during startup'
    verify_runtime_binding(project, c)
    verify_data_services(project, c)
    actual_hash = run(['docker', 'exec', name, 'sha256sum', '/new-api']).decode().split()[0]
    assert actual_hash == binary_hash, 'Runtime binary differs'
    startup_end = datetime.datetime.fromisoformat(c['State']['StartedAt'].replace('Z','+00:00')) + datetime.timedelta(minutes=2)
    log_result = subprocess.run(['docker', 'logs', '--since', c['State']['StartedAt'], '--until', startup_end.isoformat(), name], capture_output=True)
    assert log_result.returncode == 0, 'Cannot inspect startup logs'
    logs = (log_result.stdout + log_result.stderr).decode(errors='replace')
    assert not re.search(r'(?i)(panic:|fatal error:|failed to (?:initialize|connect|migrate)|migration.*(?:failed|error)|SQLSTATE)', logs), 'Startup error detected'
    observations = {}
    for label, url, resolve in [
        ('local', f'http://127.0.0.1:{port}', None),
        ('nginx', 'https://'+host, f'{host}:443:127.0.0.1'),
        ('public', 'https://'+host, None),
    ]:
        code, body = curl(url+'/api/status', resolve)
        assert code == 200 and json.loads(body).get('success') is True, label+' health failed'
        code, body = curl(url+CONFIG.get('page_path', '/'), resolve)
        assert code == 200 and asset.encode() in body, label+' frontend revision mismatch'
        code, body = curl(url+CONFIG.get('protected_api_path', '/api/channel/'), resolve)
        assert code == 401, label+' unauthorized protected API access'
        observations[label] = {'status':200,'page':200,'unauthenticated_api':401,'frontend_asset':asset}
    return observations

def backup(project, c, files, expected, stamp, resolved):
    destination = BACKUP_ROOT/project/(stamp+'-'+SHA[:12])
    destination.mkdir(parents=True, mode=0o700, exist_ok=False)
    project_dir = PROJECT_DIRS[project]
    assert project_dir.resolve() == project_dir and destination.resolve().is_relative_to(BACKUP_ROOT)
    config_files = set(files)
    config_files.update(str(p) for p in project_dir.glob('.env*') if p.is_file())
    run(['sudo','-n','tar','-czf',str(destination/'config.tar.gz'),'-C','/',*[str(Path(f).relative_to('/')) for f in sorted(config_files)]])
    run(['sudo','-n','chown',f'{os.getuid()}:{os.getgid()}',str(destination/'config.tar.gz')])
    run(['tar','-tzf',str(destination/'config.tar.gz')])
    # This is a restricted configuration backup, not an output report. It retains
    # resolved environment values so rollback does not depend on mutable env files.
    # Preserve Compose's own escaping and verify that the private snapshot can
    # be read independently of the original environment/configuration files.
    resolved_file = destination/'compose.resolved.json'
    resolved_file.write_text(json.dumps(resolved))
    round_trip = json.loads(run(compose(project,[str(resolved_file)])+['config','--format','json']))
    assert round_trip == resolved, 'Saved rollback configuration does not round-trip'
    pg_command = ['docker','exec',container_name(project, 'postgres'),'sh','-c','exec pg_dump -Fc --no-owner --no-acl -U "$POSTGRES_USER" -d "$POSTGRES_DB"']
    dump_path = destination/'postgres.dump'
    with dump_path.open('wb') as stream:
        result = subprocess.run(pg_command, stdout=stream, stderr=subprocess.PIPE)
    assert result.returncode == 0, 'Native database dump failed'
    with dump_path.open('rb') as stream:
        assert dump_path.stat().st_size > 1000 and stream.read(5) == b'PGDMP', 'Invalid database dump'
    with dump_path.open('rb') as stream:
        run(['docker','exec','-i',container_name(project, 'postgres'),'pg_restore','--list'],stdin=stream)
    with dump_path.open('rb') as src, gzip.open(destination/'postgres.dump.gz','wb') as dst:
        shutil.copyfileobj(src,dst)
    run(['gzip','-t',str(destination/'postgres.dump.gz')])
    dump_path.unlink()
    app_env = dict(value.split('=',1) for value in c['Config']['Env'] if '=' in value)
    db_env = dict(value.split('=',1) for value in inspect(container_name(project, 'postgres'))['Config']['Env'] if '=' in value)
    assert urlparse(app_env['SQL_DSN']).path.lstrip('/') == db_env['POSTGRES_DB'], 'Unexpected application database'
    redis_password = unquote(urlparse(app_env['REDIS_CONN_STRING']).password or '')
    rdb_path = '/tmp/new-api-deploy-'+SHA[:12]+'-'+stamp+'.rdb'
    redis_script = 'IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; redis-cli --rdb '+rdb_path+' && redis-check-rdb '+rdb_path
    run(['docker','exec','-i',container_name(project, 'redis'),'sh','-c',redis_script],input=(redis_password+'\n').encode())
    run(['docker','cp',container_name(project, 'redis')+':'+rdb_path,str(destination/'redis.rdb')])
    run(['docker','exec',container_name(project, 'redis'),'rm',rdb_path])
    assert (destination/'redis.rdb').stat().st_size>8
    data_mounts = [mount for mount in c['Mounts'] if mount['Destination']=='/data']
    assert len(data_mounts)==1, 'A single persistent /data mount is required'
    data_path = Path(data_mounts[0]['Source']).resolve()
    assert data_path.is_dir() and data_path != Path('/'), 'Unexpected application data mount'
    run(['sudo','-n','tar','-czf',str(destination/'app-data.tar.gz'),'-C',str(data_path.parent),data_path.name])
    run(['sudo','-n','chown',f'{os.getuid()}:{os.getgid()}',str(destination/'app-data.tar.gz')])
    run(['tar','-tzf',str(destination/'app-data.tar.gz')])
    safe_inspect = {key:c[key] for key in ['Id','Image','Mounts','NetworkSettings','RestartCount']}
    safe_inspect['HostConfig'] = {key:c['HostConfig'].get(key) for key in ['PortBindings','NetworkMode','RestartPolicy']}
    safe_inspect['Config'] = {key:c['Config'].get(key) for key in ['Image','Labels','Entrypoint','Cmd','WorkingDir']}
    (destination/'application-inspect.json').write_text(json.dumps(safe_inspect,indent=2))
    (destination/'containers-before.json').write_text(json.dumps(expected,indent=2))
    old_tag = 'new-api-local:rollback-'+project+'-'+stamp
    run(['docker','tag',c['Image'],old_tag])
    rollback_override = destination/'rollback-image.yaml'
    rollback_override.write_text('services:\n  new-api:\n    image: '+old_tag+'\n    pull_policy: never\n')
    rollback_command = compose(project,[str(destination/'compose.resolved.json'),str(rollback_override)])+['up','-d','--no-deps','--no-build','new-api']
    (destination/'rollback.sh').write_text('#!/bin/sh\nset -eu\n'+shlex.join(rollback_command)+'\n')
    os.chmod(destination/'rollback.sh',0o700)
    hashes = []
    for path in sorted(destination.iterdir()):
        if path.is_file():
            assert path.stat().st_size>0,path.name
            hashes.append(file_hash(path)+'  '+path.name)
    (destination/'SHA256SUMS').write_text('\n'.join(hashes)+'\n')
    run(['sha256sum','-c','SHA256SUMS'],cwd=destination)
    (destination/'BACKUP_VALIDATED').write_text('passed\n')
    return destination, rollback_command

def deploy(project):
    assert project in TARGETS
    completed_file = ROOT/'rollout-state.json'
    expected = json.loads((completed_file if completed_file.exists() else ROOT/'initial-containers.json').read_text())
    protected_check(expected)
    assert not (ROOT/'release-failed.json').exists(), 'Prior failure requires reconciliation before another rollout'
    prerequisites = next(target.get('requires', []) for target in CONFIG['targets'] if target['project']==project)
    for prerequisite in prerequisites:
        report_path = ROOT/(prerequisite+'-acceptance.json')
        assert report_path.exists(), 'Prerequisite acceptance is missing: '+prerequisite
        receipt = json.loads(report_path.read_text())
        assert receipt['result']=='passed' and receipt['commit']==SHA, 'Prerequisite was not accepted for this commit'
        verify_target(prerequisite, receipt)
    manifest = CONFIG
    image = json.loads(run(['docker','image','inspect',IMAGE]))[0]
    assert image['Id']==manifest['image_id'], 'Release image tag moved'
    assert image['Config']['Labels']['org.opencontainers.image.revision']==SHA
    assert image['Architecture']=='amd64' and image['Os']=='linux'
    receipt_path = ROOT/(project+'-acceptance.json')
    if receipt_path.exists():
        receipt = json.loads(receipt_path.read_text())
        assert receipt['result']=='passed' and receipt['commit']==SHA, 'Existing receipt needs reconciliation'
        verify_target(project, receipt)
        print(json.dumps({'result':'already_current','project':project}),flush=True)
        return
    c = inspect(container_name(project))
    assert c['Id']==expected[container_name(project)]['id']
    files = c['Config']['Labels']['com.docker.compose.project.config_files'].split(',')
    before_config = json.loads(run(compose(project,files)+['config','--format','json']))
    verify_runtime_config(project, c, before_config)
    verify_data_services(project, c)
    nginx_hash = nginx_check()
    before_counts = counts(project)
    old_binary_hash = run(['docker','exec',container_name(project),'sha256sum','/new-api']).decode().split()[0]
    old_code, old_page = curl('http://127.0.0.1:'+str(TARGETS[project][1])+'/')
    assert old_code==200, 'Cannot capture previous frontend'
    old_asset = re.search(rb'static/js/[A-Za-z0-9_.-]+\.js',old_page)
    assert old_asset, 'Cannot identify previous frontend asset'
    old_asset = old_asset.group(0).decode()
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d-%H%M%S-%f')
    destination, rollback_command = backup(project,c,files,expected,stamp,before_config)
    print(json.dumps({'stage':'backup_validated','project':project,'backup':str(destination),'data_counts':before_counts}),flush=True)
    protected_check(expected)
    override = ROOT/('compose.'+project+'.yaml')
    assert not override.exists(),'Deployment override already exists; inspect the previous attempt'
    override.write_text('services:\n  new-api:\n    image: '+IMAGE+'\n    pull_policy: never\n    labels:\n      org.opencontainers.image.revision: '+SHA+'\n')
    new_files = files+[str(override)]
    after_config = json.loads(run(compose(project,new_files)+['config','--format','json']))
    expected_config = copy.deepcopy(before_config)
    expected_config['services']['new-api']['image']=IMAGE
    expected_config['services']['new-api']['pull_policy']='never'
    expected_config['services']['new-api'].setdefault('labels',{})['org.opencontainers.image.revision']=SHA
    assert after_config==expected_config,'Compose changed beyond application image and revision'
    protected_check(expected)
    updated = False
    try:
        updated = True
        run(compose(project,new_files)+['up','-d','--no-deps','--no-build','new-api'],timeout=120)
        verify_runtime_config(project, inspect(container_name(project)), after_config)
        observations = accept(project,image['Id'],manifest['sha256'],manifest['frontend_asset'])
        current = protected_check(expected,excluded=[container_name(project)])
        assert nginx_check()==nginx_hash,'Proxy configuration changed'
        after_counts=counts(project)
        assert after_counts==before_counts,'Persistent configuration counts changed'
        report = {'result':'passed','commit':SHA,'project':project,'service':'new-api','host':TARGETS[project][0],
                  'previous':identity(c),'current':current[container_name(project)],'backup':str(destination),
                  'backup_validated':True,'rollback_command':str(destination/'rollback.sh'),'checks':observations,
                  'persistent_counts':after_counts,'protected_containers_unchanged':True,'nginx_unchanged':True,
                  'authenticated_admin_ui_tested':False}
        (ROOT/(project+'-acceptance.json')).write_text(json.dumps(report,indent=2))
        expected[container_name(project)]=current[container_name(project)]
        completed_file.write_text(json.dumps(expected,indent=2))
        print(json.dumps(report),flush=True)
    except Exception as error:
        failure = {'result':'failed','project':project,'backup':str(destination),'reason_type':type(error).__name__}
        (ROOT/'release-failed.json').write_text(json.dumps(failure,indent=2))
        if updated:
            try:
                run(rollback_command,timeout=120)
                accept(project,c['Image'],old_binary_hash,old_asset,c['Config']['Labels'].get('org.opencontainers.image.revision'))
                rollback_state = protected_check(expected,excluded=[container_name(project)])
                expected[container_name(project)]=rollback_state[container_name(project)]
                completed_file.write_text(json.dumps(expected,indent=2))
                failure['result']='rolled_back'
            except Exception as rollback_error:
                failure['rollback_error_type']=type(rollback_error).__name__
            (ROOT/'release-failed.json').write_text(json.dumps(failure,indent=2))
            print(json.dumps(failure),flush=True)
        raise

def verify_data_services(project, app):
    env = dict(value.split('=',1) for value in app['Config']['Env'] if '=' in value)
    for key, service in [('SQL_DSN','postgres'),('REDIS_CONN_STRING','redis')]:
        uri = urlparse(env[key])
        peer = inspect(container_name(project, service))
        shared = set(app['NetworkSettings']['Networks']) & set(peer['NetworkSettings']['Networks'])
        assert shared, 'External data service: no shared application network'
        aliases = set()
        addresses = set()
        for name in shared:
            network = peer['NetworkSettings']['Networks'][name]
            aliases.update(network.get('Aliases') or [])
            addresses.update(network[key] for key in ['IPAddress', 'GlobalIPv6Address'] if network.get(key))
        aliases.update(addresses)
        assert uri.hostname in aliases, 'External data service requires a separate backup procedure'
        resolved = {line.split()[0] for line in run(['docker', 'exec', app['Name'].lstrip('/'), 'getent', 'hosts', uri.hostname]).decode().splitlines() if line.split()}
        assert resolved and resolved <= addresses, 'Data service DNS does not resolve to the selected backup container'
        assert uri.scheme in (['postgres','postgresql'] if service=='postgres' else ['redis']), 'Unsupported data service URI'
        assert uri.port in [None, 5432 if service == 'postgres' else 6379], 'Unsupported data service port'
        query = parse_qs(uri.query, keep_blank_values=True)
        assert not uri.fragment and (not query or (service == 'postgres' and query == {'sslmode': ['disable']})), 'Data service URI options require manual verification'
        if service == 'redis':
            assert not uri.username or uri.username == 'default', 'Redis ACL users require a separate backup procedure'
        if service=='postgres':
            peer_env = dict(value.split('=',1) for value in peer['Config']['Env'] if '=' in value)
            assert uri.path.lstrip('/')==peer_env['POSTGRES_DB'], 'Unexpected application database'

def verify_target(project, receipt):
    assert receipt['commit']==SHA and receipt['current']['image_id']==CONFIG['image_id'], 'Receipt identity mismatch'
    assert identity(inspect(container_name(project)))==receipt['current'], 'Accepted application changed'
    accept(project,CONFIG['image_id'],CONFIG['sha256'],CONFIG['frontend_asset'])
    backup_path=Path(receipt['backup']).resolve()
    assert backup_path.is_relative_to(BACKUP_ROOT), 'Unexpected backup location'
    assert (backup_path/'BACKUP_VALIDATED').read_text().strip()=='passed'
    run(['sha256sum','-c','SHA256SUMS'],cwd=backup_path)

def main():
    if not __debug__:
        raise SystemExit('Run without -O; deployment guards must remain enabled')
    parser=argparse.ArgumentParser(description='Release a verified New API image to one Compose/PostgreSQL/Redis instance at a time.')
    parser.add_argument('--config',type=Path,required=True,help='Non-secret release manifest; artifacts and receipts live beside it')
    parser.add_argument('action',choices=['inspect','snapshot','deploy','verify'])
    parser.add_argument('project',nargs='?')
    args=parser.parse_args()
    os.umask(0o077)
    initialize(args.config)
    if args.project:
        assert args.project in TARGETS, 'Unknown target'
    if args.action=='inspect':
        nginx_check()
        for project in TARGETS:
            app=inspect(container_name(project));verify_data_services(project,app)
            files = app['Config']['Labels']['com.docker.compose.project.config_files'].split(',')
            verify_runtime_config(project, app, json.loads(run(compose(project,files)+['config','--format','json'])))
            print(json.dumps({'project':project,'hostname':TARGETS[project][0],'port':TARGETS[project][1],
                              'directory':str(PROJECT_DIRS[project]),'current':identity(app)}))
    elif args.action=='snapshot':
        assert not (ROOT/'initial-containers.json').exists(), 'Never overwrite the original snapshot'
        nginx_check()
        for project in TARGETS:
            app = inspect(container_name(project))
            verify_runtime_binding(project, app)
            verify_data_services(project, app)
        with (ROOT/'initial-containers.json').open('x') as stream:
            json.dump(snapshot(),stream,indent=2)
        print('SNAPSHOT_CAPTURED')
    elif args.action=='deploy':
        assert args.project, 'Select exactly one project to deploy'
        deploy(args.project)
    else:
        nginx_check()
        expected_path=ROOT/'rollout-state.json'
        protected_check(json.loads(expected_path.read_text()))
        for project in ([args.project] if args.project else TARGETS):
            receipt=json.loads((ROOT/(project+'-acceptance.json')).read_text())
            assert receipt['result']=='passed', 'Target has not passed acceptance'
            verify_target(project,receipt)
        print('FINAL_ACCEPTANCE=passed')

if __name__=='__main__':
    main()
