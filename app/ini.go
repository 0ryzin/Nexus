package main

import (
	"bytes"
	"os"
	"strings"
)

// IniFile is a minimal, order-preserving editor for the SimpleIni files Lichii
// writes (qt-config.ini / sdl2-config.ini). Untouched lines are kept verbatim.
type IniFile struct {
	lines []string
	bom   bool
	crlf  bool
}

func LoadIni(path string) (*IniFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &IniFile{crlf: true}, nil
		}
		return nil, err
	}
	f := &IniFile{}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		f.bom = true
		data = data[3:]
	}
	s := string(data)
	f.crlf = strings.Contains(s, "\r\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s != "" {
		f.lines = strings.Split(s, "\n")
	}
	return f, nil
}

func (f *IniFile) Save(path string) error {
	nl := "\n"
	if f.crlf {
		nl = "\r\n"
	}
	var b bytes.Buffer
	if f.bom {
		b.Write([]byte{0xEF, 0xBB, 0xBF})
	}
	for _, l := range f.lines {
		b.WriteString(l)
		b.WriteString(nl)
	}
	tmp := path + ".nexus-tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func sectionOf(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
		return t[1 : len(t)-1], true
	}
	return "", false
}

// find returns the line index of key in section (or -1) and the index where a
// new key for that section should be inserted (-1 if the section is missing).
func (f *IniFile) find(section, key string) (int, int) {
	in := false
	insert := -1
	for i, l := range f.lines {
		if s, ok := sectionOf(l); ok {
			if in {
				return -1, insert
			}
			in = s == section
			if in {
				insert = i + 1
			}
			continue
		}
		if !in {
			continue
		}
		if strings.TrimSpace(l) != "" {
			insert = i + 1
		}
		k, _, ok := strings.Cut(l, "=")
		if ok && strings.TrimSpace(k) == key {
			return i, insert
		}
	}
	return -1, insert
}

func (f *IniFile) Get(section, key string) (string, bool) {
	i, _ := f.find(section, key)
	if i < 0 {
		return "", false
	}
	_, v, _ := strings.Cut(f.lines[i], "=")
	v = strings.TrimSpace(v)
	return strings.Trim(v, `"`), true
}

func (f *IniFile) Set(section, key, value string) {
	line := key + "=" + value
	i, insert := f.find(section, key)
	switch {
	case i >= 0:
		f.lines[i] = line
	case insert >= 0:
		f.lines = append(f.lines[:insert], append([]string{line}, f.lines[insert:]...)...)
	default:
		if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
			f.lines = append(f.lines, "")
		}
		f.lines = append(f.lines, "["+section+"]", line)
	}
}

// SetSetting writes a Lichii "SwitchableSetting" so it is honoured: the value
// plus "\default=false" (otherwise Lichii ignores the value and uses its default).
func (f *IniFile) SetSetting(section, key, value string) {
	f.Set(section, key+`\default`, "false")
	f.Set(section, key, value)
}

// GetSetting returns the effective custom value, or ok=false if Lichii is
// using its built-in default.
func (f *IniFile) GetSetting(section, key string) (string, bool) {
	if d, ok := f.Get(section, key+`\default`); ok && d == "true" {
		return "", false
	}
	return f.Get(section, key)
}

// iniPath formats a filesystem path the way Lichii's AdjustOutputString does.
func iniPath(p string) string {
	s := strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(s, "//") && !strings.HasPrefix(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	if strings.ContainsAny(s, "!#$%^&*|;'\",<>?`~=") {
		s = `"` + s + `"`
	}
	return s
}
