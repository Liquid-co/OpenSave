//go:build windows

package sessions

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// listProcesses walks the process table and asks each process for its full
// program path. A process this user may not look at (a service, another
// user's) is skipped: no game of theirs is one of ours.
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
			if exe := imagePath(pid, buf); exe != "" {
				out = append(out, Proc{PID: int(pid), Exe: exe})
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out, nil
}

func imagePath(pid uint32, buf []uint16) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
