import json
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

import pilot_tls as tls


class PilotTLSTests(unittest.TestCase):
    def test_real_certificate_hostname_check(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
                            '-keyout', str(path / 'privkey.pem'), '-out', str(path / 'fullchain.pem'),
                            '-days', '1', '-subj', '/CN=demo.pagewright.io',
                            '-addext', 'subjectAltName=DNS:demo.pagewright.io,DNS:demo.preview.pagewright.io'],
                           check=True, capture_output=True)
            self.assertTrue(tls.certificate_ready(path, ('demo.pagewright.io', 'demo.preview.pagewright.io')))
            self.assertFalse(tls.certificate_ready(path, ('other.pagewright.io',)))

    def test_namespace(self):
        for name in ['pagewright.io', 'evil.test', 'app.pagewright.io',
                     'www.pagewright.io', 'api.pagewright.io', 'preview.pagewright.io',
                     'a.b.pagewright.io', 'A.pagewright.io', '-a.pagewright.io',
                     'a.pagewright.io;return 200;', 'a' * 64 + '.pagewright.io']:
            with self.subTest(name=name), self.assertRaises(ValueError):
                tls.groups([name])
        self.assertEqual(tls.groups(['demo.pagewright.io'])['pw-site-demo'],
                         ('demo.pagewright.io', 'demo.preview.pagewright.io'))
        with self.assertRaises(ValueError):
            tls.groups(['a.pagewright.io'] * 2)

    def test_render_pending_and_ready(self):
        entries = tls.groups(['demo.pagewright.io'])
        pending = tls.render(entries, set(), '/certs', '/webroot')
        self.assertNotIn('proxy_pass', pending)
        self.assertIn('ssl_reject_handshake on;', pending)
        self.assertIn('location ^~ /.well-known/acme-challenge/', pending)
        self.assertNotIn('server_name pagewright.io;', pending)
        self.assertNotIn('ublo.ro', pending)
        ready = tls.render(entries, set(entries), '/certs', '/webroot')
        self.assertIn('https://demo.preview.pagewright.io$request_uri', ready)
        self.assertIn('proxy_pass http://127.0.0.1:8084;', ready)
        self.assertIn('proxy_pass http://127.0.0.1:8085;', ready)
        self.assertIn('proxy_pass http://127.0.0.1:3000;', ready)
        self.assertIn('proxy_set_header Host demo.pagewright.io;', ready)
        self.assertNotIn('includeSubDomains', ready)

    def test_retry_limits(self):
        self.assertTrue(tls.eligible([], 'site', 100000))
        self.assertFalse(tls.eligible([{'at': 99999, 'name': 'site'}], 'site', 100000))
        self.assertTrue(tls.eligible([{'at': 90000, 'name': 'site'}], 'site', 100000))
        self.assertFalse(tls.eligible([{'at': 99999, 'name': str(i)} for i in range(10)], 'site', 100000))

    def test_config_rollback_and_recovery(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            (base / 'public').mkdir()
            conf = base / 'nginx.conf'
            old, new = tls.MARKER + 'old', tls.MARKER + 'new'
            conf.write_text(old)
            with patch.object(tls, 'reload_nginx', side_effect=[RuntimeError('reload'), None]):
                with self.assertRaises(RuntimeError):
                    tls.install_config(new, conf, base)
            self.assertEqual(conf.read_text(), old)
            self.assertFalse((base / 'previous-config.json').exists())
            self.assertEqual(json.loads((base / 'public/ready.json').read_text())['hosts'], {})
            with patch.object(tls, 'reload_nginx', side_effect=RuntimeError('reload')):
                with self.assertRaises(RuntimeError):
                    tls.install_config(new, conf, base)
            self.assertTrue((base / 'previous-config.json').exists())
            with patch.object(tls, 'reload_nginx') as reload:
                tls.install_config(new, conf, base)
                self.assertEqual(reload.call_count, 2)
            self.assertEqual(conf.read_text(), new)
            with patch.object(tls, 'reload_nginx') as reload:
                tls.install_config(new, conf, base)
                reload.assert_not_called()

    def test_reject_unmanaged_config(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            conf = base / 'nginx.conf'
            conf.write_text('operator config')
            with self.assertRaises(ValueError):
                tls.install_config(tls.MARKER, conf, base)
            self.assertEqual(conf.read_text(), 'operator config')

    def test_read_only_default(self):
        import argparse
        args = argparse.Namespace(apply=False, postgres_container='pagewright-pilot-postgres-1')
        with patch.object(tls, 'inventory', return_value=['demo.pagewright.io']), patch('builtins.print'), patch.object(tls, 'install_config') as install:
            tls.reconcile(args)
            install.assert_not_called()

    def test_inventory_command_is_fixed(self):
        with patch.object(tls, 'run', return_value='demo.pagewright.io\n') as run:
            self.assertEqual(tls.inventory('pagewright-pilot-postgres-1'), ['demo.pagewright.io'])
            self.assertIn('SELECT fqdn', run.call_args.args[0][-1])
        with self.assertRaises(ValueError):
            tls.inventory('other-container')

    def test_failed_issuance_is_persisted_and_not_immediately_retried(self):
        import argparse
        args = argparse.Namespace(apply=True, production=False, email='operator@example.test', postgres_container='pagewright-pilot-postgres-1')
        with tempfile.TemporaryDirectory() as directory, patch.object(tls, 'BASE', Path(directory)), patch.object(tls.os, 'geteuid', return_value=0), patch.object(tls, 'inventory', return_value=['demo.pagewright.io']), patch.object(tls, 'install_config'), patch.object(tls, 'certificate_ready', return_value=False), patch.object(tls, 'run', side_effect=RuntimeError('failure')) as run, patch('builtins.print'):
            tls.reconcile(args)
            self.assertEqual(run.call_count, 2)
            for call in run.call_args_list:
                self.assertIn('--staging', call.args[0])
                self.assertIn('--webroot', call.args[0])
            run.reset_mock()
            tls.reconcile(args)
            run.assert_not_called()
            state = json.loads((Path(directory) / 'public/ready.json').read_text())
            self.assertEqual(state['hosts'], {})

    def test_readiness_requires_both_https_probes(self):
        import argparse
        args = argparse.Namespace(apply=True, production=True, email='operator@example.test', postgres_container='pagewright-pilot-postgres-1')
        with tempfile.TemporaryDirectory() as directory, patch.object(tls, 'BASE', Path(directory)), patch.object(tls.os, 'geteuid', return_value=0), patch.object(tls, 'inventory', return_value=['demo.pagewright.io']), patch.object(tls, 'install_config'), patch.object(tls, 'certificate_ready', return_value=True), patch.object(tls, 'run') as run, patch.object(tls, 'probe') as probe, patch('builtins.print'):
            tls.reconcile(args)
            self.assertEqual(probe.call_count, 4)
            run.assert_not_called()  # reuse existing certificates
            state_path = Path(directory) / 'public/ready.json'
            self.assertEqual(json.loads(state_path.read_text())['hosts'], {'demo.pagewright.io': True})
            probe.side_effect = lambda host: (_ for _ in ()).throw(OSError('probe failed')) if host == 'demo.preview.pagewright.io' else None
            tls.reconcile(args)
            self.assertEqual(json.loads(state_path.read_text())['hosts'], {})


if __name__ == '__main__':
    unittest.main()
