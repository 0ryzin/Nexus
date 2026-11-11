//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows hardware detection without cgo: registry and GetLogicalProcessorInformationEx (CPU),
// GlobalMemoryStatusEx (RAM), DXGI (graphics adapters, video memory, driver version), the Vulkan
// loader (Vulkan support per GPU), RtlGetVersion (Windows version) and the display settings API.
func detectHardware() Hardware {
	h := Hardware{}
	safe := func(what string, f func()) {
		defer func() {
			if r := recover(); r != nil {
				h.Notes = append(h.Notes, fmt.Sprintf("%s could not be read (%v).", what, r))
			}
		}()
		f()
	}
	safe("Processor information", func() { detectCPU(&h) })
	safe("Memory information", func() { detectMemory(&h) })
	safe("Windows version", func() { detectOS(&h) })
	safe("Graphics adapters (DXGI)", func() { detectDXGI(&h) })
	safe("Vulkan support", func() { detectVulkan(&h) })
	safe("Display settings", func() { detectDisplays(&h) })
	return h
}

func detectCPU(h *Hardware) {
	h.CPU.Architecture = runtime.GOARCH
	h.CPU.Features = cpuFeatures()
	h.CPU.Threads = runtime.NumCPU()
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
			h.CPU.Model = strings.Join(strings.Fields(v), " ")
		}
		if v, _, err := k.GetStringValue("VendorIdentifier"); err == nil {
			h.CPU.Vendor = map[string]string{"GenuineIntel": "Intel", "AuthenticAMD": "AMD"}[v]
			if h.CPU.Vendor == "" {
				h.CPU.Vendor = v
			}
		}
		if v, _, err := k.GetIntegerValue("~MHz"); err == nil {
			h.CPU.BaseMHz = int(v)
		}
		k.Close()
	}
	// physical cores: one RelationProcessorCore record per core
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetLogicalProcessorInformationEx")
	var n uint32
	_, _, _ = proc.Call(0, 0, uintptr(unsafe.Pointer(&n)))
	if n > 0 && n < 1<<20 {
		buf := make([]byte, n)
		if r, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r != 0 {
			cores := 0
			for off := uint32(0); off+8 <= n; {
				size := binary.LittleEndian.Uint32(buf[off+4:])
				if binary.LittleEndian.Uint32(buf[off:]) == 0 {
					cores++
				}
				if size == 0 {
					break
				}
				off += size
			}
			h.CPU.Cores = cores
		}
	}
}

func detectMemory(h *Hardware) {
	var ms struct {
		Length, MemoryLoad                        uint32
		TotalPhys, AvailPhys, TotalPageFile       uint64
		AvailPageFile, TotalVirtual, AvailVirtual uint64
		AvailExtendedVirtual                      uint64
	}
	ms.Length = uint32(unsafe.Sizeof(ms))
	p := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	if r, _, _ := p.Call(uintptr(unsafe.Pointer(&ms))); r != 0 {
		h.Memory = MemoryInfo{TotalMB: int64(ms.TotalPhys >> 20), AvailableMB: int64(ms.AvailPhys >> 20),
			UsedMB: int64((ms.TotalPhys - ms.AvailPhys) >> 20), LoadPercent: int(ms.MemoryLoad)}
	}
}

func detectOS(h *Hardware) {
	v := windows.RtlGetVersion()
	h.OS.Arch = runtime.GOARCH
	build := v.BuildNumber
	h.OS.Name = "Windows 10"
	if v.MajorVersion == 10 && build >= 22000 {
		h.OS.Name = "Windows 11"
	} else if v.MajorVersion != 10 {
		h.OS.Name = fmt.Sprintf("Windows %d.%d", v.MajorVersion, v.MinorVersion)
	}
	h.OS.Build = fmt.Sprint(build)
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		if dv, _, err := k.GetStringValue("DisplayVersion"); err == nil {
			h.OS.Version = dv
		}
		if ed, _, err := k.GetStringValue("EditionID"); err == nil && ed != "" {
			h.OS.Name += " " + map[string]string{"Core": "Home", "Professional": "Pro"}[ed]
			h.OS.Name = strings.TrimSpace(h.OS.Name)
		}
		if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
			h.OS.Build = fmt.Sprintf("%d.%d", build, ubr)
		}
		k.Close()
	}
}

// ---------- DXGI

type guid struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}

