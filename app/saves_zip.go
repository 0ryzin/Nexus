package main

// Import Save from ZIP: safety checks for archives the user selects.
//
// A ZIP is only ever read. NEXUS never executes anything from it, never extracts outside a
// staging folder inside the user's save folder, and refuses archives that could escape that
// folder or exhaust the disk or memory (zip bombs). Every entry of a save is decompressed and
// its CRC checked in the staging folder before any existing save is touched.

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
)

const (
	maxSaveZipBytes      = 1 << 30 // the archive itself (it is read into memory)
	maxSaveZipEntries    = 50_000  // files and folders
	maxSaveZipTotal      = 8 << 30 // all files, uncompressed
	maxSaveZipFile       = 2 << 30 // one file, uncompressed
	maxSaveZipRatio      = 1000    // uncompressed : compressed, for files over 1 MB
	maxSaveZipPathLength = 400     // characters
	maxSaveZipDepth      = 32      // folder levels
)

var errZipTooLarge = fmt.Errorf("The ZIP archive is larger than %d GB. Put only the save folders in the ZIP (saves are usually a few MB).", maxSaveZipBytes>>30)

// zipProblem is a reason an archive is refused, with what the user can do about it.
type zipProblem struct{ msg string }

func (z zipProblem) Error() string { return z.msg }

// cleanZipName returns the entry's path with "/" separators, or an error if the path could
// point outside the folder it is extracted to or cannot exist on Windows.
func cleanZipName(raw string) (string, error) {
	n := strings.ReplaceAll(raw, "\\", "/")
	if strings.HasPrefix(n, "/") || (len(n) >= 2 && n[1] == ':') {
		return "", fmt.Errorf("%q is an absolute path", raw)
	}
	n = strings.TrimSuffix(n, "/")
	if n == "" {
		return "", nil
	}
	if len(n) > maxSaveZipPathLength {
		return "", fmt.Errorf("%q is too long", raw[:40]+"…")
	}
	parts := strings.Split(n, "/")
	if len(parts) > maxSaveZipDepth {
		return "", fmt.Errorf("%q is nested too deeply", raw)
	}
	for _, p := range parts {
		switch {
		case p == "" || p == "." || p == "..":
			return "", fmt.Errorf("%q contains a relative path (%q)", raw, p)
		case strings.ContainsAny(p, ":*?\"<>|"):
			return "", fmt.Errorf("%q contains characters Windows does not allow", raw)
		case strings.HasSuffix(p, ".") || strings.HasSuffix(p, " "):
			return "", fmt.Errorf("%q has a name ending in a dot or space", raw)
		case winReserved[strings.ToUpper(strings.SplitN(p, ".", 2)[0])]:
			return "", fmt.Errorf("%q uses a name reserved by Windows", raw)
		}
		for _, r := range p {
			if unicode.IsControl(r) {
				return "", fmt.Errorf("%q contains control characters", raw)
			}
		}
	}
	return path.Clean(n), nil
}

// checkSaveZip validates the whole archive before anything in it is used.
func checkSaveZip(zr *zip.Reader) error {
	if len(zr.File) > maxSaveZipEntries {
		return zipProblem{fmt.Sprintf("The ZIP archive has more than %d entries. Put only the save folders in the ZIP.", maxSaveZipEntries)}
	}
	var total uint64
	seen := map[string]bool{}
	for _, f := range zr.File {
		name, err := cleanZipName(f.Name)
		if err != nil {
			return zipProblem{"The ZIP archive contains an unsafe path, so NEXUS will not use it: " + err.Error() + ". Create the ZIP again from the save folders themselves."}
		}
		if name == "" {
			continue
		}
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 || mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice) != 0 {
			return zipProblem{fmt.Sprintf("The ZIP archive contains a link or special file (%s). Saves are ordinary files and folders; create the ZIP again from the save folders.", name)}
		}
		if f.Flags&0x1 != 0 {
			return zipProblem{"The ZIP archive is password-protected. Create it again without a password."}
		}
		key := strings.ToLower(name)
		if seen[key] && !f.FileInfo().IsDir() {
			return zipProblem{fmt.Sprintf("The ZIP archive contains %s twice. Create it again.", name)}
		}
		seen[key] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return zipProblem{fmt.Sprintf("%s is compressed with a method NEXUS cannot read. Use Windows' \"Compress to ZIP file\" or another ZIP tool's standard (Deflate) setting.", name)}
		}
		if f.UncompressedSize64 > maxSaveZipFile {
			return zipProblem{fmt.Sprintf("%s is larger than %d GB, which is not a save file.", name, maxSaveZipFile>>30)}
		}
		if f.UncompressedSize64 > 1<<20 && f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > maxSaveZipRatio {
			return zipProblem{fmt.Sprintf("%s would expand to an unusual size (%d MB from %d KB), so NEXUS will not open it.", name, f.UncompressedSize64>>20, f.CompressedSize64>>10)}
		}
		total += f.UncompressedSize64
		if total > maxSaveZipTotal {
			return zipProblem{fmt.Sprintf("The ZIP archive expands to more than %d GB. Put only the save folders in the ZIP.", maxSaveZipTotal>>30)}
		}
	}
	return nil
}

// openSaveZip opens and checks an archive the user selected.
func openSaveZip(data []byte) (*zip.Reader, error) {
	if int64(len(data)) > maxSaveZipBytes {
		return nil, errZipTooLarge
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if errors.Is(err, zip.ErrFormat) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, zipProblem{"That file is not a ZIP archive, or the archive is damaged or incomplete. If it was downloaded or copied, try again; otherwise create the ZIP again."}
		}
		return nil, zipProblem{"The ZIP archive cannot be read: " + err.Error()}
	}
	if err := checkSaveZip(zr); err != nil {
		return nil, err
	}
	return zr, nil
}

// unsupportedZipHelp explains how to make an archive NEXUS recognises.
const unsupportedZipHelp = "NEXUS looks for save folders named after the game's 16-character ID (for example 0100ABCD12340000), optionally inside a player folder named with a 32-character ID, or for saves with their metadata file (ExtraData0). Open the other program's save folder, select the game folders (or the player folder that contains them), compress them to a ZIP and import that ZIP. If your saves have no ID in their folder names, you can still import them and choose the game yourself."
