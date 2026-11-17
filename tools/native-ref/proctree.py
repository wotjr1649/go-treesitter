"""Bounded tool commands; own their descendants until the command is reaped.

POSIX supervisors share one outer process group. An inner timeout kills only its
leader; its grandchildren are stopped when the outermost command ends.
"""

import ctypes
import os
import signal
import subprocess
import sys


if os.name != "nt":
    def _interrupt(signum, frame):
        raise KeyboardInterrupt()

    signal.signal(signal.SIGTERM, _interrupt)


if os.name == "nt":
    from ctypes import wintypes

    class _BasicLimits(ctypes.Structure):
        _fields_ = [("user_time", ctypes.c_longlong), ("job_time", ctypes.c_longlong),
                    ("flags", wintypes.DWORD), ("min_ws", ctypes.c_size_t),
                    ("max_ws", ctypes.c_size_t), ("active", wintypes.DWORD),
                    ("affinity", ctypes.c_size_t), ("priority", wintypes.DWORD),
                    ("scheduling", wintypes.DWORD)]

    class _ExtendedLimits(ctypes.Structure):
        _fields_ = [("basic", _BasicLimits), ("io", ctypes.c_ulonglong * 6),
                    ("process_memory", ctypes.c_size_t), ("job_memory", ctypes.c_size_t),
                    ("peak_process_memory", ctypes.c_size_t), ("peak_job_memory", ctypes.c_size_t)]

    class _ThreadEntry(ctypes.Structure):
        _fields_ = [("size", wintypes.DWORD), ("usage", wintypes.DWORD),
                    ("tid", wintypes.DWORD), ("pid", wintypes.DWORD),
                    ("priority", wintypes.LONG), ("delta", wintypes.LONG), ("flags", wintypes.DWORD)]

    _kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    _kernel.CreateJobObjectW.argtypes = [ctypes.c_void_p, wintypes.LPCWSTR]
    _kernel.CreateJobObjectW.restype = wintypes.HANDLE
    _kernel.SetInformationJobObject.argtypes = [wintypes.HANDLE, ctypes.c_int, ctypes.c_void_p, wintypes.DWORD]
    _kernel.SetInformationJobObject.restype = wintypes.BOOL
    _kernel.AssignProcessToJobObject.argtypes = [wintypes.HANDLE, wintypes.HANDLE]
    _kernel.AssignProcessToJobObject.restype = wintypes.BOOL
    _kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    _kernel.CloseHandle.restype = wintypes.BOOL
    _kernel.CreateToolhelp32Snapshot.argtypes = [wintypes.DWORD, wintypes.DWORD]
    _kernel.CreateToolhelp32Snapshot.restype = wintypes.HANDLE
    _kernel.Thread32First.argtypes = [wintypes.HANDLE, ctypes.POINTER(_ThreadEntry)]
    _kernel.Thread32First.restype = wintypes.BOOL
    _kernel.Thread32Next.argtypes = [wintypes.HANDLE, ctypes.POINTER(_ThreadEntry)]
    _kernel.Thread32Next.restype = wintypes.BOOL
    _kernel.OpenThread.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    _kernel.OpenThread.restype = wintypes.HANDLE
    _kernel.ResumeThread.argtypes = [wintypes.HANDLE]
    _kernel.ResumeThread.restype = wintypes.DWORD

    def _checked(result):
        if not result:
            raise ctypes.WinError(ctypes.get_last_error())
        return result

    def _resume(pid):
        # Popen closes the initial thread handle before returning to its caller.
        snapshot = _kernel.CreateToolhelp32Snapshot(4, 0)  # TH32CS_SNAPTHREAD
        if snapshot == ctypes.c_void_p(-1).value:
            raise ctypes.WinError(ctypes.get_last_error())
        try:
            entry = _ThreadEntry()
            entry.size = ctypes.sizeof(entry)
            found = _kernel.Thread32First(snapshot, ctypes.byref(entry))
            while found:
                if entry.pid == pid:
                    thread = _checked(_kernel.OpenThread(2, False, entry.tid))  # THREAD_SUSPEND_RESUME
                    try:
                        if _kernel.ResumeThread(thread) != 1:
                            raise OSError("unexpected initial thread suspend count")
                        return
                    finally:
                        _kernel.CloseHandle(thread)
                entry.size = ctypes.sizeof(entry)
                found = _kernel.Thread32Next(snapshot, ctypes.byref(entry))
            raise OSError(f"cannot find suspended thread of process {pid}")
        finally:
            _kernel.CloseHandle(snapshot)