var (
	iidIDXGIFactory1 = guid{0x770aae78, 0xf26f, 0x4dba, [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
	iidIDXGIDevice   = guid{0x54ec77fa, 0x1377, 0x44e6, [8]byte{0x8c, 0x32, 0x88, 0xfd, 0x5f, 0x44, 0xc8, 0x4c}}
)

func comCall(obj uintptr, index int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func detectDXGI(h *Hardware) {
	create := windows.NewLazySystemDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
	if create.Find() != nil {
		h.Notes = append(h.Notes, "DXGI is not available, so graphics adapters could not be listed.")
		return
	}
	var factory uintptr
	if r, _, _ := create.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); r != 0 || factory == 0 {
		h.Notes = append(h.Notes, "DXGI did not list the graphics adapters.")
		return
	}
	defer comCall(factory, 2) // Release
	for i := uintptr(0); i < 16; i++ {
		var adapter uintptr
		if r := comCall(factory, 12, i, uintptr(unsafe.Pointer(&adapter))); r != 0 || adapter == 0 { // EnumAdapters1
			break
		}
		var desc [312]byte
		comCall(adapter, 10, uintptr(unsafe.Pointer(&desc[0]))) // GetDesc1
		var umd int64
		hasDriver := comCall(adapter, 9, uintptr(unsafe.Pointer(&iidIDXGIDevice)), uintptr(unsafe.Pointer(&umd))) == 0 // CheckInterfaceSupport
		comCall(adapter, 2)
		name := windows.UTF16ToString((*[128]uint16)(unsafe.Pointer(&desc[0]))[:])
		flags := binary.LittleEndian.Uint32(desc[304:])
		g := GPUInfo{Model: strings.TrimSpace(name), VendorID: binary.LittleEndian.Uint32(desc[256:]), DeviceID: binary.LittleEndian.Uint32(desc[260:]),
			DedicatedMB: int64(binary.LittleEndian.Uint64(desc[272:]) >> 20), SharedMB: int64(binary.LittleEndian.Uint64(desc[288:]) >> 20)}
		if flags&2 != 0 || (g.VendorID == 0x1414 && g.DeviceID == 0x8c) { // software adapter (Basic Render Driver)
			continue
		}
		g.Vendor = gpuVendorName(g.VendorID)
		if hasDriver {
			u := uint64(umd)
			g.Driver = fmt.Sprintf("%d.%d.%d.%d", u>>48&0xffff, u>>32&0xffff, u>>16&0xffff, u&0xffff)
		}
		g.DirectX = "DirectX 11/12 (DXGI)"
		g.Kind = classifyGPU(g)
		g.Integrated = g.Kind == "integrated"
		g.Primary = len(h.GPUs) == 0 // DXGI lists the adapter driving the main display first
		h.GPUs = append(h.GPUs, g)
	}
}

// classifyGPU: integrated GPUs share system memory and have little or no dedicated memory.
// The Vulkan device type (when available) replaces this estimate.
func classifyGPU(g GPUInfo) string {
	m := strings.ToLower(g.Model)
	switch {
	case g.VendorID == 0x8086 && !strings.Contains(m, "arc a") && !strings.Contains(m, "arc b") && g.DedicatedMB < 2048:
		return "integrated"
	case g.DedicatedMB > 0 && g.DedicatedMB <= 1024:
		return "integrated"
	case g.DedicatedMB > 1024:
		return "discrete"
	}
	return "unknown"
}

// ---------- Vulkan

type vkApplicationInfo struct {
	SType         uint32
	PNext         uintptr
	PAppName      *byte
	AppVersion    uint32
	PEngineName   *byte
	EngineVersion uint32
	APIVersion    uint32
}

type vkInstanceCreateInfo struct {
	SType                 uint32
	PNext                 uintptr
	Flags                 uint32
	PApplicationInfo      *vkApplicationInfo
	EnabledLayerCount     uint32
	PPEnabledLayerNames   uintptr
	EnabledExtensionCount uint32
	PPEnabledExtNames     uintptr
}

func detectVulkan(h *Hardware) {
	dll := windows.NewLazySystemDLL("vulkan-1.dll")
	if dll.Load() != nil {
		h.Notes = append(h.Notes, "No Vulkan driver is installed (vulkan-1.dll not found). Update the graphics driver to use Vulkan.")
		return
	}
	h.Vulkan.Loader = true
	if p := dll.NewProc("vkEnumerateInstanceVersion"); p.Find() == nil {
		var v uint32
		if r, _, _ := p.Call(uintptr(unsafe.Pointer(&v))); r == 0 {
			h.Vulkan.Instance = vkVersionString(v)
		}
	} else {
		h.Vulkan.Instance = "1.0"
	}
	name := append([]byte("NEXUS hardware check"), 0)
	app := vkApplicationInfo{PAppName: &name[0], PEngineName: &name[0], APIVersion: 1<<22 | 1<<12} // 1.1
	ci := vkInstanceCreateInfo{SType: 1, PApplicationInfo: &app}
	app.SType = 0
	var inst uintptr
	if r, _, _ := dll.NewProc("vkCreateInstance").Call(uintptr(unsafe.Pointer(&ci)), 0, uintptr(unsafe.Pointer(&inst))); r != 0 || inst == 0 {
		h.Notes = append(h.Notes, "The Vulkan driver could not be started for the hardware check.")
		return
	}
	runtime.KeepAlive(name)
	runtime.KeepAlive(&app)
	defer dll.NewProc("vkDestroyInstance").Call(inst, 0)
	var count uint32
	enum := dll.NewProc("vkEnumeratePhysicalDevices")
	_, _, _ = enum.Call(inst, uintptr(unsafe.Pointer(&count)), 0)
	if count == 0 || count > 16 {
		return
	}
	devs := make([]uintptr, count)
	_, _, _ = enum.Call(inst, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&devs[0])))
	props := dll.NewProc("vkGetPhysicalDeviceProperties")
	for _, d := range devs[:count] {
		var p [1024]byte // VkPhysicalDeviceProperties is 824 bytes
		_, _, _ = props.Call(d, uintptr(unsafe.Pointer(&p[0])))
		api := binary.LittleEndian.Uint32(p[0:])
		drv := binary.LittleEndian.Uint32(p[4:])
		ven := binary.LittleEndian.Uint32(p[8:])
		dev := binary.LittleEndian.Uint32(p[12:])
		typ := binary.LittleEndian.Uint32(p[16:])
		dname := strings.TrimRight(string(p[20:276]), "\x00")
		if i := strings.IndexByte(dname, 0); i >= 0 {
			dname = dname[:i]
		}
		if typ == 4 { // CPU (software rasteriser)
			continue
		}
		h.Vulkan.Available = true
		matched := false
		for i := range h.GPUs {
			g := &h.GPUs[i]
			if g.VendorID == ven && g.DeviceID == dev && g.VulkanAPI == "" {
				g.VulkanAPI, g.VulkanDriver = vkVersionString(api), vkDriverString(ven, drv)
				switch typ {
				case 1:
					g.Kind, g.Integrated = "integrated", true
				case 2:
					g.Kind, g.Integrated = "discrete", false
				}
				matched = true
				break
			}
		}
		if !matched {
			kind := map[uint32]string{1: "integrated", 2: "discrete", 3: "virtual"}[typ]
			h.GPUs = append(h.GPUs, GPUInfo{Model: dname, Vendor: gpuVendorName(ven), VendorID: ven, DeviceID: dev, Kind: kind,
				Integrated: typ == 1, VulkanAPI: vkVersionString(api), VulkanDriver: vkDriverString(ven, drv), Primary: len(h.GPUs) == 0})
		}
	}
}

