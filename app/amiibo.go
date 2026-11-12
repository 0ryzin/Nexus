package main

// NEXUS amiibo library.
//
// Two kinds of entries:
//   - dumps the user imports: NTAG215 dumps (532/540/572 bytes) of their own figures and cards,
//     or 1 KB MIFARE dumps. NEXUS checks the structure, reads the amiibo ID and keeps a copy; the
//     file the user chose is never changed.
//   - virtual amiibo NEXUS creates from an amiibo ID: a plain (unencrypted, unsigned) NTAG215
//     image with a random serial number that the engine accepts as a "plain amiibo". It works
//     only inside NEXUS. It is not a Nintendo-authenticated amiibo, carries no signature or keys,
//     and cannot be written to a figure or NFC tag to make one.
//
// When a game scans for an amiibo, the chosen entry's file is handed to the engine
// (nexus_engine_amiibo). The user's own amiibo keys (key_retail.bin) are needed only for games
// that write data back to an encrypted dump; reading works without them, and virtual amiibo never
// need them. NEXUS includes no amiibo dumps, images or keys.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Amiibo struct {
	ID       string `json:"id"`                 // NEXUS entry ID
	Name     string `json:"name"`               // the user's name for it
	AmiiboID string `json:"amiiboId,omitempty"` // 16 hex digits (character, variant, type, model, series, 02)
	Series   string `json:"series,omitempty"`
	Kind     string `json:"kind"` // dump | virtual | mifare
	Owner    string `json:"owner,omitempty"`
	UID      string `json:"uid,omitempty"`
	File     string `json:"file"` // inside the amiibo folder
	Added    int64  `json:"added"`
}

type AmiiboStore struct {
	app  *App
	dir  string
	mu   sync.Mutex
	list []Amiibo
}

func NewAmiiboStore(a *App, dir string) *AmiiboStore {
	s := &AmiiboStore{app: a, dir: dir}
	_ = os.MkdirAll(dir, 0o755)
	_ = readJSON(filepath.Join(dir, "amiibo.json"), &s.list)
	return s
}

func (s *AmiiboStore) saveLocked() error {
	return writeJSONAtomic(filepath.Join(s.dir, "amiibo.json"), s.list)
}

// List returns the library, the given user's amiibo and unassigned ones first.
func (s *AmiiboStore) List() []Amiibo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Amiibo(nil), s.list...)
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (s *AmiiboStore) Get(id string) (Amiibo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.list {
		if a.ID == id {
			return a, true
		}
	}
	return Amiibo{}, false
}

func (s *AmiiboStore) Path(a Amiibo) string { return filepath.Join(s.dir, filepath.Base(a.File)) }

// ---------- dump formats

const (
	amiiboSizeNoPassword = 532
	amiiboSize           = 540
	amiiboSizeSigned     = 572
	mifareSize           = 1024
)

var errNotAmiibo = errors.New("This file is not an amiibo dump. NEXUS reads NTAG215 dumps of amiibo (532, 540 or 572 bytes, as made by amiibo backup tools from your own figures) and 1 KB MIFARE dumps.")

// ParseAmiiboDump checks a dump and returns its kind, amiibo ID and serial number.
func ParseAmiiboDump(b []byte) (kind, amiiboID, uid string, err error) {
	switch len(b) {
	case mifareSize:
		return "mifare", "", hex.EncodeToString(b[:4]), nil
	case amiiboSizeNoPassword, amiiboSize, amiiboSizeSigned:
	default:
		return "", "", "", errNotAmiibo
	}
	cc := []byte{0xF1, 0x10, 0xFF, 0xEE}
	switch {
	case string(b[12:16]) == string(cc): // raw (encrypted) dump, as read from the tag
		if b[3] != 0x88^b[0]^b[1]^b[2] || b[8] != b[4]^b[5]^b[6]^b[7] {
			return "", "", "", fmt.Errorf("%w (Its serial number checksum is wrong; the dump may be damaged.)", errNotAmiibo)
		}
		return "dump", hex.EncodeToString(b[0x54:0x5C]), hex.EncodeToString(append(append([]byte{}, b[0:3]...), b[4:8]...)), nil
	case string(b[4:8]) == string(cc): // plain (decrypted) layout
		u := b[0x1D4:0x1DC]
		if u[3] != 0x88^u[0]^u[1]^u[2] || b[0] != u[4]^u[5]^u[6]^u[7] {
			return "", "", "", fmt.Errorf("%w (Its serial number checksum is wrong; the dump may be damaged.)", errNotAmiibo)
		}
		return "dump", hex.EncodeToString(b[0x1DC:0x1E4]), hex.EncodeToString(append(append([]byte{}, u[0:3]...), u[4:8]...)), nil
	}
	return "", "", "", errNotAmiibo
}