def start(args, **kwargs):
    """Start contained before any child code can create grandchildren."""
    if os.name != "nt":
        provided = kwargs.pop("env", None)
        env = dict(os.environ if provided is None else provided)
        outermost = os.environ.get("TSG_TOOL_TREE") != "1"
        env["TSG_TOOL_TREE"] = "1"
        proc = None
        try:
            proc = subprocess.Popen(args, start_new_session=outermost, env=env, **kwargs)
            proc._tree_group = outermost
            return proc
        except BaseException:
            if proc is not None:
                try:
                    if outermost:
                        if proc.pid <= 1:
                            raise ValueError("invalid process group")
                        os.killpg(proc.pid, signal.SIGKILL)
                    else:
                        proc.kill()
                except ProcessLookupError:
                    pass
                proc.wait(timeout=10)
            raise
    job = _checked(_kernel.CreateJobObjectW(None, None))
    proc = None
    try:
        limits = _ExtendedLimits()
        limits.basic.flags = 0x2000  # JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
        _checked(_kernel.SetInformationJobObject(job, 9, ctypes.byref(limits), ctypes.sizeof(limits)))
        proc = subprocess.Popen(args, creationflags=0x4, **kwargs)  # CREATE_SUSPENDED
        _checked(_kernel.AssignProcessToJobObject(job, int(proc._handle)))
        _resume(proc.pid)
        proc._tree_job = job
        return proc
    except BaseException:
        _kernel.CloseHandle(job)
        if proc is not None:
            proc.kill()
            proc.wait(timeout=10)
        raise


def stop_tree(proc):
    """Also stop descendants whose direct parent has already exited."""
    if os.name == "nt":
        job = getattr(proc, "_tree_job", None)
        if job is not None:
            _checked(_kernel.CloseHandle(job))
            proc._tree_job = None
    else:
        # Every call kills again: descendants forked while an earlier group kill ran must still be reached.
        try:
            if proc._tree_group:
                if proc.pid <= 1:
                    raise ValueError("invalid process group")
                os.killpg(proc.pid, signal.SIGKILL)
            else:
                proc.kill()
        except ProcessLookupError:
            pass
        except PermissionError:
            # macOS reports EPERM, not ESRCH, for a group left with only zombies (our own descendants).
            if not (proc._tree_group and sys.platform == "darwin"):
                raise
    proc.wait(timeout=10)


def run(args, *, timeout=1800, check=False, capture_output=False, input=None, **kwargs):
    """The subprocess.run subset used by repository tools, with tree cleanup."""
    if timeout is None or timeout <= 0:
        raise ValueError("a positive command timeout is required")
    if capture_output:
        if "stdout" in kwargs or "stderr" in kwargs:
            raise ValueError("capture_output cannot be combined with stdout/stderr")
        kwargs.update(stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if input is not None:
        kwargs["stdin"] = subprocess.PIPE
    proc = start(list(map(str, args)), **kwargs)
    try:
        stdout, stderr = proc.communicate(input, timeout=timeout)
    except subprocess.TimeoutExpired as error:
        stop_tree(proc)
        error.output, error.stderr = proc.communicate(timeout=10)
        raise
    finally:
        stop_tree(proc)
    result = subprocess.CompletedProcess(args, proc.returncode, stdout, stderr)
    if check:
        result.check_returncode()
    return result
