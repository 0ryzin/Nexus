package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// parseVulkanDevice reads the fields NEXUS uses from VkPhysicalDeviceProperties (p) and
// VkPhysicalDeviceMemoryProperties (m), as laid out in memory on 64-bit platforms.
func parseVulkanDevice(index int, p, m []byte) VulkanDevice {
	le := binary.LittleEndian
	api := le.Uint32(p[0:])
	d := VulkanDevice{Index: index, VendorID: le.Uint32(p[8:]), DeviceID: le.Uint32(p[12:]),
		API: fmt.Sprintf("%d.%d.%d", api>>22, (api>>12)&0x3ff, api&0xfff)}
	d.Type = map[uint32]string{0: "other", 1: "integrated", 2: "discrete", 3: "virtual", 4: "cpu"}[le.Uint32(p[16:])]
	name := p[20 : 20+256]
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	d.Name = string(name)
	heaps := le.Uint32(m[260:])
	for h := uint32(0); h < heaps && h < 16; h++ {
		off := 264 + h*16
		if le.Uint32(m[off+8:])&1 != 0 { // VK_MEMORY_HEAP_DEVICE_LOCAL_BIT
			d.LocalMB += le.Uint64(m[off:]) >> 20
		}
	}
	return d
}
