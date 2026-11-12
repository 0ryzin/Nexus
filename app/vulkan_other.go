//go:build !windows

package main

import "errors"

// listVulkanDevices: on other platforms the engine's own device choice is used (Android devices
// have one GPU; the Android app reports it for the readiness check).
func listVulkanDevices() ([]VulkanDevice, error) {
	return nil, errors.New("not available on this platform")
}
