#!/usr/bin/env python3
"""Opt-in local disposable restore drill. Never uses the pilot or an app .env."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import uuid

import pilot_backup as backup


def main():
    os.umask(0o077)
    root = Path(__file__).resolve().parent.parent
    token = uuid.uuid4().hex[:12]
    source, target = "pw-backup-src-"+token, "pw-backup-dst-"+token
    work = Path(tempfile.mkdtemp(prefix="pagewright-backup-drill-"))
    revision = backup.run(["git", "-C", str(root), "rev-parse", "HEAD"])

    def compose(project, *args, capture=False, stdin=None):
        command = ["docker", "compose", "--env-file", "/dev/null", "-p", project,
                   "-f", str(root/"docker-compose.backup-test.yaml"), "--profile", "test", *args]
        result = subprocess.run(command, cwd=root, check=True, stdin=stdin,
                                stdout=subprocess.PIPE if capture else None, timeout=1200)
        return result.stdout if capture else None

    args = argparse.Namespace(project=source, directory=work/"bundle", revision=revision,
                              db_user="pagewright", db_name="pagewright", quiesced=True, trust_backup=True)
    try:
        # Parse the actual operator overlay using fixture-only interpolation,
        # never the caller's environment or a checkout's .env.
        env = {"PAGEWRIGHT_POSTGRES_PASSWORD": "restore-config-test-only",
               "PAGEWRIGHT_REDIS_PASSWORD": "restore-redis-test-only",
               "PAGEWRIGHT_SERVICE_TOKEN": "restore-internal-config-test-only",
               "PAGEWRIGHT_JWT_SECRET": "restore-jwt-config-test-only"}
        cfg = json.loads(subprocess.check_output(["docker", "compose", "--env-file", "/dev/null",
                "-p", target, "-f", str(root/"docker-compose.yaml"), "-f", str(root/"docker-compose.restore.yaml"),
                "config", "--format", "json"], env=env, timeout=30))
        assert all(not s.get("ports") for s in cfg["services"].values())
        for service in ("gateway", "manager"):
            assert cfg["services"][service]["entrypoint"] == ["/bin/false"]
        assert not cfg["services"]["manager"].get("volumes")
        for logical, (service, destination) in backup.VOLUMES.items():
            mounts = [m for m in cfg["services"][service]["volumes"] if m["target"] == destination]
            assert len(mounts) == 1 and mounts[0]["volume"]["nocopy"]
        print("PASS recovery overlay: no published ports/dispatch/socket and empty-volume creation", flush=True)
        compose(source,"build")
        compose(source,"up","-d","--wait","postgres","redis","storage","serving","nginx")
        evidence = compose(source,"run","--rm","-T","--no-deps","fixture","seed",capture=True)
        json.loads(evidence)  # no fabricated acceptance if the fixture failed
        (work/"evidence.json").write_bytes(evidence)
        redis_values = {"pagewright:job:backup-fixture": '{"job_id":"backup-fixture","status":"completed"}',
                        "fence:site:backup-fixture": "42", "pagewright:queue:commits:backup-fixture": "fixture-digest"}
        for key,value in redis_values.items():
            compose(source,"exec","-T","redis","redis-cli","SET",key,value,capture=True)
        try:
            backup.backup(args)
        except backup.Refused:
            print("PASS backup refuses live application writers", flush=True)
        else:
            raise AssertionError("live backup was accepted")
        compose(source,"stop","nginx","serving","storage")
        backup.backup(args)
        backup.verify(args.directory)
        print("PASS coordinated bundle and all checksums", flush=True)
        # Prove the restored site cannot read any original application volume.
        compose(source,"down","--volumes","--remove-orphans")
        compose(target,"create","postgres","redis","storage","serving","nginx")
        compose(target,"up","-d","--wait","postgres")
        args.project = target
        backup.restore(args)
        try:
            backup.restore(args)
        except backup.Refused:
            print("PASS populated restore target refused", flush=True)
        else:
            raise AssertionError("restore overwrote existing state")
        compose(target,"up","-d","--wait","redis","storage","serving","nginx")
        for key,value in redis_values.items():
            actual = compose(target,"exec","-T","redis","redis-cli","--raw","GET",key,capture=True).decode().strip()
            if actual != value:
                raise AssertionError("Redis reservation/fence evidence not restored")
        print("PASS Redis terminal reservation, commit identity and fence retained", flush=True)
        with (work/"evidence.json").open("rb") as data:
            compose(target,"run","--rm","-T","--no-deps","fixture","check",stdin=data)
        print("PASS restore drill; private fixture evidence retained at "+str(work), flush=True)
    finally:
        # Targets were generated in this process; never accept caller-supplied
        # project names for destructive test cleanup.
        for project in (source,target):
            compose(project,"down","--volumes","--remove-orphans")


if __name__ == "__main__":
    main()
