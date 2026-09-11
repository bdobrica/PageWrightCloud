#!/usr/bin/env python3
"""Trusted-operator, offline, coordinated backups. Never reads Compose secrets.

No app starts, volume deletion, overwrites, image pulls, or in-place restores.
Docker daemon access is operator authority, not a public application endpoint.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
import tarfile
import time

VOLUMES = {"redis_data": ("redis", "/data"), "nfs_data": ("storage", "/nfs"),
           "www_data": ("serving", "/var/www"),
           "nginx_config": ("serving", "/etc/nginx/sites-enabled"),
           "pagewright_config": ("serving", "/etc/pagewright")}
FILES = {"postgres.dump", *(name + ".tar.gz" for name in VOLUMES)}
LIMIT = 100 * 1024**3
NAME = re.compile(r"[a-z0-9][a-z0-9_-]{0,62}\Z")
SHA = re.compile(r"[a-f0-9]{64}\Z")
IMAGE = re.compile(r"sha256:[a-f0-9]{64}\Z")


class Refused(Exception):
    pass


def require(condition, reason):
    if not condition:
        raise Refused(reason)


def run(args, *, stdin=None, stdout=None, timeout=120):
    # Never surface subprocess output: SQL/dump/daemon diagnostics can be private.
    try:
        result = subprocess.run(args, stdin=stdin, stdout=stdout or subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=timeout, check=True)
        return result.stdout.decode().strip() if stdout is None else None
    except (OSError, subprocess.SubprocessError, UnicodeError):
        raise Refused("operation failed; keep maintenance in place and retain partial output") from None


def inspect(cid):
    template = '{"id":{{json .Id}},"image":{{json .Image}},"state":{{json .State}},"labels":{{json .Config.Labels}},"mounts":{{json .Mounts}}}'
    return json.loads(run(["docker", "inspect", "--format", template, cid]))


def inventory(project, redis_running=False):
    require(NAME.fullmatch(project), "invalid explicit Compose project")
    ids = run(["docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+project]).split()
    require(ids, "project must already exist; no resources are created by this tool")
    services = {}
    for cid in ids:
        item = inspect(cid)
        service = item["labels"].get("com.docker.compose.service")
        require(service and service not in services, "one stopped instance per service required")
        require(item["state"]["Status"] in ("running", "exited", "created"), "unstable/paused/dead container")
        expected = service == "postgres" or (service == "redis" and redis_running)
        require(item["state"]["Running"] == expected, "stop every project service except required databases first")
        if service == "redis" and not redis_running:
            require(item["state"].get("ExitCode", 0) == 0, "Redis did not shut down cleanly")
        services[service] = item
    require({"postgres", "redis", "storage", "serving"} <= services.keys(), "missing database/storage/serving containers")
    volumes = {}
    for name, (service, destination) in {**VOLUMES, "postgres_data": ("postgres", "/var/lib/postgresql/data")}.items():
        mounts = [m for m in services[service]["mounts"] if m["Destination"] == destination]
        require(len(mounts) == 1 and mounts[0]["Type"] == "volume", "only named local volumes at standard paths are supported")
        volume = mounts[0]["Name"]
        info = json.loads(run(["docker", "volume", "inspect", volume]))[0]
        labels = info.get("Labels") or {}
        require(info["Driver"] == "local" and not info.get("Options"), "external/bind/network-backed volumes require a separate procedure")
        require(labels.get("com.docker.compose.project") == project and labels.get("com.docker.compose.volume") == name,
                "volume ownership does not match project and logical name")
        volumes[name] = volume
    require(len(set(volumes.values())) == len(volumes), "shared volume mapping rejected")
    # No other live container may write these volumes; do not inspect its secrets.
    for cid in run(["docker", "ps", "-q"]).split():
        item = inspect(cid)
        used = {m.get("Name") for m in item["mounts"] if m.get("RW")}
        if used.intersection(volumes.values()):
            require(item["id"] in {services[s]["id"] for s in ("postgres", "redis") if services[s]["state"]["Running"]},
                    "another running container writes a selected volume")
    networks = set(run(["docker", "network", "ls", "--filter", "label=com.docker.compose.project="+project,
                        "--format", "{{.Name}}"]).split())
    for cid in run(["docker", "ps", "-q", "--filter", "label=io.pagewright.role=worker"]).split():
        require(inspect(cid)["labels"].get("io.pagewright.network") not in networks, "drain project workers first")
    return services, volumes


def pg(services, user, db, command, *, stdin=None, stdout=None):
    return run(["docker", "exec", "-i", services["postgres"]["id"], *command,
                "-h", "/var/run/postgresql", "-U", user, "-d", db, "--no-password"],
               stdin=stdin, stdout=stdout, timeout=1200)


def sql(services, user, db, query):
    return pg(services, user, db, ["psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-c", query])


def idle_database(services, user, db):
    require(sql(services, user, db, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend'") == "0",
            "database still has client connections")


def helper(image, volume, command, *, writable=False, stdin=None, stdout=None):
    require(IMAGE.fullmatch(image), "helper must be a locally resolved image ID")
    mount = "type=volume,src="+volume+",dst=/volume,volume-nocopy" + ("" if writable else ",readonly")
    cid = run(["docker", "create", "-i", "--pull=never", "--network=none", "--read-only",
                "--security-opt=no-new-privileges", "--cap-drop=ALL", "--cap-add=DAC_OVERRIDE",
                *(["--cap-add=CHOWN", "--cap-add=FOWNER"] if writable else []),
                "--pids-limit=32", "--memory=256m", "--cpus=1", "--user=0:0", "--mount", mount,
                "--entrypoint", command[0], image, *command[1:]])
    require(re.fullmatch(r"[a-f0-9]{64}", cid), "invalid helper container identity")
    try:
        return run(["docker", "start", "--attach", "--interactive", cid], stdin=stdin, stdout=stdout, timeout=1200)
    finally:
        # Killing the attached Docker client alone does not reliably stop its
        # container. Remove only this exact returned helper, never its volume.
        run(["docker", "rm", "--force", cid])


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024*1024), b""):
            h.update(chunk)
    return h.hexdigest()


def validate_archive(path, logical):
    names, links, total = {}, {}, 0
    with tarfile.open(path, "r:gz") as archive:
        for member in archive:
            require(len(names) < 1000000, "archive has too many entries")
            raw = member.name.removeprefix("./").rstrip("/")
            if raw in ("", "."):
                require(member.isdir() and not member.mode & 0o6000, "invalid archive root")
                continue
            parts = raw.split("/")
            require(not raw.startswith("/") and all(p not in ("", ".", "..") for p in parts), "unsafe archive path")
            require(raw not in names and not member.mode & 0o6000, "duplicate path or special permissions")
            require(member.isfile() or member.isdir() or member.issym(), "special files and hardlinks are unsupported")
            require(member.size >= 0, "negative archive size")
            total += member.size
            require(total <= LIMIT, "unpacked archive exceeds limit")
            if member.issym():
                require(logical == "www_data" and len(parts) == 3 and parts[-1] in ("public", "preview")
                        and re.fullmatch(r"artifacts/[A-Za-z0-9][A-Za-z0-9._-]{0,199}/public", member.linkname),
                        "only managed relative publication symlinks are supported")
                links[raw] = str(PurePosixPath(raw).parent / member.linkname)
            require(not (logical == "nginx_config" and parts[-1] == ".pagewright-transaction"), "resolve Nginx recovery journal before backup")
            if logical == "www_data" and parts[-1] == ".deployment.json":
                require(member.isfile() and member.size <= 4096, "invalid deployment receipt")
                receipt = json.load(archive.extractfile(member))
                require(isinstance(receipt, dict) and receipt.get("status") in ("completed", "failed"), "resolve pending serving deployment first")
            names[raw] = member.isdir()
    for name in names:
        parent = PurePosixPath(name).parent
        while str(parent) != ".":
            require(str(parent) not in names or names[str(parent)], "archive traverses a non-directory")
            parent = parent.parent
    for target in links.values():
        require(names.get(target) is True, "publication symlink has no archived target directory")


def private_directory(path):
    require(not path.is_symlink() and path.is_dir(), "bundle must be a real directory")
    require(path.stat().st_mode & 0o077 == 0, "bundle directory must be private (0700)")


def verify(directory):
    private_directory(directory)
    manifest_path = directory / "manifest.json"
    require(manifest_path.is_file() and not manifest_path.is_symlink() and manifest_path.stat().st_size <= 65536,
            "missing or invalid completion manifest")
    require(not manifest_path.stat().st_mode & 0o077, "completion manifest must be private")
    manifest = json.loads(manifest_path.read_text())
    require(isinstance(manifest, dict), "manifest must be an object")
    require(manifest.get("format") == "pagewright-offline-backup-v1", "unsupported backup format")
    require(set(manifest.get("files", {})) == FILES, "incomplete or unexpected backup members")
    require(NAME.fullmatch(manifest.get("project", "")) and re.fullmatch(r"[a-f0-9]{40}", manifest.get("revision", "")), "invalid backup identity")
    require(set(manifest.get("images", {})) == {"postgres", "redis", "storage", "serving"}
            and all(IMAGE.fullmatch(v) for v in manifest["images"].values()), "invalid image identity")
    require(set(manifest.get("volumes", {})) == set(VOLUMES) | {"postgres_data"}, "missing volume provenance")
    require(set(p.name for p in directory.iterdir()) == FILES | {"manifest.json"}, "unexpected bundle files")
    for name, info in manifest["files"].items():
        path = directory / name
        st = path.lstat()
        require(stat.S_ISREG(st.st_mode) and not st.st_mode & 0o077 and 0 < st.st_size <= LIMIT, "unsafe backup file")
        require(st.st_size == info.get("bytes") and SHA.fullmatch(info.get("sha256", "")) and digest(path) == info["sha256"], "backup checksum mismatch")
        if name.endswith(".tar.gz"):
            validate_archive(path, name.removesuffix(".tar.gz"))
    with (directory / "postgres.dump").open("rb") as dump:
        require(dump.read(5) == b"PGDMP", "not a custom-format PostgreSQL dump")
    return manifest


REDIS_IDLE = """
if redis.call('LLEN','pagewright:queue')~=0 or redis.call('SCARD','pagewright:queue:active')~=0 or redis.call('HLEN','pagewright:queue:sites')~=0 then return 0 end
local cursor='0'; local count=0
repeat
 local result=redis.call('SCAN',cursor,'MATCH','pagewright:job:*','COUNT',256); cursor=result[1]
 for _,key in ipairs(result[2]) do
  count=count+1; if count>100000 then return 0 end
  local job=cjson.decode(redis.call('GET',key))
  if job.status~='completed' and job.status~='failed' then return 0 end
 end
