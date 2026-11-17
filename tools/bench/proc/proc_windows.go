package proc

import (
	"encoding/binary"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var createJob = kernel32.NewProc("CreateJobObjectW")
var setJob = kernel32.NewProc("SetInformationJobObject")
var assignJob = kernel32.NewProc("AssignProcessToJobObject")
var threadFirst = kernel32.NewProc("Thread32First")
var threadNext = kernel32.NewProc("Thread32Next")
var openThread = kernel32.NewProc("OpenThread")
var resumeThread = kernel32.NewProc("ResumeThread")

type threadEntry struct {
	Size, Usage, ID, Owner      uint32
	BasePriority, DeltaPriority int32
	Flags                       uint32
}

// os/exec closes the initial thread handle, so enumerate the one suspended thread.
func resume(pid uint32) error {
	snapshot, err := syscall.CreateToolhelp32Snapshot(4, 0) // TH32CS_SNAPTHREAD
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(snapshot)
	entry := threadEntry{Size: uint32(unsafe.Sizeof(threadEntry{}))}
	ok, _, callErr := threadFirst.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		if entry.Owner == pid {
			handle, _, err := openThread.Call(2, 0, uintptr(entry.ID)) // THREAD_SUSPEND_RESUME
			if handle == 0 {
				return err
			}
			defer syscall.CloseHandle(syscall.Handle(handle))
			count, _, err := resumeThread.Call(handle)
			if uint32(count) == ^uint32(0) {
				return err
			}
			if count != 1 {
				return fmt.Errorf("unexpected initial suspend count %d", count)
			}
			return nil
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		ok, _, callErr = threadNext.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	}
	return fmt.Errorf("find suspended thread of %d: %w", pid, callErr)
}

func prepare(cmd *exec.Cmd) (func() error, func() error, error) {
	handle, _, err := createJob.Call(0, 0)
	if handle == 0 {
		return nil, nil, fmt.Errorf("CreateJobObject: %w", err)
	}
	stop := func() error { return syscall.CloseHandle(syscall.Handle(handle)) }
	// JOBOBJECT_EXTENDED_LIMIT_INFORMATION has 8-byte alignment even on Windows/386.
	var limits [144]byte
	binary.LittleEndian.PutUint32(limits[16:], 0x2000) // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	size := uintptr(144)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		size = 112
	}
	ok, _, err := setJob.Call(handle, 9, uintptr(unsafe.Pointer(&limits[0])), size)
	if ok == 0 {
		stop()
		return nil, nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= 4 // CREATE_SUSPENDED closes the spawn-before-assignment race.
	attach := func() error {
		process, err := syscall.OpenProcess(0x0101, false, uint32(cmd.Process.Pid)) // SET_QUOTA | TERMINATE
		if err != nil {
			return err
		}
		defer syscall.CloseHandle(process)
		ok, _, err := assignJob.Call(handle, uintptr(process))
		if ok == 0 {
			return fmt.Errorf("AssignProcessToJobObject: %w", err)
		}
		return resume(uint32(cmd.Process.Pid))
	}
	return stop, attach, nil
}
