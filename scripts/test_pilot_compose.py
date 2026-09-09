"""Render only, using dummy credentials and no private .env or Docker daemon."""
import json
import os
import subprocess
import unittest


class PilotComposeTest(unittest.TestCase):
    def test_private_ports_and_host_contract(self):
        env = {'PATH': os.environ['PATH'],
               'PAGEWRIGHT_POSTGRES_PASSWORD': 'test-only-database-password',
               'PAGEWRIGHT_REDIS_PASSWORD': 'test-only-redis-password',
               'PAGEWRIGHT_SERVICE_TOKEN': 'test-only-service-token',
               'PAGEWRIGHT_JWT_SECRET': 'test-only-jwt-not-a-production-secret'}
        result = subprocess.run(['docker', 'compose', '--env-file', '/dev/null', '-p', 'pagewright-pilot',
                                 '-f', 'docker-compose.yaml', '-f', 'docker-compose.pilot.yaml',
                                 'config', '--format', 'json'], env=env, check=True, capture_output=True, text=True)
        config = json.loads(result.stdout)
        expected = {'gateway': ('8085', 8085), 'ui': ('3000', 80), 'nginx': ('8084', 80)}
        for name, service in config['services'].items():
            ports = service.get('ports', [])
            if name in expected:
                self.assertEqual(len(ports), 1)
                self.assertEqual(ports[0]['host_ip'], '127.0.0.1')
                self.assertEqual((ports[0]['published'], ports[0]['target']), expected[name])
            else:
                self.assertEqual(ports, [])
            self.assertFalse(service.get('privileged', False))
        gateway = config['services']['gateway']
        self.assertEqual(gateway['environment']['PAGEWRIGHT_HOSTING_SCHEME'], 'https')
        self.assertEqual(gateway['environment']['PAGEWRIGHT_HOSTING_PORT'], '443')
        self.assertEqual(gateway['environment']['PAGEWRIGHT_SIGNUP_MODE'], 'closed')
        self.assertEqual(gateway['environment']['PAGEWRIGHT_APP_ORIGINS'], 'https://app.pagewright.io')
        mount = next(v for v in gateway['volumes'] if v['target'] == '/run/pagewright-tls')
        self.assertTrue(mount['read_only'])
        self.assertEqual(mount['source'], '/var/lib/pagewright-tls/public')
        self.assertEqual(config['services']['manager']['environment']['PAGEWRIGHT_WORKER_NETWORK'], 'pagewright-pilot_pagewright')


if __name__ == '__main__':
    unittest.main()
