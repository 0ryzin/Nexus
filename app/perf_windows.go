//go:build windows

package main

import (
	"encoding/binary"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pdh                         = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery            = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter    = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData     = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedArray    = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procGetSystemTimes          = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")
	procGlobalMemoryStatusExPer = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	procGetSystemFirmwareTable  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemFirmwareTable")
)

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

type sysSampler struct {
	once                sync.Once
	idle, kernel, user  uint64
	haveCPU             bool
	query               uintptr
	gpuCounter, vramCtr uintptr
	pdhOK               bool
	nvmlOK              bool
	nvmlDev             uintptr
	nvmlTemp            *windows.LazyProc
	ramSpeed            float64
	vramMB              float64
	notes               []string
}

func ftU64(ft windows.Filetime) uint64 { return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime) }

func (s *sysSampler) init() {
	// GPU counters (Windows 10 1709+, every vendor)
	var q uintptr
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&q))); r == 0 {
		s.query = q
		a, _ := windows.UTF16PtrFromString(`\GPU Engine(*engtype_3D)\Utilization Percentage`)
		b, _ := windows.UTF16PtrFromString(`\GPU Adapter Memory(*)\Dedicated Usage`)
		r1, _, _ := procPdhAddEnglishCounter.Call(q, uintptr(unsafe.Pointer(a)), 0, uintptr(unsafe.Pointer(&s.gpuCounter)))
		r2, _, _ := procPdhAddEnglishCounter.Call(q, uintptr(unsafe.Pointer(b)), 0, uintptr(unsafe.Pointer(&s.vramCtr)))
		s.pdhOK = r1 == 0 && r2 == 0
		procPdhCollectQueryData.Call(q) // first sample (rates need two)
	}
	// NVIDIA: GPU temperature through NVML (installed with the NVIDIA driver)
	nvml := windows.NewLazySystemDLL("nvml.dll")
	if nvml.Load() == nil {
		initp, get := nvml.NewProc("nvmlInit_v2"), nvml.NewProc("nvmlDeviceGetHandleByIndex_v2")
		if r, _, _ := initp.Call(); r == 0 {
			if r, _, _ := get.Call(0, uintptr(unsafe.Pointer(&s.nvmlDev))); r == 0 {
				s.nvmlOK, s.nvmlTemp = true, nvml.NewProc("nvmlDeviceGetTemperature")
			}
		}
	}
	s.ramSpeed = smbiosMemorySpeed()
	if hw := gameGPU(detectHardwareCached()); hw != nil && hw.DedicatedMB > 0 {
		s.vramMB = float64(hw.DedicatedMB)
	}
}

func detectHardwareCached() Hardware {
	hwMu.Lock()
	defer hwMu.Unlock()
	if hwCache != nil {
		return *hwCache
	}
	return Hardware{}
}

// pdhArray sums a counter's instances per adapter (instance names contain "luid_0x…_0x…") and
// returns the largest adapter total.
func (s *sysSampler) pdhArray(counter uintptr) (float64, bool) {
	var size, count uint32
	r, _, _ := procPdhGetFormattedArray.Call(counter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		return 0, false
	}
	buf := make([]byte, size)
	r, _, _ = procPdhGetFormattedArray.Call(counter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return 0, false
	}
	type item struct {
		name   *uint16
		status uint32
		_      uint32
		value  float64
	}
	items := unsafe.Slice((*item)(unsafe.Pointer(&buf[0])), count)
	per := map[string]float64{}
	for _, it := range items {
		if it.status != 0 && it.status != 1 { // PDH_CSTATUS_VALID_DATA / NEW_DATA
			continue
		}
		name := windows.UTF16PtrToString(it.name)
		key := name
		if i := strings.Index(name, "luid_"); i >= 0 {
			key = name[i:]
			if j := strings.Index(key, "_phys"); j > 0 {
				key = key[:j]
			}
		}
		per[key] += it.value
	}
	best := 0.0
	for _, v := range per {
		if v > best {
			best = v
		}
	}
	return best, len(per) > 0
}

