//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listVulkanDevices asks the Vulkan loader (vulkan-1.dll from System32) for the physical
// devices in this process, in the order the engine will see them (gpu_select.go). The result
// is cached: it is the same until NEXUS restarts.
var vkDevicesOnce struct {
	sync.Once
	devs []VulkanDevice
	err  error
}

func listVulkanDevices() ([]VulkanDevice, error) {
	vkDevicesOnce.Do(func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			vkDevicesOnce.devs, vkDevicesOnce.err = enumerateVulkan()
		}()
		select {
		case <-done:
		case <-time.After(8 * time.Second): // a broken Vulkan layer can hang instance creation
			vkDevicesOnce.err = fmt.Errorf("the Vulkan driver did not answer")
		}
	})
	return vkDevicesOnce.devs, vkDevicesOnce.err
}

func enumerateVulkan() ([]VulkanDevice, error) {
	dll := windows.NewLazySystemDLL("vulkan-1.dll") // System32 only
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("vulkan-1.dll: %v", err)
	}
	create, destroy := dll.NewProc("vkCreateInstance"), dll.NewProc("vkDestroyInstance")
	enum, props, mem := dll.NewProc("vkEnumeratePhysicalDevices"), dll.NewProc("vkGetPhysicalDeviceProperties"), dll.NewProc("vkGetPhysicalDeviceMemoryProperties")
	for _, p := range []*windows.LazyProc{create, destroy, enum, props, mem} {
		if err := p.Find(); err != nil {
			return nil, err
		}
	}
	appName := []byte("NEXUS\x00")
	app := vkApplicationInfo{PAppName: &appName[0], PEngineName: &appName[0], APIVersion: 1<<22 | 1<<12}
	ci := vkInstanceCreateInfo{SType: 1, PApplicationInfo: &app}
	var inst uintptr
	if r, _, _ := create.Call(uintptr(unsafe.Pointer(&ci)), 0, uintptr(unsafe.Pointer(&inst))); int32(r) != 0 {
		return nil, fmt.Errorf("vkCreateInstance failed (%d)", int32(r))
	}
	runtime.KeepAlive(appName)
	defer destroy.Call(inst, 0)
	var count uint32
	if r, _, _ := enum.Call(inst, uintptr(unsafe.Pointer(&count)), 0); int32(r) != 0 || count == 0 {
		return nil, fmt.Errorf("no Vulkan devices (%d)", int32(r))
	}
	pds := make([]uintptr, count)
	if r, _, _ := enum.Call(inst, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&pds[0]))); int32(r) < 0 {
		return nil, fmt.Errorf("vkEnumeratePhysicalDevices failed (%d)", int32(r))
	}
	var out []VulkanDevice
	for i, pd := range pds[:count] {
		var p [1024]byte // VkPhysicalDeviceProperties (824 bytes)
		props.Call(pd, uintptr(unsafe.Pointer(&p[0])))
		var m [600]byte // VkPhysicalDeviceMemoryProperties (520 bytes)
		mem.Call(pd, uintptr(unsafe.Pointer(&m[0])))
		out = append(out, parseVulkanDevice(i, p[:], m[:]))
	}
	return out, nil
}