func vkDriverString(vendor, v uint32) string {
	switch vendor {
	case 0x10DE: // NVIDIA
		return fmt.Sprintf("%d.%d", v>>22&0x3ff, v>>14&0xff)
	case 0x8086: // Intel on Windows
		return fmt.Sprintf("%d.%d", v>>14, v&0x3fff)
	}
	return vkVersionString(v)
}

// ---------- displays

func detectDisplays(h *Hardware) {
	u32 := windows.NewLazySystemDLL("user32.dll")
	enumDev := u32.NewProc("EnumDisplayDevicesW")
	enumSet := u32.NewProc("EnumDisplaySettingsW")
	for i := uintptr(0); i < 16; i++ {
		var dd [840]byte
		binary.LittleEndian.PutUint32(dd[0:], 840)
		if r, _, _ := enumDev.Call(0, i, uintptr(unsafe.Pointer(&dd[0])), 0); r == 0 {
			break
		}
		state := binary.LittleEndian.Uint32(dd[324:])
		if state&1 == 0 { // DISPLAY_DEVICE_ATTACHED_TO_DESKTOP
			continue
		}
		var dm [220]byte
		binary.LittleEndian.PutUint16(dm[68:], 220)
		if r, _, _ := enumSet.Call(uintptr(unsafe.Pointer(&dd[4])), 0xFFFFFFFF, uintptr(unsafe.Pointer(&dm[0]))); r == 0 {
			continue
		}
		d := Display{
			Name:      fmt.Sprintf("Display %d", len(h.Displays)+1),
			Width:     int(binary.LittleEndian.Uint32(dm[172:])),
			Height:    int(binary.LittleEndian.Uint32(dm[176:])),
			RefreshHz: int(binary.LittleEndian.Uint32(dm[184:])),
			Primary:   state&4 != 0,
		}
		// every mode the display offers at its current resolution: the available refresh rates
		seen := map[int]bool{}
		for m := uintptr(0); m < 2000; m++ {
			var mm [220]byte
			binary.LittleEndian.PutUint16(mm[68:], 220)
			if r, _, _ := enumSet.Call(uintptr(unsafe.Pointer(&dd[4])), m, uintptr(unsafe.Pointer(&mm[0]))); r == 0 {
				break
			}
			if int(binary.LittleEndian.Uint32(mm[172:])) == d.Width && int(binary.LittleEndian.Uint32(mm[176:])) == d.Height {
				if hz := int(binary.LittleEndian.Uint32(mm[184:])); hz > 1 && !seen[hz] {
					seen[hz] = true
					d.RefreshRates = append(d.RefreshRates, hz)
				}
			}
		}
		sort.Ints(d.RefreshRates)
		h.Displays = append(h.Displays, d)
	}
}
