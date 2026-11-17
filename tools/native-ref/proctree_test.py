import ctypes
import json
import os
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import Mock, PropertyMock, patch

import proctree


def process_alive(pid):
    if os.name == "nt":
        kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel.OpenProcess.argtypes = [ctypes.c_ulong, ctypes.c_int, ctypes.c_ulong]
        kernel.OpenProcess.restype = ctypes.c_void_p
        kernel.WaitForSingleObject.argtypes = [ctypes.c_void_p, ctypes.c_ulong]
        kernel.CloseHandle.argtypes = [ctypes.c_void_p]
        handle = kernel.OpenProcess(0x100000, False, pid)
        if not handle:
            return False
        try:
            return kernel.WaitForSingleObject(handle, 0) == 258
        finally:
            kernel.CloseHandle(handle)
    try:
        os.kill(pid, 0)
        stat = Path(f"/proc/{pid}/stat")
        return not stat.exists() or stat.read_text().split(")", 1)[1].split()[0] != "Z"
    except ProcessLookupError:
        return False


def tree_command(pidfile, exit_parent=False):
    source = ("import json,os,pathlib,subprocess,sys,time; "
              "child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)']); "
              "ready=pathlib.Path(sys.argv[1]+'.tmp'); "
              "ready.write_text(json.dumps([os.getpid(),child.pid])); ready.replace(sys.argv[1]); "
              + ("sys.exit(0)" if exit_parent else "time.sleep(60)"))
    return [sys.executable, "-c", source, str(pidfile)]


def assert_stopped(test, pidfile):
    pids = json.loads(pidfile.read_text())
    deadline = time.monotonic() + 5
    while any(process_alive(pid) for pid in pids) and time.monotonic() < deadline:
        time.sleep(0.02)
    test.assertEqual([pid for pid in pids if process_alive(pid)], [], f"live tree: {pids}")


