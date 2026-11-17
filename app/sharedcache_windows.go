//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// defaultSharedShaderDir: %ProgramData%\NEXUS\ShaderCache, readable and writable by every user.
func defaultSharedShaderDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		return ""
	}
	return filepath.Join(pd, "NEXUS", "ShaderCache")
}

// sharedDirSDDL: SYSTEM and Administrators full control, Users modify (read, write, create,
// delete what is inside), inherited by everything created inside. Without it a file one Windows
// user's NEXUS created in ProgramData could not be appended to by another's.
const sharedDirSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1301bf;;;BU)"

func prepareSharedDir(d string) error {
	created := !dirExists(d)
	if err := os.MkdirAll(d, 0o777); err != nil {
		return err
	}
	if created {
		sd, err := windows.SecurityDescriptorFromString(sharedDirSDDL)
		if err != nil {
			return err
		}
		dacl, _, err := sd.DACL()
		if err != nil {
			return err
		}
		if err := windows.SetNamedSecurityInfo(d, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			return err
		}
	}
	// usable by this user: a file can be created and removed
	probe := filepath.Join(d, ".nexus-write-test")
	if err := os.WriteFile(probe, nil, 0o666); err != nil {
		return err
	}
	return os.Remove(probe)
}
