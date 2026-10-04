//go:build windows

package sessions

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listProcesses walks the process table and asks each process for its full
// program path, how much memory it holds and how much processor time it has
// used. A process whose path cannot be read is listed by its file name alone
// (Proc.Name): a game protected against tampering keeps its path from
// everyone, and matching by name within its install folder's programs is
// still exact enough.
func listProcesses() ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	var out []Proc
	buf := make([]uint16, windows.MAX_LONG_PATH)
	for {
		if pid := entry.ProcessID; pid > 4 {
			if p, ok := inspect(pid, buf); ok {
				out = append(out, p)
			} else if name := windows.UTF16ToString(entry.ExeFile[:]); name != "" {
				// Its path is not ours to read: a game protected against
				// tampering, run elevated, or another user's. Known by name.
				out = append(out, Proc{PID: int(pid), Name: name})
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out, nil
}

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// processMemoryCounters is PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func inspect(pid uint32, buf []uint16) (Proc, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Proc{}, false
	}
	defer windows.CloseHandle(h)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return Proc{}, false
	}
	p := Proc{PID: int(pid), Exe: windows.UTF16ToString(buf[:n])}

	var creation, exit, kernel, user windows.Filetime
	timesOK := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user) == nil
	var mem processMemoryCounters
	mem.cb = uint32(unsafe.Sizeof(mem))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mem)), uintptr(mem.cb))
	if timesOK && r != 0 {
		p.Measured = true
		p.Memory = uint64(mem.WorkingSetSize)
		// FILETIME counts 100-nanosecond intervals.
		ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) +
			(uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
		p.CPU = time.Duration(ticks * 100)
	}
	return p, true
}