func (s *sysSampler) fill(p *PerfSample) {
	s.once.Do(s.init)
	// CPU: busy share of all cores since the last sample
	var idle, kern, user windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&user))); r != 0 {
		i, k, u := ftU64(idle), ftU64(kern), ftU64(user)
		if s.haveCPU {
			di, dk, du := i-s.idle, k-s.kernel, u-s.user // kernel time includes idle time
			if total := dk + du; total > 0 {
				p.CPU = fptr(clampF(100*float64(total-di)/float64(total), 0, 100))
			}
		}
		s.idle, s.kernel, s.user, s.haveCPU = i, k, u, true
	}
	// RAM
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, _ := procGlobalMemoryStatusExPer.Call(uintptr(unsafe.Pointer(&ms))); r != 0 {
		p.RAMMB = fptr(float64(ms.TotalPhys) / (1 << 20))
		p.RAMUsedMB = fptr(float64(ms.TotalPhys-ms.AvailPhys) / (1 << 20))
	}
	if s.ramSpeed > 0 {
		p.RAMSpeed = fptr(s.ramSpeed)
	}
	// GPU
	if s.pdhOK {
		if r, _, _ := procPdhCollectQueryData.Call(s.query); r == 0 {
			if v, ok := s.pdhArray(s.gpuCounter); ok {
				p.GPU = fptr(clampF(v, 0, 100))
			}
			if v, ok := s.pdhArray(s.vramCtr); ok {
				p.VRAMUsedMB = fptr(v / (1 << 20))
			}
		}
	}
	if s.vramMB > 0 {
		p.VRAMMB = fptr(s.vramMB)
	}
	if s.nvmlOK {
		var t uint32
		if r, _, _ := s.nvmlTemp.Call(s.nvmlDev, 0, uintptr(unsafe.Pointer(&t))); r == 0 && t > 0 && t < 130 {
			p.GPUTemp = fptr(float64(t))
		}
	}
	// CPU temperature: no dependable sensor interface for applications on Windows
}

type memoryStatusEx struct {
	Length, MemoryLoad                                 uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile uint64
	TotalVirtual, AvailVirtual, AvailExtendedVirtual   uint64
}

// smbiosMemorySpeed reads the configured memory speed (MT/s) of the installed modules from the
// SMBIOS tables (type 17 "Memory Device"); 0 when not available.
func smbiosMemorySpeed() float64 {
	const rsmb = 'R'<<24 | 'S'<<16 | 'M'<<8 | 'B'
	n, _, _ := procGetSystemFirmwareTable.Call(rsmb, 0, 0, 0)
	if n == 0 || n > 1<<20 {
		return 0
	}
	buf := make([]byte, n)
	if r, _, _ := procGetSystemFirmwareTable.Call(rsmb, 0, uintptr(unsafe.Pointer(&buf[0])), n); r == 0 {
		return 0
	}
	// RawSMBIOSData: 8-byte header, then the table
	if len(buf) < 8 {
		return 0
	}
	tbl := buf[8:]
	best := 0.0
	for i := 0; i+4 <= len(tbl); {
		typ, ln := tbl[i], int(tbl[i+1])
		if ln < 4 || i+ln > len(tbl) {
			break
		}
		if typ == 17 && ln >= 0x17 {
			speed := float64(binary.LittleEndian.Uint16(tbl[i+0x15:]))
			if ln >= 0x22 {
				if c := float64(binary.LittleEndian.Uint16(tbl[i+0x20:])); c > 0 && c != 0xFFFF {
					speed = c
				}
			}
			if speed > 0 && speed != 0xFFFF && speed > best {
				best = speed
			}
		}
		if typ == 127 {
			break
		}
		// skip the formatted area and the strings (ended by two zero bytes)
		j := i + ln
		for j+1 < len(tbl) && !(tbl[j] == 0 && tbl[j+1] == 0) {
			j++
		}
		i = j + 2
	}
	return best
}

var _ = time.Second
