import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import pilot_backup as backup


class BackupTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name)

    def tar(self, entries, name="www_data"):
        path = self.directory / (name+".tar.gz")
        with tarfile.open(path, "w:gz") as out:
            for entry in entries:
                info = tarfile.TarInfo(entry[0])
                kind = entry[1]
                info.mode = 0o755 if kind == "dir" else 0o644
                if kind == "dir":
                    info.type = tarfile.DIRTYPE
                elif kind == "link":
                    info.type = tarfile.SYMTYPE; info.linkname = entry[2]
                elif kind == "hardlink":
                    info.type = tarfile.LNKTYPE; info.linkname = entry[2]
                elif kind == "device":
                    info.type = tarfile.CHRTYPE
                else:
                    info.size = len(entry[2])
                out.addfile(info, io.BytesIO(entry[2]) if kind == "file" else None)
        path.chmod(0o600)
        return path

    def bundle(self):
        for logical in backup.VOLUMES:
            self.tar([(".", "dir")], logical)
        (self.directory/"postgres.dump").write_bytes(b"PGDMPtest fixture")
        (self.directory/"postgres.dump").chmod(0o600)
        manifest = {"format": "pagewright-offline-backup-v1", "revision": "a"*40, "project": "source",
                    "database": "pagewright", "volumes": {k: "source_"+k for k in (*backup.VOLUMES, "postgres_data")},
                    "images": {k: "sha256:"+"b"*64 for k in ("postgres", "redis", "storage", "serving")},
                    "files": {f: {"bytes": (self.directory/f).stat().st_size, "sha256": backup.digest(self.directory/f)} for f in backup.FILES}}
        (self.directory/"manifest.json").write_text(json.dumps(manifest))
        (self.directory/"manifest.json").chmod(0o600)
        return manifest

    def test_valid_bundle(self):
        self.bundle()
        backup.verify(self.directory)

    def test_corruption_missing_and_extra_members(self):
        for scenario in ("corrupt", "missing", "extra", "symlink", "public"):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as tmp:
                self.directory = Path(tmp); self.bundle()
                path = self.directory/"postgres.dump"
                if scenario == "corrupt": path.write_bytes(b"PGDMPbad")
                elif scenario == "missing": path.unlink()
                elif scenario == "extra": (self.directory/"unexpected").touch()
                elif scenario == "public": path.chmod(0o644)
                else: path.unlink(); path.symlink_to("manifest.json")
                with self.assertRaises((backup.Refused, OSError)):
                    backup.verify(self.directory)

    def test_incomplete_manifest_never_accepted(self):
        with self.assertRaises(backup.Refused): backup.verify(self.directory)

    def test_managed_symlinks_preserved(self):
        path = self.tar([("example.test/site.example.test/artifacts/v1/public", "dir"),
                         ("example.test/site.example.test/public", "link", "artifacts/v1/public")])
        backup.validate_archive(path, "www_data")

    def test_unsafe_archives_rejected(self):
        cases = [ [("../escape", "file", b"x")], [("/absolute", "file", b"x")],
                  [("x", "hardlink", "../x")], [("x", "device")],
                  [("x", "file", b"x"), ("x/child", "file", b"x")],
                  [("x", "file", b"x"), ("x", "file", b"y")],
                  [("d/site/public", "link", "../../escape")],
                  [("d/site/public", "link", "artifacts/v1/public")],
                  [("d/site/.deployment.json", "file", b'{"status":"pending"}')]]
        for entries in cases:
            with self.subTest(entries=entries), self.assertRaises(backup.Refused):
                backup.validate_archive(self.tar(entries), "www_data")
        with self.assertRaises(backup.Refused):
            backup.validate_archive(self.tar([(".pagewright-transaction", "file", b"pending")], "nginx_config"), "nginx_config")

    def test_restore_validates_before_contacting_docker(self):
        args = argparse.Namespace(directory=self.directory, trust_backup=True, project="target", revision="a"*40)
        with patch.object(backup, "inventory") as inventory:
            with self.assertRaises(backup.Refused): backup.restore(args)
            inventory.assert_not_called()
        self.bundle(); args.project="source"
        with patch.object(backup, "inventory") as inventory:
            with self.assertRaises(backup.Refused): backup.restore(args)
            inventory.assert_not_called()

    def test_helper_isolated_readonly_source_and_no_privileged(self):
        with patch.object(backup, "run", side_effect=["c"*64, "", ""]) as run:
            backup.helper("sha256:"+"b"*64,"target_www_data",["tar","-czf","-","-C","/volume","."])
            command = run.call_args_list[0].args[0]
            for value in ("--network=none", "--read-only", "--cap-drop=ALL", "--pull=never"):
                self.assertIn(value, command)
            self.assertIn("type=volume,src=target_www_data,dst=/volume,volume-nocopy,readonly",command)
            self.assertNotIn("--privileged", command)
            self.assertFalse(any("docker.sock" in arg for arg in command))
            self.assertEqual(run.call_args.args[0], ["docker", "rm", "--force", "c"*64])

    def test_failed_helper_is_reaped_without_deleting_volume(self):
        with patch.object(backup, "run", side_effect=["c"*64, backup.Refused("timeout"), ""]) as run:
            with self.assertRaises(backup.Refused):
                backup.helper("sha256:"+"b"*64, "target_www_data", ["tar","-xf","-"], writable=True)
            self.assertEqual(run.call_args.args[0], ["docker", "rm", "--force", "c"*64])

    def test_unsettled_work_rejected_before_redis_stop(self):
        args = argparse.Namespace(project="source", directory=self.directory/"new", revision="a"*40,
                                  db_user="pagewright", db_name="pagewright", quiesced=True)
        with patch.object(backup, "inventory", return_value=({}, {})), patch.object(backup, "idle_database"), \
             patch.object(backup, "sql", return_value="1"), patch.object(backup,"run") as run:
            with self.assertRaises(backup.Refused): backup.backup(args)
            run.assert_not_called()
            self.assertFalse(args.directory.exists())

    def test_requires_maintenance_acknowledgement(self):
        with patch.object(backup, "inventory") as inventory:
            with self.assertRaises(backup.Refused): backup.backup(argparse.Namespace(quiesced=False))
            inventory.assert_not_called()

    def test_inventory_fails_closed_for_unsafe_resources(self):
        for scenario in ("valid", "live-app", "wrong-owner", "bind", "foreign-writer", "worker"):
            with self.subTest(scenario=scenario):
                records = {}
                for service in ("postgres", "redis", "storage", "serving"):
                    running = service in ("postgres", "redis") or (scenario == "live-app" and service == "storage")
                    records[service] = {"id":service,"image":"sha256:"+"b"*64,
                        "state":{"Status":"running" if running else "exited","Running":running,"ExitCode":0},
                        "labels":{"com.docker.compose.service":service},"mounts":[]}
                mapping = {**backup.VOLUMES,"postgres_data":("postgres","/var/lib/postgresql/data")}
                for name,(service,destination) in mapping.items():
                    records[service]["mounts"].append({"Type":"bind" if scenario=="bind" else "volume",
                        "Name":"source_"+name,"Destination":destination,"RW":True})
                records["foreign"]={"id":"foreign","labels":{},"mounts":[{"RW":True,"Name":"source_www_data"}]}
                records["worker"]={"labels":{"io.pagewright.network":"source_default"},"mounts":[],"id":"worker"}
                def fake_run(cmd, **kwargs):
                    if cmd[:3]==["docker","volume","inspect"]:
                        return json.dumps([{"Driver":"local","Options":{},"Labels":{
                            "com.docker.compose.project":"wrong" if scenario=="wrong-owner" else "source",
                            "com.docker.compose.volume":cmd[3].removeprefix("source_")}}])
                    if cmd[:3]==["docker","network","ls"]: return "source_default"
                    if "label=io.pagewright.role=worker" in cmd: return "worker" if scenario=="worker" else ""
                    if "-aq" in cmd: return "postgres redis storage serving"
                    return "postgres redis"+(" foreign" if scenario=="foreign-writer" else "")
                with patch.object(backup,"run",side_effect=fake_run), patch.object(backup,"inspect",side_effect=lambda cid:records[cid]):
                    if scenario=="valid": backup.inventory("source",redis_running=True)
                    else:
                        with self.assertRaises(backup.Refused): backup.inventory("source",redis_running=True)


if __name__ == "__main__":
    unittest.main()
