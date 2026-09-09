"""Explicit disposable real-nginx HTTPS acceptance. No ACME or remote host calls."""
from http import client as http_client
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch
import uuid

import pilot_tls as tls


class RuntimeTest(unittest.TestCase):
    def test_https_routing_and_pending_hosts(self):
        def docker(*args):
            return subprocess.check_output(['docker', '--host', 'unix:///var/run/docker.sock', *args], text=True).strip()

        name = 'pagewright-tls-test-' + uuid.uuid4().hex[:12]
        with tempfile.TemporaryDirectory(prefix='pagewright-tls-runtime-') as directory:
            base = Path(directory)
            base.chmod(0o755)
            entries = tls.groups(['demo.pagewright.io', 'pending.pagewright.io'])
            certs = base / 'certs'
            certs.mkdir()
            for certname, hosts in entries.items():
                path = certs / certname
                path.mkdir()
                subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
                                '-keyout', str(path / 'privkey.pem'), '-out', str(path / 'fullchain.pem'),
                                '-days', '1', '-subj', '/CN=' + hosts[0],
                                '-addext', 'subjectAltName=' + ','.join('DNS:' + h for h in hosts)],
                               capture_output=True, check=True)
            challenge = base / 'webroot/.well-known/acme-challenge'
            challenge.mkdir(parents=True)
            (challenge / 'test-token').write_text('infrastructure-token')
            generated = tls.render(entries, {'pw-platform', 'pw-site-demo'}, '/fixture/certs', '/fixture/webroot')
            fixture = ''.join(f'server {{ listen {p}; add_header Content-Security-Policy "default-src self" always; location / {{ return 200 "{p}|$http_host|$http_x_forwarded_proto"; }} }}' for p in (3000, 8084, 8085))
            # Model a separate apex/blog vhost to check exact-name precedence.
            fixture += 'server { listen 80; server_name pagewright.io www.pagewright.io ublo.ro; return 200 "existing-site"; }'
            (base / 'nginx.conf').write_text('events {} http { access_log off; ' + fixture + generated + ' }')
            started = False
            try:
                docker('run', '-d', '--name', name, '-p', '127.0.0.1::80', '-p', '127.0.0.1::443',
                       '-v', f'{base}:/fixture:ro', 'nginx:alpine', 'nginx', '-c', '/fixture/nginx.conf', '-g', 'daemon off;')
                started = True
                http_port = int(docker('port', name, '80').split(':')[-1])
                https_port = int(docker('port', name, '443').split(':')[-1])
                for attempt in range(50):
                    try:
                        with socket.create_connection(('127.0.0.1', http_port), timeout=1):
                            break
                    except OSError:
                        if attempt == 49:
                            raise
                        time.sleep(0.1)

                def http(host, path='/'):
                    connection = http_client.HTTPConnection('127.0.0.1', http_port, timeout=5)
                    try:
                        connection.request('GET', path, headers={'Host': host})
                        response = connection.getresponse()
                        return response.status, dict(response.getheaders()), response.read()
                    finally:
                        connection.close()

                self.assertEqual(http('pending.pagewright.io')[0], 503)
                self.assertEqual(http('pending.preview.pagewright.io', '/.well-known/acme-challenge/test-token')[2], b'infrastructure-token')
                self.assertEqual(http('unknown.pagewright.io')[0], 404)
                for host in ('pagewright.io', 'www.pagewright.io', 'ublo.ro'):
                    self.assertEqual(http(host)[2], b'existing-site')
                self.assertEqual(http('demo.pagewright.io')[1]['Location'], 'https://demo.pagewright.io/')
                for certname in ('pw-platform', 'pw-site-demo'):
                    context = ssl.create_default_context(cafile=str(certs / certname / 'fullchain.pem'))
                    for host in entries[certname]:
                        with socket.create_connection(('127.0.0.1', https_port), timeout=5) as raw:
                            with context.wrap_socket(raw, server_hostname=host) as connection:
                                connection.sendall(f'GET / HTTP/1.1\r\nHost: {host}\r\nConnection: close\r\n\r\n'.encode())
                                response = http_client.HTTPResponse(connection)
                                response.begin()
                                self.assertEqual(response.status, 200)
                                self.assertIn('max-age=86400', response.getheader('Strict-Transport-Security'))
                                self.assertEqual(response.getheader('Content-Security-Policy'), 'default-src self')
                                self.assertEqual(response.getheader('X-Content-Type-Options'), 'nosniff')
                                self.assertEqual(response.getheader('X-Frame-Options'), 'DENY')
                                port = 3000 if host.startswith('app.') else 8085 if host.startswith('api.') else 8084
                                self.assertEqual(response.read().decode(), f'{port}|{host}|https')
                        connect = socket.create_connection
                        with patch.object(tls.socket, 'create_connection', side_effect=lambda address, timeout: connect(('127.0.0.1', https_port), timeout)), patch.object(tls.ssl, 'create_default_context', return_value=context):
                            tls.probe(host)
                context = ssl.create_default_context(cafile=str(certs / 'pw-site-pending/fullchain.pem'))
                for host in ('pending.pagewright.io', 'unknown.pagewright.io'):
                    with socket.create_connection(('127.0.0.1', https_port), timeout=5) as raw:
                        with self.assertRaises(ssl.SSLError):
                            context.wrap_socket(raw, server_hostname=host)
            finally:
                if started:
                    docker('rm', '-f', name)


if __name__ == '__main__':
    unittest.main()