class ProcessTreeTest(unittest.TestCase):
    def test_default_timeout_and_explicit_longer_timeout_are_preserved(self):
        for kwargs, expected in (({}, 1800), ({"timeout": 3600}, 3600)):
            with self.subTest(kwargs=kwargs):
                proc = Mock(returncode=0)
                proc.communicate.return_value = (b"out", b"")
                with patch.object(proctree, "start", return_value=proc), patch.object(proctree, "stop_tree"):
                    proctree.run(["unused"], **kwargs)
                proc.communicate.assert_called_once_with(None, timeout=expected)

    def test_posix_marker_keeps_nested_commands_in_the_outer_group(self):
        for inherited, outermost in (("", True), ("1", False)):
            with self.subTest(inherited=inherited), patch.object(proctree.os, "name", "posix"), \
                    patch.dict(os.environ, {"TSG_TOOL_TREE": inherited}), \
                    patch.object(subprocess, "Popen") as spawn, \
                    patch.object(os, "killpg", create=True) as killpg, \
                    patch.object(signal, "SIGKILL", 9, create=True):
                spawn.return_value.pid = 1234
                proc = proctree.start(["unused"], env={"EXPLICIT": "yes"})
                spawn.assert_called_once_with(["unused"], start_new_session=outermost,
                                              env={"EXPLICIT": "yes", "TSG_TOOL_TREE": "1"})
                proctree.stop_tree(proc)
                if outermost:
                    killpg.assert_called_once_with(1234, signal.SIGKILL)
                    proc.kill.assert_not_called()
                else:
                    killpg.assert_not_called()
                    proc.kill.assert_called_once_with()
                proc.wait.assert_called_once_with(timeout=10)

    def test_posix_start_interruption_stops_the_started_process(self):
        for inherited, outermost in (("", True), ("1", False)):
            proc = Mock(pid=1234)
            type(proc)._tree_group = PropertyMock(side_effect=KeyboardInterrupt)
            with self.subTest(inherited=inherited), patch.object(proctree.os, "name", "posix"), \
                    patch.dict(os.environ, {"TSG_TOOL_TREE": inherited}), \
                    patch.object(subprocess, "Popen", return_value=proc), \
                    patch.object(os, "killpg", create=True) as killpg, \
                    patch.object(signal, "SIGKILL", 9, create=True):
                with self.assertRaises(KeyboardInterrupt):
                    proctree.start(["unused"])
                if outermost:
                    killpg.assert_called_once_with(1234, signal.SIGKILL)
                    proc.kill.assert_not_called()
                else:
                    killpg.assert_not_called()
                    proc.kill.assert_called_once_with()
                proc.wait.assert_called_once_with(timeout=10)

    @unittest.skipIf(os.name == "nt", "POSIX process groups and SIGTERM")
    def test_sigterm_to_supervisor_reaps_child_and_grandchild(self):
        with tempfile.TemporaryDirectory() as tmp:
            pids = Path(tmp) / "pids.json"
            source = ("import sys; sys.path.insert(0,sys.argv[1]); import proctree; "
                      "proctree.run(sys.argv[2:],timeout=30)")
            env = dict(os.environ)
            env.pop("TSG_TOOL_TREE", None)
            supervisor = subprocess.Popen([sys.executable, "-c", source, str(Path(__file__).parent),
                                           *tree_command(pids)], env=env,
                                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                deadline = time.monotonic() + 5
                while not pids.exists() and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(pids.exists(), "supervisor did not start its tree")
                supervisor.send_signal(signal.SIGTERM)
                self.assertNotEqual(supervisor.wait(timeout=10), 0)
                assert_stopped(self, pids)
            finally:
                if supervisor.poll() is None:
                    supervisor.kill()
                supervisor.wait(timeout=10)
                if pids.exists():
                    try:
                        os.killpg(json.loads(pids.read_text())[0], signal.SIGKILL)
                    except ProcessLookupError:
                        pass

    def test_nested_helper_timeout_reaps_all_three_processes(self):
        with tempfile.TemporaryDirectory() as tmp:
            pids = Path(tmp) / "nested-pids.json"
            leaf = ("import json,os,pathlib,subprocess,sys,time; "
                    "child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)']); "
                    "ready=pathlib.Path(sys.argv[1]+'.tmp'); "
                    "ready.write_text(json.dumps([int(sys.argv[2]),os.getpid(),child.pid])); ready.replace(sys.argv[1]); "
                    "time.sleep(60)")
            supervisor = ("import os,sys; sys.path.insert(0,sys.argv[1]); import proctree; "
                          "proctree.run([sys.executable,'-c',sys.argv[2],sys.argv[3],str(os.getpid())],timeout=30)")
            command = [sys.executable, "-c", supervisor, str(Path(__file__).parent), leaf, str(pids)]
            with self.assertRaises(subprocess.TimeoutExpired):
                proctree.run(command, capture_output=True, timeout=2)
            self.assertEqual(len(json.loads(pids.read_text())), 3)
            assert_stopped(self, pids)

    def test_timeout_reaps_child_and_grandchild_with_inherited_pipes(self):
        with tempfile.TemporaryDirectory() as tmp:
            pids = Path(tmp) / "pids.json"
            with self.assertRaises(subprocess.TimeoutExpired):
                proctree.run(tree_command(pids), capture_output=True, timeout=1)
            assert_stopped(self, pids)

    def test_parent_exit_with_inherited_pipes_waits_until_timeout(self):
        with tempfile.TemporaryDirectory() as tmp:
            pids = Path(tmp) / "pids.json"
            # communicate has its own finite wait even when only a descendant owns the pipe.
            with self.assertRaises(subprocess.TimeoutExpired):
                proctree.run(tree_command(pids, exit_parent=True), capture_output=True, timeout=1)
            assert_stopped(self, pids)

    def test_interrupt_reaps_the_whole_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            pids = Path(tmp) / "pids.json"
            def interrupted(proc, *args, **kwargs):
                deadline = time.monotonic() + 5
                while not pids.exists() and time.monotonic() < deadline:
                    time.sleep(0.01)
                raise KeyboardInterrupt()

            with patch.object(subprocess.Popen, "communicate", interrupted):
                with self.assertRaises(KeyboardInterrupt):
                    proctree.run(tree_command(pids), timeout=10)
            assert_stopped(self, pids)

    def test_output_and_exit_status_are_preserved(self):
        result = proctree.run([sys.executable, "-c", "import sys; print('out'); sys.exit(7)"],
                              capture_output=True, text=True, timeout=10)
        self.assertEqual((result.returncode, result.stdout), (7, "out\n"))
        with self.assertRaises(subprocess.CalledProcessError):
            proctree.run([sys.executable, "-c", "raise SystemExit(3)"], check=True, timeout=10)


if __name__ == "__main__":
    unittest.main()