until cursor=='0'
return 1
"""


def backup(args):
    require(args.quiesced, "explicit --quiesced maintenance acknowledgement required")
    require(re.fullmatch(r"[a-f0-9]{40}", args.revision or ""), "record the reviewed full Git revision")
    services, volumes = inventory(args.project, redis_running=True)
    idle_database(services, args.db_user, args.db_name)
    pending = sql(services, args.db_user, args.db_name, """SELECT
      (SELECT count(*) FROM build_submissions WHERE dispatch_state NOT IN ('accepted','rejected') OR status NOT IN ('completed','failed')) +
      (SELECT count(*) FROM deployments WHERE status='pending') +
      (SELECT count(*) FROM pilot_attempts WHERE busy) +
      (SELECT count(*) FROM pilot_provider_reservations WHERE active)""")
    require(pending == "0", "resolve active/uncertain builds, deployments and provider slots before backup")
    redis = ["docker", "exec", services["redis"]["id"], "redis-cli", "--raw"]
    require(run(redis + ["EVAL", REDIS_IDLE, "0"]) == "1", "Redis contains nonterminal work; drain/reconcile first")
    require(run(redis + ["CONFIG", "GET", "appendonly"]).splitlines() == ["appendonly", "yes"], "Redis AOF required")
    schema = sql(services, args.db_user, args.db_name, "SELECT max(version) FROM schema_migrations")
    args.directory.mkdir(mode=0o700)  # exclusive, never merge/overwrite
    private_directory(args.directory)
    run(["docker", "stop", "--time=30", services["redis"]["id"]], timeout=60)
    cold, after_volumes = inventory(args.project)
    require(volumes == after_volumes and all(cold[s]["id"] == services[s]["id"] for s in services), "project changed during shutdown")
    with (args.directory / "postgres.dump").open("xb") as out:
        pg(cold, args.db_user, args.db_name, ["pg_dump", "--format=custom", "--no-owner", "--no-acl"], stdout=out)
    for name in VOLUMES:
        with (args.directory / (name+".tar.gz")).open("xb") as out:
            helper(cold["postgres"]["image"], volumes[name], ["tar", "-czf", "-", "-C", "/volume", "."], stdout=out)
    final, final_volumes = inventory(args.project)
    require(final_volumes == volumes and all(final[s]["id"] == cold[s]["id"] for s in cold), "project changed during backup")
    idle_database(final, args.db_user, args.db_name)
    for name in VOLUMES:
        validate_archive(args.directory / (name+".tar.gz"), name)
    for name in FILES:
        with (args.directory / name).open("rb") as member:
            os.fsync(member.fileno())
    manifest = {"format": "pagewright-offline-backup-v1", "project": args.project,
                "revision": args.revision, "created_at_unix": int(time.time()), "schema_version": schema,
                "database": args.db_name, "volumes": volumes,
                "images": {s: cold[s]["image"] for s in ("postgres", "redis", "storage", "serving")},
                "files": {name: {"bytes": (args.directory/name).stat().st_size, "sha256": digest(args.directory/name)} for name in sorted(FILES)}}
    # Completion marker is written last; failed output is retained, never restored.
    with (args.directory / "manifest.json").open("x") as out:
        json.dump(manifest, out, indent=2)
        out.flush(); os.fsync(out.fileno())
    directory_fd = os.open(args.directory, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(directory_fd)
    finally:
        os.close(directory_fd)
    verify(args.directory)


def restore(args):
    require(args.trust_backup, "--trust-backup required: SQL and Nginx configuration are executable trusted input")
    manifest = verify(args.directory)  # validate every byte before contacting target
    require(args.project != manifest["project"], "in-place/source-project restore is forbidden")
    require(args.revision == manifest["revision"], "restore with the recorded application revision")
    services, volumes = inventory(args.project)
    require(not set(volumes.values()).intersection(manifest["volumes"].values()), "source volumes cannot be restore targets")
    require({s: services[s]["image"] for s in manifest["images"]} == manifest["images"], "restore exact recorded service image IDs first")
    require(args.db_name == manifest["database"], "restore the recorded database name")
    idle_database(services, args.db_user, args.db_name)
    require(sql(services, args.db_user, args.db_name, "SELECT count(*) FROM pg_namespace WHERE nspname !~ '^pg_' AND nspname <> 'information_schema' AND nspname <> 'public'") == "0", "target database has custom schemas")
    require(sql(services, args.db_user, args.db_name, "SELECT count(*) FROM pg_class WHERE relnamespace='public'::regnamespace") == "0", "target database is not empty")
    require(sql(services, args.db_user, args.db_name, "SELECT (SELECT count(*) FROM pg_proc WHERE pronamespace='public'::regnamespace)+(SELECT count(*) FROM pg_type WHERE typnamespace='public'::regnamespace)") == "0", "target database has existing functions/types")
    for name in VOLUMES:
        try:
            helper(services["postgres"]["image"], volumes[name], ["sh", "-ec", 'test -z "$(ls -A /volume)"'])
        except Refused:
            raise Refused("target volume "+name+" is nonempty or cannot be inspected; create fresh volumes with nocopy") from None
    # Partial restore is deliberately not retryable in place. Retain for inspection
    # and create another fresh target; no delete/clean/overwrite switch exists.
    for name in VOLUMES:
        with (args.directory / (name+".tar.gz")).open("rb") as source:
            helper(services["postgres"]["image"], volumes[name], ["tar", "-xzpf", "-", "-C", "/volume"], writable=True, stdin=source)
    with (args.directory / "postgres.dump").open("rb") as source:
        pg(services, args.db_user, args.db_name, ["pg_restore", "--single-transaction", "--exit-on-error", "--no-owner", "--no-acl"], stdin=source)
    final, final_volumes = inventory(args.project)
    require(final_volumes == volumes and all(final[s]["id"] == services[s]["id"] for s in services), "target changed during restore")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("backup", "verify", "restore"))
    parser.add_argument("--directory", type=Path, required=True)
    parser.add_argument("--project")
    parser.add_argument("--revision")
    parser.add_argument("--db-name", default="pagewright")
    parser.add_argument("--db-user", default="pagewright")
    parser.add_argument("--quiesced", action="store_true")
    parser.add_argument("--trust-backup", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    try:
        require(args.directory.is_absolute(), "use an absolute private backup path")
        if args.action == "verify":
            verify(args.directory)
        else:
            require(args.project and NAME.fullmatch(args.project), "explicit --project required")
            require(NAME.fullmatch(args.db_name) and NAME.fullmatch(args.db_user), "invalid database identity")
            (backup if args.action == "backup" else restore)(args)
        print(args.action+" verified; application services remain stopped")
    except (Refused, OSError, ValueError, TypeError, KeyError, tarfile.TarError) as exc:
        print("Backup operation refused: "+(str(exc) if isinstance(exc, Refused) else "invalid or inaccessible input"), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