// validAmiiboID: 16 hex digits ending in 02 (the NTAG215 tag type every amiibo carries).
func validAmiiboID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 8 && b[7] == 0x02
}

// VirtualAmiiboImage builds a plain NTAG215 image (the decoded layout the engine reads as a
// "plain amiibo") for an amiibo ID, with a random 7-byte serial number. Not initialised: the
// game asks to register it (owner and nickname) the first time, as with a new figure.
func VirtualAmiiboImage(amiiboID string) ([]byte, error) {
	id, err := hex.DecodeString(amiiboID)
	if err != nil || !validAmiiboID(amiiboID) {
		return nil, errors.New("An amiibo ID is 16 hexadecimal digits ending in 02, for example 0100000000040002.")
	}
	b := make([]byte, amiiboSize)
	uid := make([]byte, 7)
	if _, err := rand.Read(uid); err != nil {
		return nil, err
	}
	uid[0] = 0x04 // NXP, as on real NTAG215 tags
	// TagUuid at 0x1D4: part1[3], BCC0, part2[3], last byte; BCC1 at 0x00
	u := b[0x1D4:0x1DC]
	copy(u[0:3], uid[0:3])
	u[3] = 0x88 ^ uid[0] ^ uid[1] ^ uid[2]
	copy(u[4:8], uid[3:7])
	b[0x00] = u[4] ^ u[5] ^ u[6] ^ u[7]
	b[0x01] = 0x48                                       // internal number
	b[0x02], b[0x03] = 0x0F, 0xE0                        // static lock 0xE00F
	copy(b[0x04:0x08], []byte{0xF1, 0x10, 0xFF, 0xEE})   // capability container
	b[0x28] = 0xA5                                       // constant
	copy(b[0x1DC:0x1E4], id)                             // model info: the amiibo ID (last byte: tag type 2)
	copy(b[0x208:0x20C], []byte{0x01, 0x00, 0x0F, 0xBD}) // dynamic lock
	copy(b[0x20C:0x210], []byte{0x00, 0x00, 0x00, 0x04}) // CFG0
	copy(b[0x210:0x214], []byte{0x5F, 0x00, 0x00, 0x00}) // CFG1
	return b, nil
}

// ---------- library operations

func newAmiiboEntryID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Import copies a dump into the library (the original file is only read).
func (s *AmiiboStore) Import(path, owner string) (Amiibo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Amiibo{}, err
	}
	if fi.Size() > 4096 {
		return Amiibo{}, errNotAmiibo
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Amiibo{}, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return s.add(data, name, owner)
}

func (s *AmiiboStore) add(data []byte, name, owner string) (Amiibo, error) {
	kind, aid, uid, err := ParseAmiiboDump(data)
	if err != nil {
		return Amiibo{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.list {
		if a.UID != "" && a.UID == uid && a.AmiiboID == aid {
			return Amiibo{}, fmt.Errorf("This amiibo is already in your library as %q.", a.Name)
		}
	}
	a := Amiibo{ID: newAmiiboEntryID(), Name: firstNonEmpty(strings.TrimSpace(name), "amiibo"), AmiiboID: aid, Kind: kind, Owner: owner, UID: uid, Added: nowUnix()}
	a.File = a.ID + ".bin"
	if err := writeFileAtomic(filepath.Join(s.dir, a.File), data); err != nil {
		return Amiibo{}, err
	}
	s.list = append(s.list, a)
	if err := s.saveLocked(); err != nil {
		return Amiibo{}, err
	}
	s.app.Log.Printf("amiibo: added %s (%s, %s)", a.Name, a.Kind, a.AmiiboID)
	return a, nil
}

// CreateVirtual adds a NEXUS virtual amiibo for an amiibo ID.
func (s *AmiiboStore) CreateVirtual(amiiboID, name, series, owner string) (Amiibo, error) {
	amiiboID = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(amiiboID, "0x")))
	img, err := VirtualAmiiboImage(amiiboID)
	if err != nil {
		return Amiibo{}, err
	}
	a, err := s.add(img, firstNonEmpty(name, "Virtual amiibo "+amiiboID), owner)
	if err != nil {
		return Amiibo{}, err
	}
	return s.Update(a.ID, func(x *Amiibo) { x.Kind, x.Series = "virtual", series })
}

