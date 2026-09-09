#!/usr/bin/env python3
"""Host-only, closed-pilot HTTP-01 reconciler. No API request can invoke Certbot.

Default invocation is read-only. --apply requires root; production issuance also
requires --production. Install this file root-owned, outside the application tree.
"""
import argparse
import fcntl
import json
import os
from pathlib import Path
import re
import socket
import ssl
import subprocess
import tempfile
import time

DOMAIN = "pagewright.io"
BASE = Path("/var/lib/pagewright-tls")
CONF = Path("/etc/nginx/conf.d/pagewright-pilot.conf")
MARKER = "# Managed by pagewright pilot_tls.py\n"
LABEL = re.compile(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\Z")
RESERVED = {"app", "api", "www", "preview"}


def site_label(fqdn):
    suffix = "." + DOMAIN
    label = fqdn[:-len(suffix)] if fqdn.endswith(suffix) else ""
    if not LABEL.fullmatch(label) or label in RESERVED:
        raise ValueError("invalid registered pilot hostname")
    return label


def groups(sites):
    result = {"pw-platform": ("app." + DOMAIN, "api." + DOMAIN)}
    if len(sites) > 500 or len(set(sites)) != len(sites):
        raise ValueError("invalid pilot inventory size/duplicates")
    for site in sorted(sites):
        label = site_label(site)
        result["pw-site-" + label] = (site, label + ".preview." + DOMAIN)
    return result


def run(args, timeout=30):
    # Never use a shell or report command output that could contain credentials.
    result = subprocess.run(args, capture_output=True, text=True, timeout=timeout,
                            env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C"})
    if result.returncode:
        raise RuntimeError("command failed: " + Path(args[0]).name)
    return result.stdout


def inventory(container):
    if not re.fullmatch(r"pagewright-pilot-postgres-1", container):
        raise ValueError("only the dedicated pilot PostgreSQL container is supported")
    raw = run(["docker", "--host", "unix:///var/run/docker.sock", "exec", container, "psql", "-X", "-U", "pagewright",
               "-d", "pagewright", "-At", "-v", "ON_ERROR_STOP=1", "-c",
               "SELECT fqdn FROM sites WHERE initialization_status = 'ready' ORDER BY fqdn LIMIT 501;"])
    return raw.splitlines()


def atomic(path, data, mode=0o644):
    fd, temporary = tempfile.mkstemp(prefix=".pw-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            os.fchmod(stream.fileno(), mode)
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def certificate_ready(directory, hosts):
    try:
        run(["openssl", "x509", "-in", str(directory / "fullchain.pem"),
             "-noout", "-checkend", "3600"])
        for host in hosts:
            run(["openssl", "x509", "-in", str(directory / "fullchain.pem"),
                 "-noout", "-checkhost", host])
        return (directory / "privkey.pem").is_file()
    except (RuntimeError, subprocess.TimeoutExpired):
        return False


def render(entries, ready, certroot, webroot):
    # All names come from groups(), never HTTP Host headers or arbitrary aliases.
    lines = [MARKER]
    for name, hosts in entries.items():
        for host in hosts:
            redirect = f"return 308 https://{host}$request_uri;" if name in ready else "return 503;"
            lines.append(f"""server {{
    listen 80;
    listen [::]:80;
    server_name {host};
    location ^~ /.well-known/acme-challenge/ {{
        root {webroot};
        default_type text/plain;
        disable_symlinks on;
        try_files $uri =404;
    }}
    location / {{ {redirect} }}
}}
""")
            if name not in ready:
                lines.append(f"server {{ listen 443 ssl; listen [::]:443 ssl; server_name {host}; ssl_reject_handshake on; }}\n")
                continue
            port = 3000 if host == "app." + DOMAIN else 8085 if host == "api." + DOMAIN else 8084
            lines.append(f"""server {{
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name {host};
    ssl_certificate {certroot}/{name}/fullchain.pem;
    ssl_certificate_key {certroot}/{name}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    client_max_body_size 1m;
    proxy_hide_header Strict-Transport-Security;
    proxy_hide_header X-Content-Type-Options;
    proxy_hide_header X-Frame-Options;
    proxy_hide_header Referrer-Policy;
    add_header Strict-Transport-Security "max-age=86400" always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    add_header Referrer-Policy no-referrer always;
    location = /__pagewright_tls_ready {{ return 200 "pagewright-pilot-ready"; }}
    location ^~ /.well-known/acme-challenge/ {{ return 404; }}
    location / {{
        proxy_pass http://127.0.0.1:{port};
        proxy_set_header Host {host};
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
    }}
}}
""")
    # Exact existing apex/www hosts retain priority over these regex fallbacks.
    pattern = r'~^([a-z0-9-]+\.)+(pagewright\.io)$'
    lines.append(f"server {{ listen 80; listen [::]:80; server_name {pattern}; return 404; }}\n")
    lines.append(f"server {{ listen 443 ssl; listen [::]:443 ssl; server_name {pattern}; ssl_reject_handshake on; }}\n")
    return "".join(lines)


def reload_nginx():
    run(["nginx", "-t"])
    run(["systemctl", "reload", "nginx"])


def install_config(text, conf=CONF, base=BASE):
    journal = base / "previous-config.json"
    if journal.exists():
        previous = json.loads(journal.read_text())
        if previous is None:
            conf.unlink(missing_ok=True)
        else:
            if not previous.startswith(MARKER):
                raise ValueError("invalid recovery journal")
            atomic(conf, previous)
        reload_nginx()
        journal.unlink()
    if conf.is_symlink():
        raise ValueError("refusing symlinked pilot configuration")
    before = conf.read_text() if conf.exists() else None
    if before is not None and not before.startswith(MARKER):
        raise ValueError("refusing unmanaged pilot configuration")
    if before == text:
        return
    atomic(base / "public" / "ready.json", '{"expires":0,"hosts":{}}')
    atomic(journal, json.dumps(before), 0o600)
    atomic(conf, text)
    try:
        reload_nginx()
    except Exception:
        if before is None:
            conf.unlink(missing_ok=True)
        else:
            atomic(conf, before)
        reload_nginx()  # On failure retain the journal and block the next write.
        journal.unlink()
        raise
    journal.unlink()


def probe(host):
    # Verify a publicly trusted certificate AND that this vhost is installed.
    with socket.create_connection(("127.0.0.1", 443), timeout=5) as raw:
        with ssl.create_default_context().wrap_socket(raw, server_hostname=host) as connection:
            connection.sendall((f"GET /__pagewright_tls_ready HTTP/1.1\r\nHost: {host}\r\nConnection: close\r\n\r\n").encode())
            response = b""
            while len(response) < 8192:
                part = connection.recv(4096)
                if not part:
                    break
                response += part
            if not response.startswith(b"HTTP/1.1 200 ") or not response.endswith(b"pagewright-pilot-ready"):
                raise RuntimeError("HTTPS routing probe failed")


def eligible(attempts, name, now):
    recent = [item for item in attempts if item["at"] > now - 86400]
    return len(recent) < 10 and not any(item["name"] == name and item["at"] > now - 3600 for item in recent)


def reconcile(args):
    entries = groups(inventory(args.postgres_container))
    if not args.apply:
        print(json.dumps({"mode": "read-only", "certificates": entries}, indent=2))
        return
    if os.geteuid() != 0:
        raise ValueError("--apply must run as root")
    if not args.email or not re.fullmatch(r"[^\s@]+@[^\s@]+\.[^\s@]+", args.email):
        raise ValueError("an operator ACME email is required")
    BASE.mkdir(mode=0o755, exist_ok=True)
    for path in (BASE / "public", BASE / "webroot"):
        path.mkdir(mode=0o755, exist_ok=True)
    with (BASE / "controller.lock").open("w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        acme = BASE / ("production" if args.production else "staging")
        for path in (acme, acme / "config", acme / "work", acme / "logs"):
            path.mkdir(mode=0o700, exist_ok=True)
        # Nginx master reads keys; its unprivileged workers read only webroot.
        certroot = BASE / "production/config/live"
        ready = {name for name, hosts in entries.items() if certificate_ready(certroot / name, hosts)}
        install_config(render(entries, ready, certroot, BASE / "webroot"))
        attempts_path = acme / "attempts.json"
        attempts = json.loads(attempts_path.read_text()) if attempts_path.exists() else []
        now = time.time()
        attempts = [item for item in attempts if item["at"] > now - 86400]
        issued = 0
        for name, hosts in entries.items():
            if certificate_ready(acme / "config/live" / name, hosts) or not eligible(attempts, name, now) or issued >= 2:
                continue
            attempts.append({"name": name, "at": now})
            atomic(attempts_path, json.dumps(attempts), 0o600)  # persist before network work
            issued += 1
            command = ["certbot", "certonly", "--non-interactive", "--agree-tos", "--email", args.email,
                       "--webroot", "-w", str(BASE / "webroot"), "--preferred-challenges", "http",
                       "--config-dir", str(acme / "config"), "--work-dir", str(acme / "work"),
                       "--logs-dir", str(acme / "logs"), "--cert-name", name]
            if not args.production:
                command.append("--staging")
            for host in hosts:
                command.extend(["-d", host])
            try:
                run(command, timeout=180)
            except (RuntimeError, subprocess.TimeoutExpired):
                print("Certificate attempt failed; retry throttled:", name)
        ready = {name for name, hosts in entries.items() if certificate_ready(certroot / name, hosts)}
        install_config(render(entries, ready, certroot, BASE / "webroot"))
        active = {}
        for name in sorted(ready):
            try:
                for host in entries[name]:
                    probe(host)
                if name != "pw-platform":
                    active[entries[name][0]] = True
            except (OSError, RuntimeError):
                print("HTTPS probe pending:", name)
        atomic(BASE / "public/ready.json", json.dumps({"expires": int(time.time()) + 600, "hosts": active}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--production", action="store_true")
    parser.add_argument("--email", default="")
    parser.add_argument("--postgres-container", default="pagewright-pilot-postgres-1")
    try:
        reconcile(parser.parse_args())
    except Exception as error:
        # Avoid tracebacks or subprocess output containing deployment details.
        print("Pilot TLS reconciliation failed:", type(error).__name__)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
