import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


class ReleaseGuardTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location('release_under_test', Path(__file__).parents[1]/'compose_release.py')
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.module.ROOT = self.root
        self.module.SHA = 'a'*40
        self.module.IMAGE = 'new-api-local:release-example'
        self.module.TARGETS = {'test': ('test.example.com', 3001), 'site': ('api.example.com', 3002)}
        self.module.CONFIG = {'commit':'a'*40,'image':'new-api-local:release-example','image_id':'sha256:'+'b'*64,
                              'sha256':'c'*64,'frontend_asset':'static/js/index.abc.js',
                              'targets':[{'project':'test'},{'project':'site','requires':['test']}]}
        self.baseline = {'test-app':{'id':'old-app','restarts':0},'test-db':{'id':'old-db','restarts':0}}
        (self.root/'initial-containers.json').write_text(json.dumps(self.baseline))

    def test_protected_database_restart_blocks_rollout_before_backup(self):
        changed = {'test-app':self.baseline['test-app'],'test-db':{'id':'old-db','restarts':1}}
        with patch.object(self.module,'snapshot',return_value=changed), patch.object(self.module,'run') as command:
            with self.assertRaisesRegex(AssertionError,'Protected container changed: test-db'):
                self.module.deploy('test')
            command.assert_not_called()

    def test_new_container_blocks_rollout_instead_of_silently_excluding_it(self):
        with patch.object(self.module,'snapshot',return_value={**self.baseline,'unrelated-app':{'id':'new'}}):
            with self.assertRaisesRegex(AssertionError,'inventory changed'):
                self.module.protected_check(self.baseline,excluded=['test-app'])

    def test_target_replacement_is_allowed_while_database_identity_is_preserved(self):
        changed = {**self.baseline,'test-app':{'id':'new-app','restarts':0}}
        with patch.object(self.module,'snapshot',return_value=changed):
            self.assertEqual(self.module.protected_check(self.baseline,excluded=['test-app']),changed)

    def test_domain_requires_accepted_test_receipt_before_inspecting_new_image(self):
        with patch.object(self.module,'snapshot',return_value=self.baseline), patch.object(self.module,'run') as command:
            with self.assertRaisesRegex(AssertionError,'Prerequisite acceptance is missing'):
                self.module.deploy('site')
            command.assert_not_called()

    def test_prior_failure_blocks_continuing_the_rollout(self):
        (self.root/'release-failed.json').write_text('{"result":"rolled_back"}')
        with patch.object(self.module,'snapshot',return_value=self.baseline), patch.object(self.module,'run') as command:
            with self.assertRaisesRegex(AssertionError,'Prior failure'):
                self.module.deploy('test')
            command.assert_not_called()

    def test_moved_release_tag_is_rejected_before_backup_or_replace(self):
        moved = json.dumps([{'Id':'sha256:'+'d'*64}]).encode()
        with patch.object(self.module,'snapshot',return_value=self.baseline), patch.object(self.module,'run',return_value=moved) as command:
            with self.assertRaisesRegex(AssertionError,'Release image tag moved'):
                self.module.deploy('test')
            self.assertEqual(command.call_args.args[0],['docker','image','inspect','new-api-local:release-example'])
            self.assertEqual(command.call_count,1)

    def test_invalid_commit_fails_before_any_docker_call(self):
        manifest=self.root/'release.json'
        manifest.write_text(json.dumps({**self.module.CONFIG,'commit':'short'}))
        with patch.object(self.module,'run') as command:
            with self.assertRaisesRegex(AssertionError,'full commit SHA'):
                self.module.initialize(manifest)
            command.assert_not_called()

    def test_cli_error_does_not_expose_stderr_credentials(self):
        result=subprocess.CompletedProcess(['docker','exec'],1,b'',b'password=example-secret')
        with patch.object(self.module.subprocess,'run',return_value=result):
            with self.assertRaises(RuntimeError) as error:
                self.module.run(['docker','exec'])
        self.assertNotIn('example-secret',str(error.exception))
        self.assertIn('exit=1',str(error.exception))

    def test_external_database_is_rejected_before_data_backup(self):
        app={'Config':{'Env':['SQL_DSN=postgres://user:example-secret@external.example.com/db','REDIS_CONN_STRING=redis://cache']},'NetworkSettings':{'Networks':{'network':{}}}}
        peer={'Name':'/custom-db','NetworkSettings':{'Networks':{'network':{'Aliases':['postgres'],'IPAddress':'172.20.0.2'}}}}
        with patch.object(self.module,'container_name',return_value='custom-db'), patch.object(self.module,'inspect',return_value=peer):
            with self.assertRaisesRegex(AssertionError,'External data service'):
                self.module.verify_data_services('test',app)

    def runtime_fixture(self):
        service = {'image':'old-image','environment':{'SQL_DSN':'postgres://postgres/db'},
                   'volumes':[{'type':'bind','source':str(self.root),'target':'/data'}],
                   'ports':[{'host_ip':'127.0.0.1','target':3000,'published':'3001'}],
                   'networks':{'default':None},'restart':'unless-stopped'}
        image = {'Id':'old-image-id','Config':{'Env':['PATH=/usr/bin'],'Cmd':None,'Entrypoint':['/new-api'],'WorkingDir':'/data','User':''}}
        app = {'Image':'old-image-id','Config':{**image['Config'],'Env':['PATH=/usr/bin','SQL_DSN=postgres://postgres/db'],'Labels':{}},
               'Mounts':[{'Type':'bind','Source':str(self.root),'Destination':'/data','RW':True}],
               'HostConfig':{'PortBindings':{'3000/tcp':[{'HostIp':'127.0.0.1','HostPort':'3001'}]},
                             'RestartPolicy':{'Name':'unless-stopped','MaximumRetryCount':0}},
               'NetworkSettings':{'Networks':{'test_default':{}}}}
        return app, {'services':{'new-api':service},'networks':{'default':{'name':'test_default'}}}, image

    def test_complete_runtime_contract_matches_before_release(self):
        app, config, image = self.runtime_fixture()
        with patch.object(self.module,'run',return_value=json.dumps([image]).encode()):
            self.module.verify_runtime_config('test',app,config)

    def test_compose_escaped_dollars_match_literal_runtime_environment(self):
        app, config, image = self.runtime_fixture()
        config['services']['new-api']['environment']['LITERAL']='example$$VALUE$$$$'
        app['Config']['Env'].append('LITERAL=example$VALUE$$')
        with patch.object(self.module,'run',return_value=json.dumps([image]).encode()):
            self.module.verify_runtime_config('test',app,config)

    def test_on_disk_runtime_drift_is_rejected(self):
        for change in ['removed_environment','changed_data_source','changed_command','changed_port']:
            with self.subTest(change=change):
                app, config, image = self.runtime_fixture()
                service=config['services']['new-api']
                if change=='removed_environment':service['environment']={}
                elif change=='changed_data_source':service['volumes'][0]['source']=str(self.root/'different')
                elif change=='changed_command':service['command']=['--different']
                else:service['ports'][0]['published']='3002'
                with patch.object(self.module,'run',return_value=json.dumps([image]).encode()):
                    with self.assertRaises(AssertionError):
                        self.module.verify_runtime_config('test',app,config)

    def test_hostname_port_for_another_application_is_rejected(self):
        app, _, _ = self.runtime_fixture()
        with self.assertRaisesRegex(AssertionError,'selected application'):
            self.module.verify_runtime_binding('site',app)

    def test_database_alias_on_an_unshared_network_is_rejected(self):
        app={'Name':'/test-app','Config':{'Env':['SQL_DSN=postgres://postgres/db']},'NetworkSettings':{'Networks':{'app-net':{}}}}
        peer={'Name':'/test-db','NetworkSettings':{'Networks':{'other-net':{'Aliases':['postgres'],'IPAddress':'172.20.0.2'}}}}
        with patch.object(self.module,'container_name',return_value='test-db'), patch.object(self.module,'inspect',return_value=peer), patch.object(self.module,'run') as command:
            with self.assertRaisesRegex(AssertionError,'no shared application network'):
                self.module.verify_data_services('test',app)
            command.assert_not_called()

    def test_database_dns_resolving_to_another_container_is_rejected(self):
        app={'Name':'/test-app','Config':{'Env':['SQL_DSN=postgres://postgres/db']},'NetworkSettings':{'Networks':{'app-net':{}}}}
        peer={'Name':'/test-db','NetworkSettings':{'Networks':{'app-net':{'Aliases':['postgres'],'IPAddress':'172.20.0.2'}}}}
        with patch.object(self.module,'container_name',return_value='test-db'), patch.object(self.module,'inspect',return_value=peer), patch.object(self.module,'run',return_value=b'172.20.0.99 postgres\n'):
            with self.assertRaisesRegex(AssertionError,'selected backup container'):
                self.module.verify_data_services('test',app)

    def test_nginx_single_root_proxy_binds_domain_to_target(self):
        config='server { listen 443 ssl; server_name api.example.com; location / { proxy_pass http://127.0.0.1:3002; proxy_set_header Connection ""; } }'
        self.module.TARGETS={'site':('api.example.com',3002)}
        with patch.object(self.module.subprocess,'run',return_value=subprocess.CompletedProcess([],0,config.encode(),b'')):
            self.assertEqual(len(self.module.nginx_check()),64)

    def test_nginx_unused_correct_port_does_not_hide_wrong_root_proxy(self):
        config='server { listen 443 ssl; server_name api.example.com; location / { proxy_pass http://127.0.0.1:3001; } location /unused { proxy_pass http://127.0.0.1:3002; } }'
        self.module.TARGETS={'site':('api.example.com',3002)}
        with patch.object(self.module.subprocess,'run',return_value=subprocess.CompletedProcess([],0,config.encode(),b'')):
            with self.assertRaisesRegex(AssertionError,'single root proxy location'):
                self.module.nginx_check()

    def test_nginx_rewrite_cannot_bypass_the_selected_root_proxy(self):
        config='server { listen 443 ssl; server_name api.example.com; location / { rewrite ^ https://test.example.com; proxy_pass http://127.0.0.1:3002; } }'
        self.module.TARGETS={'site':('api.example.com',3002)}
        with patch.object(self.module.subprocess,'run',return_value=subprocess.CompletedProcess([],0,config.encode(),b'')):
            with self.assertRaisesRegex(AssertionError,'Unsupported Nginx location'):
                self.module.nginx_check()

    def test_nginx_include_cannot_hide_additional_routing(self):
        config='# configuration file /site.conf:\nserver { listen 443 ssl; server_name api.example.com; include /extra.conf; location / { proxy_pass http://127.0.0.1:3002; } }\n# configuration file /extra.conf:\nlocation /api { proxy_pass http://127.0.0.1:3001; }\n'
        self.module.TARGETS={'site':('api.example.com',3002)}
        with patch.object(self.module.subprocess,'run',return_value=subprocess.CompletedProcess([],0,config.encode(),b'')):
            with self.assertRaisesRegex(AssertionError,'TLS-only'):
                self.module.nginx_check()


if __name__=='__main__':
    unittest.main()
