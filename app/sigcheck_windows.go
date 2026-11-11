//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// checkNVIDIASignature: the file has a valid Authenticode signature (WinVerifyTrust, with the
// certificate chain and revocation checked by Windows) and the signer is NVIDIA Corporation.
func checkNVIDIASignature(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE, UnionChoice: windows.WTD_CHOICE_FILE, StateAction: windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file)}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	if verr != nil {
		return fmt.Errorf("its digital signature is not valid (%v)", verr)
	}
	signer, err := signerName(path)
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(signer), "nvidia") {
		return fmt.Errorf("it is signed by %q, not by NVIDIA", signer)
	}
	return nil
}

var procCryptMsgGetParam = windows.NewLazySystemDLL("crypt32.dll").NewProc("CryptMsgGetParam")

// signerName reads the signing certificate's display name from the file's embedded signature.
func signerName(path string) (string, error) {
	p, _ := windows.UTF16PtrFromString(path)
	var enc, ctype, ftype uint32
	var store, msg windows.Handle
	err := windows.CryptQueryObject(windows.CERT_QUERY_OBJECT_FILE, unsafe.Pointer(p), windows.CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED,
		windows.CERT_QUERY_FORMAT_FLAG_BINARY, 0, &enc, &ctype, &ftype, &store, &msg, nil)
	if err != nil {
		return "", errors.New("it has no embedded signature")
	}
	defer windows.CertCloseStore(store, 0)
	defer procCryptMsgClose.Call(uintptr(msg))
	const cmsgSignerInfoParam = 6
	var size uint32
	if r, _, _ := procCryptMsgGetParam.Call(uintptr(msg), cmsgSignerInfoParam, 0, 0, uintptr(unsafe.Pointer(&size))); r == 0 || size == 0 {
		return "", errors.New("its signer could not be read")
	}
	buf := make([]byte, size)
	if r, _, _ := procCryptMsgGetParam.Call(uintptr(msg), cmsgSignerInfoParam, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return "", errors.New("its signer could not be read")
	}
	// CMSG_SIGNER_INFO: dwVersion, Issuer (CERT_NAME_BLOB), SerialNumber (CRYPT_INTEGER_BLOB), …
	type blob struct {
		Size uint32
		Data uintptr
	}
	type signerInfo struct {
		Version uint32
		Issuer  blob
		Serial  blob
	}
	si := (*signerInfo)(unsafe.Pointer(&buf[0]))
	var certInfo windows.CertInfo
	certInfo.Issuer = windows.CertNameBlob{Size: si.Issuer.Size, Data: (*byte)(unsafe.Pointer(si.Issuer.Data))}
	certInfo.SerialNumber = windows.CryptIntegerBlob{Size: si.Serial.Size, Data: (*byte)(unsafe.Pointer(si.Serial.Data))}
	const certFindSubjectCert = 0x000B0000
	cert, err := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, 0, certFindSubjectCert, unsafe.Pointer(&certInfo), nil)
	if err != nil || cert == nil {
		return "", errors.New("its signing certificate could not be found")
	}
	defer windows.CertFreeCertificateContext(cert)
	name := make([]uint16, 256)
	n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &name[0], uint32(len(name)))
	if n <= 1 {
		return "", errors.New("its signer has no name")
	}
	return windows.UTF16ToString(name[:n]), nil
}

var procCryptMsgClose = windows.NewLazySystemDLL("crypt32.dll").NewProc("CryptMsgClose")