func (s *AmiiboStore) Update(id string, fn func(a *Amiibo)) (Amiibo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.list {
		if s.list[i].ID == id {
			fn(&s.list[i])
			s.list[i].Name = strings.TrimSpace(s.list[i].Name)
			if s.list[i].Name == "" {
				s.list[i].Name = "amiibo"
			}
			return s.list[i], s.saveLocked()
		}
	}
	return Amiibo{}, errors.New("That amiibo is not in the library.")
}

// Remove deletes an entry and its copy (the user's original file is elsewhere and unchanged).
func (s *AmiiboStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.list {
		if a.ID == id {
			_ = os.Remove(filepath.Join(s.dir, filepath.Base(a.File)))
			s.list = append(s.list[:i], s.list[i+1:]...)
			return s.saveLocked()
		}
	}
	return errors.New("That amiibo is not in the library.")
}

// Place puts an amiibo on the running game's virtual NFC reader ("" takes it away).
func (s *AmiiboStore) Place(id string) error {
	st := s.app.Emu.State()
	if !st.Running {
		return errors.New("No game is running.")
	}
	host, err := s.app.Emu.Host()
	if err != nil {
		return err
	}
	path := ""
	if id != "" {
		a, ok := s.Get(id)
		if !ok {
			return errors.New("That amiibo is not in the library.")
		}
		path = s.Path(a)
	}
	switch r := host.Amiibo(path); r {
	case 0:
		return nil
	case 1:
		return errors.New("The game is not scanning for an amiibo right now. Use the amiibo feature in the game first (for example, the amiibo menu), then choose the amiibo.")
	case 2:
		return errors.New("The amiibo file could not be read.")
	case 3:
		return errNotAmiibo
	case -2:
		return errors.New("This NEXUS Engine build cannot use amiibo (engine 0.6 or newer is needed).")
	default:
		return errors.New("No game is running.")
	}
}

// ---------- amiibo keys (the user's own key_retail.bin)

const amiiboKeysSize = 160

func (s *AmiiboStore) keysPath() string { return filepath.Join(s.app.Emu.KeysDir(), "key_retail.bin") }

func (s *AmiiboStore) KeysInstalled() bool {
	fi, err := os.Stat(s.keysPath())
	return err == nil && fi.Size() == amiiboKeysSize
}

// InstallKeys copies the user's amiibo keys file next to their console keys.
func (s *AmiiboStore) InstallKeys(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) != amiiboKeysSize {
		return fmt.Errorf("This is not an amiibo keys file (key_retail.bin is %d bytes; this file is %d).", amiiboKeysSize, len(b))
	}
	_ = os.MkdirAll(filepath.Dir(s.keysPath()), 0o755)
	return writeFileAtomic(s.keysPath(), b)
}

// ---------- amiibo names (AmiiboAPI, an open database of amiibo IDs and names; text only)

type AmiiboInfo struct {
	Name      string `json:"name"`
	Series    string `json:"amiiboSeries"`
	GameSerie string `json:"gameSeries"`
	Type      string `json:"type"`
	Head      string `json:"head"`
	Tail      string `json:"tail"`
}

func (i AmiiboInfo) ID() string { return strings.ToLower(i.Head + i.Tail) }

var amiiboAPIBases = []string{"https://www.amiiboapi.org/api/amiibo/", "https://www.amiiboapi.com/api/amiibo/"}

// LookupAmiibo searches the open amiibo database by name, or by ID (16 hex digits).
func LookupAmiibo(query string) ([]AmiiboInfo, error) {
	q := strings.TrimSpace(query)
	v := url.Values{}
	if validAmiiboID(strings.ToLower(q)) {
		v.Set("head", q[:8])
		v.Set("tail", q[8:])
	} else if q != "" {
		v.Set("name", q)
	} else {
		return nil, errors.New("Type a name or an amiibo ID.")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for _, base := range amiiboAPIBases {
		resp, err := client.Get(base + "?" + v.Encode())
		if err != nil {
			lastErr = err
			continue
		}
		var out struct {
			Amiibo json.RawMessage `json:"amiibo"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode == 404 {
			return nil, nil // nothing found
		}
		if err != nil || resp.StatusCode != 200 {
			lastErr = fmt.Errorf("the amiibo database answered %s", resp.Status)
			continue
		}
		var list []AmiiboInfo
		if json.Unmarshal(out.Amiibo, &list) != nil {
			var one AmiiboInfo
			if json.Unmarshal(out.Amiibo, &one) == nil {
				list = []AmiiboInfo{one}
			}
		}
		if len(list) > 50 {
			list = list[:50]
		}
		return list, nil
	}
	return nil, fmt.Errorf("The amiibo database could not be reached (%v). You can still create a virtual amiibo by typing its ID.", lastErr)
}
