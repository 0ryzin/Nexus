package main

// Miis: each NEXUS user keeps their own Mii collection; before a game starts NEXUS writes that
// user's Miis into the engine's Mii database, the file the engine's mii service (nn::mii) reads,
// so games that use Miis (Mario Kart 8 Deluxe's Mii racer, for example) list them.
//
// The format is the Switch's own, as the engine implements it (src/core/hle/service/mii):
//   - CoreData, 0x30 bytes: seven little-endian u32 words of bit fields (StoreDataBitFields,
//     core_data.h) and the nickname, 10 UTF-16LE characters.
//   - StoreData, 0x44 bytes: CoreData, the create ID (a random RFC 4122 v4 UUID), the data CRC
//     (CRC-16/CCITT of CoreData + create ID, byte-swapped) and the device CRC (mii_util.h
//     CalculateDeviceCrc16 over the device's Mii author ID; the engine uses "Eden Default UID").
//   - The database, NintendoFigurineDatabase, 0x1A98 bytes: "NFDB", 100 StoreData slots, version 1,
//     the number used (at most 99: the engine treats 100 as damaged), and the CRC of everything
//     before it. It lives in the system save 8000000000000030 as MiiDatabase.dat.
// The engine checks all three CRCs and every field's range (CoreData::IsValid) and empties a
// database that fails, so NEXUS validates the same ranges before writing anything.
//
// Miis are not amiibo (amiibo.go). Nothing here comes from Nintendo: no Mii parts artwork is
// shipped; the editor shows a simplified drawing of NEXUS's own.

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

const (
	miiCoreSize    = 0x30
	miiStoreSize   = 0x44
	miiDBSize      = 0x1A98
	miiDBSlots     = 100
	miiMaxInDB     = 99 // a database holding 100 fails the engine's integrity check
	miiDBMagic     = 0x4244464e
	miiNameMax     = 10
	miiDBSaveID    = "8000000000000030"
	miiDBFileName  = "MiiDatabase.dat"
	miiDeviceIDStr = "Eden Default UID" // Common::UUID::MakeDefault(), the engine's Mii author ID
)

// miiField: one bit field of CoreData (word 0-6, bit position, width) and its highest valid value.
type miiField struct {
	Key        string
	Word, Bit  uint
	Width, Max uint32
}

// The order and limits of core_data.h / mii_types.h (MaxHeight..., enum Max values).
var miiFields = []miiField{
	{"hairType", 0, 0, 8, 131}, {"height", 0, 8, 7, 127}, {"moleType", 0, 15, 1, 1}, {"build", 0, 16, 7, 127},
	{"hairFlip", 0, 23, 1, 1}, {"hairColor", 0, 24, 7, 99}, {"type", 0, 31, 1, 1},
	{"eyeColor", 1, 0, 7, 99}, {"gender", 1, 7, 1, 1}, {"eyebrowColor", 1, 8, 7, 99}, {"mouthColor", 1, 16, 7, 99},
	{"beardColor", 1, 24, 7, 99},
	{"glassColor", 2, 0, 7, 99}, {"eyeType", 2, 8, 6, 59}, {"regionMove", 2, 14, 2, 3}, {"mouthType", 2, 16, 6, 35},
	{"fontRegion", 2, 22, 2, 3}, {"eyeY", 2, 24, 5, 18}, {"glassScale", 2, 29, 3, 7},
	{"eyebrowType", 3, 0, 5, 23}, {"mustacheType", 3, 5, 3, 6}, {"noseType", 3, 8, 5, 17}, {"beardType", 3, 13, 3, 6},
	{"noseY", 3, 16, 5, 18}, {"mouthAspect", 3, 21, 3, 6}, {"mouthY", 3, 24, 5, 18}, {"eyebrowAspect", 3, 29, 3, 6},
	{"mustacheY", 4, 0, 5, 16}, {"eyeRotate", 4, 5, 3, 7}, {"glassY", 4, 8, 5, 20}, {"eyeAspect", 4, 13, 3, 6},
	{"moleX", 4, 16, 5, 16}, {"eyeScale", 4, 21, 3, 7}, {"moleY", 4, 24, 5, 30},
	{"glassType", 5, 0, 5, 19}, {"favoriteColor", 5, 8, 4, 11}, {"facelineType", 5, 12, 4, 11},
	{"facelineColor", 5, 16, 4, 9}, {"facelineWrinkle", 5, 20, 4, 11}, {"facelineMake", 5, 24, 4, 11}, {"eyeX", 5, 28, 4, 12},
	{"eyebrowScale", 6, 0, 4, 8}, {"eyebrowRotate", 6, 4, 4, 11}, {"eyebrowX", 6, 8, 4, 12}, {"eyebrowY", 6, 12, 4, 15},
	{"noseScale", 6, 16, 4, 8}, {"mouthScale", 6, 20, 4, 8}, {"mustacheScale", 6, 24, 4, 8}, {"moleScale", 6, 28, 4, 8},
}

// Mii is one Mii of a user's collection.
type Mii struct {
	ID       string            `json:"id"` // the create ID, 32 hex digits
	Name     string            `json:"name"`
	Fields   map[string]uint32 `json:"fields"`
	Created  int64             `json:"created"`
	Modified int64             `json:"modified"`
}

// MiiCollection is a user's Miis; Default is used first (games that pick the first Mii get it).
type MiiCollection struct {
	Default string `json:"default,omitempty"`
	Miis    []Mii  `json:"miis"`
}

// baseMiiFields: the engine's base Mii (RawData::BaseMii, BuildBase), with the Ver3 colour tables
// already applied (hair 1 -> 1, eye 0 -> 8, mouth 0 -> 19, glasses 0 -> 8, eyebrow_y - 3).
func baseMiiFields(female bool) map[string]uint32 {
	f := map[string]uint32{
		"facelineType": 0, "facelineColor": 0, "facelineWrinkle": 0, "facelineMake": 0,
		"hairType": 33, "hairColor": 1, "hairFlip": 0,
		"eyeType": 2, "eyeColor": 8, "eyeScale": 4, "eyeAspect": 3, "eyeRotate": 4, "eyeX": 2, "eyeY": 12,
		"eyebrowType": 6, "eyebrowColor": 1, "eyebrowScale": 4, "eyebrowAspect": 3, "eyebrowRotate": 6, "eyebrowX": 2, "eyebrowY": 7,
		"noseType": 1, "noseScale": 4, "noseY": 9,
		"mouthType": 23, "mouthColor": 19, "mouthScale": 4, "mouthAspect": 3, "mouthY": 13,
		"mustacheType": 0, "beardType": 0, "beardColor": 8, "mustacheScale": 4, "mustacheY": 10,
		"glassType": 0, "glassColor": 8, "glassScale": 4, "glassY": 10,
		"moleType": 0, "moleScale": 4, "moleX": 2, "moleY": 20,
		"height": 64, "build": 64, "gender": 0, "favoriteColor": 0, "regionMove": 0, "fontRegion": 0, "type": 0,
	}
	if female {
		f["hairType"], f["eyeType"], f["eyeRotate"], f["eyebrowType"], f["gender"] = 12, 4, 3, 0, 1
	}
	return f
}

// validateMii checks what CoreData::IsValid checks: every field in range and a usable name.
func validateMii(m *Mii) error {
	name := []rune(strings.TrimSpace(m.Name))
	if len(name) == 0 {
		return errors.New("Give the Mii a name.")
	}
	if len(utf16.Encode(name)) > miiNameMax {
		return fmt.Errorf("A Mii name can have at most %d characters.", miiNameMax)
	}
	for _, r := range name {
		if r == 0 || r < 0x20 {
			return errors.New("The name contains a character a Mii name cannot have.")
		}
	}
	for _, f := range miiFields {
		v, ok := m.Fields[f.Key]
		if !ok {
			return fmt.Errorf("The Mii has no value for %s.", f.Key)
		}
		if v > f.Max {
			return fmt.Errorf("%s is %d; the highest a Mii allows is %d.", f.Key, v, f.Max)
		}
	}
	if m.Fields["type"] != 0 {
		return errors.New("Special Miis cannot be made here.")
	}
	return nil
}

// miiCore encodes CoreData.
func miiCore(m *Mii) [miiCoreSize]byte {
	var b [miiCoreSize]byte
	var w [7]uint32
	for _, f := range miiFields {
		w[f.Word] |= (m.Fields[f.Key] & (1<<f.Width - 1)) << f.Bit
	}
	for i, v := range w {
		binary.LittleEndian.PutUint32(b[i*4:], v)
	}
	u := utf16.Encode([]rune(strings.TrimSpace(m.Name)))
	for i := 0; i < miiNameMax && i < len(u); i++ {
		binary.LittleEndian.PutUint16(b[0x1c+i*2:], u[i])
	}
	return b
}

// decodeMiiCore reads CoreData.
func decodeMiiCore(b []byte) Mii {
	m := Mii{Fields: map[string]uint32{}}
	for _, f := range miiFields {
		w := binary.LittleEndian.Uint32(b[f.Word*4:])
		m.Fields[f.Key] = (w >> f.Bit) & (1<<f.Width - 1)
	}
	var u []uint16
	for i := 0; i < miiNameMax; i++ {
		c := binary.LittleEndian.Uint16(b[0x1c+i*2:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	m.Name = string(utf16.Decode(u))
	return m
}

// miiCRC16: MiiUtil::CalculateCrc16 (CRC-16/CCITT, initial 0), returned byte-swapped as there.
func miiCRC16(data []byte) uint16 {
	var crc uint32
	for _, c := range data {
		crc ^= uint32(c) << 8
		for j := 0; j < 8; j++ {
			crc <<= 1
			if crc&0x10000 != 0 {
				crc = (crc ^ 0x1021) & 0xFFFF
			}
		}
	}
	v := uint16(crc)
	return v<<8 | v>>8
}

// miiDeviceCRC16: MiiUtil::CalculateDeviceCrc16 (the author ID, then size*8 zero bits).
func miiDeviceCRC16(author []byte, size int) uint16 {
	var crc uint32
	step := func() {
		crc <<= 1
		if crc&0x10000 != 0 {
			crc ^= 0x1021
		}
		crc &= 0x1FFFF // only the low 16 bits and the tested bit matter (the engine keeps more)
	}
	for _, c := range author {
		for j := 0; j < 8; j++ {
			step()
		}
		crc ^= uint32(c)
	}
	for i := 0; i < size*8; i++ {
		step()
	}
	v := uint16(crc)
	return v<<8 | v>>8
}

// miiStore encodes StoreData with both checksums.
func miiStore(m *Mii) ([miiStoreSize]byte, error) {
	var b [miiStoreSize]byte
	id, err := hex.DecodeString(m.ID)
	if err != nil || len(id) != 16 {
		return b, errors.New("invalid Mii ID")
	}
	core := miiCore(m)
	copy(b[:], core[:])
	copy(b[miiCoreSize:], id)
	binary.LittleEndian.PutUint16(b[0x40:], miiCRC16(b[:0x40]))
	binary.LittleEndian.PutUint16(b[0x42:], miiDeviceCRC16([]byte(miiDeviceIDStr), miiStoreSize))
	return b, nil
}

// buildMiiDatabase: the engine's MiiDatabase.dat holding miis in order.
func buildMiiDatabase(miis []Mii) ([]byte, error) {
	if len(miis) > miiMaxInDB {
		return nil, fmt.Errorf("at most %d Miis fit in a game's Mii list", miiMaxInDB)
	}
	b := make([]byte, miiDBSize)
	binary.LittleEndian.PutUint32(b, miiDBMagic)
	for i := range miis {
		if err := validateMii(&miis[i]); err != nil {
			return nil, fmt.Errorf("%s: %w", miis[i].Name, err)
		}
		s, err := miiStore(&miis[i])
		if err != nil {
			return nil, err
		}
		copy(b[4+i*miiStoreSize:], s[:])
	}
	tail := 4 + miiDBSlots*miiStoreSize
	b[tail] = 1 // version
	b[tail+1] = byte(len(miis))
	binary.LittleEndian.PutUint16(b[tail+2:], miiCRC16(b[:tail+2]))
	return b, nil
}

// parseMiiDatabase reads a MiiDatabase.dat (the engine's, or one from another emulator built on
// the same core). Entries whose own data checksum fails are skipped and counted.
func parseMiiDatabase(b []byte) (miis []Mii, skipped int, err error) {
	if len(b) != miiDBSize || binary.LittleEndian.Uint32(b) != miiDBMagic {
		return nil, 0, errors.New("This is not a Mii database (MiiDatabase.dat).")
	}
	tail := 4 + miiDBSlots*miiStoreSize
	if binary.LittleEndian.Uint16(b[tail+2:]) != miiCRC16(b[:tail+2]) {
		return nil, 0, errors.New("The Mii database is damaged (checksum).")
	}
	n := int(b[tail+1])
	if n >= miiDBSlots {
		return nil, 0, errors.New("The Mii database is damaged (count).")
	}
	for i := 0; i < n; i++ {
		e := b[4+i*miiStoreSize : 4+(i+1)*miiStoreSize]
		if binary.LittleEndian.Uint16(e[0x40:]) != miiCRC16(e[:0x40]) {
			skipped++
			continue
		}
		m := decodeMiiCore(e[:miiCoreSize])
		m.ID = strings.ToUpper(hex.EncodeToString(e[miiCoreSize:0x40]))
		if m.Fields["type"] != 0 || validateMii(&m) != nil {
			skipped++ // special Miis and out-of-range data are not imported
			continue
		}
		miis = append(miis, m)
	}
	return miis, skipped, nil
}

// newMiiID: a random RFC 4122 version 4 UUID, as MiiUtil::MakeCreateId.
func newMiiID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return strings.ToUpper(hex.EncodeToString(b))
}

// ---------------------------------------------------------------- per-user collections

type MiiStore struct {
	app *App
	mu  sync.Mutex
}

func (s *MiiStore) path(profileID string) string {
	return filepath.Join(s.app.Paths.Profiles, filepath.Base(profileID), "miis.json")
}

func (s *MiiStore) Get(profileID string) MiiCollection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(profileID)
}

func (s *MiiStore) getLocked(profileID string) MiiCollection {
	var c MiiCollection
	_ = readJSONFile(s.path(profileID), &c)
	if c.Miis == nil {
		c.Miis = []Mii{}
	}
	return c
}

func (s *MiiStore) putLocked(profileID string, c MiiCollection) error {
	if err := os.MkdirAll(filepath.Dir(s.path(profileID)), 0o755); err != nil {
		return err
	}
	return writeJSONAtomic(s.path(profileID), c)
}

func (s *MiiStore) checkUser(profileID string) error {
	if _, ok := s.app.Profiles.Get(profileID); !ok {
		return errors.New("User not found.")
	}
	return nil
}

// Save adds a Mii (empty ID) or replaces the one with its ID.
func (s *MiiStore) Save(profileID string, m Mii) (Mii, error) {
	if err := s.checkUser(profileID); err != nil {
		return m, err
	}
	m.Name = strings.TrimSpace(m.Name)
	if m.Fields == nil {
		return m, errors.New("The Mii has no data.")
	}
	if err := validateMii(&m); err != nil {
		return m, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.getLocked(profileID)
	now := time.Now().Unix()
	m.Modified = now
	if m.ID == "" {
		if len(c.Miis) >= miiMaxInDB {
			return m, fmt.Errorf("A user can have at most %d Miis.", miiMaxInDB)
		}
		m.ID, m.Created = newMiiID(), now
		c.Miis = append(c.Miis, m)
		if c.Default == "" {
			c.Default = m.ID
		}
	} else {
		found := false
		for i := range c.Miis {
			if c.Miis[i].ID == m.ID {
				m.Created = c.Miis[i].Created
				c.Miis[i], found = m, true
			}
		}
		if !found {
			return m, errors.New("That Mii no longer exists.")
		}
	}
	return m, s.putLocked(profileID, c)
}

func (s *MiiStore) Delete(profileID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.getLocked(profileID)
	out := c.Miis[:0]
	for _, m := range c.Miis {
		if m.ID != id {
			out = append(out, m)
		}
	}
	if len(out) == len(c.Miis) {
		return errors.New("That Mii no longer exists.")
	}
	c.Miis = out
	if c.Default == id {
		c.Default = ""
		if len(out) > 0 {
			c.Default = out[0].ID
		}
	}
	return s.putLocked(profileID, c)
}

func (s *MiiStore) SetDefault(profileID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.getLocked(profileID)
	for _, m := range c.Miis {
		if m.ID == id {
			c.Default = id
			return s.putLocked(profileID, c)
		}
	}
	return errors.New("That Mii no longer exists.")
}

// Import adds the Miis of a MiiDatabase.dat that the user does not have yet (same create ID).
func (s *MiiStore) Import(profileID string, data []byte) (added, skipped int, err error) {
	if err := s.checkUser(profileID); err != nil {
		return 0, 0, err
	}
	miis, bad, err := parseMiiDatabase(data)
	if err != nil {
		return 0, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.getLocked(profileID)
	have := map[string]bool{}
	for _, m := range c.Miis {
		have[m.ID] = true
	}
	now := time.Now().Unix()
	for _, m := range miis {
		if have[m.ID] {
			continue
		}
		if len(c.Miis) >= miiMaxInDB {
			bad++
			continue
		}
		m.Created, m.Modified = now, now
		c.Miis = append(c.Miis, m)
		added++
	}
	if c.Default == "" && len(c.Miis) > 0 {
		c.Default = c.Miis[0].ID
	}
	return added, bad, s.putLocked(profileID, c)
}

// ordered: the user's Miis with the default first.
func (c MiiCollection) ordered() []Mii {
	out := make([]Mii, 0, len(c.Miis))
	for _, m := range c.Miis {
		if m.ID == c.Default {
			out = append(out, m)
		}
	}
	for _, m := range c.Miis {
		if m.ID != c.Default {
			out = append(out, m)
		}
	}
	return out
}

// engineDBPath: where the engine's mii service reads its database.
func (s *MiiStore) engineDBPath() string {
	return filepath.Join(s.app.Emu.DataDir(), "nand", "system", "save", miiDBSaveID, miiDBFileName)
}

// PrepareForGame writes the playing user's Miis into the engine's database before a game starts.
// A user without Miis leaves the database as it is (Miis made in a game stay where they are);
// the previous database is backed up once per day before it is replaced.
func (s *MiiStore) PrepareForGame(profileID string) error {
	c := s.Get(profileID)
	if len(c.Miis) == 0 {
		return nil
	}
	data, err := buildMiiDatabase(c.ordered())
	if err != nil {
		return err
	}
	p := s.engineDBPath()
	if old, err := os.ReadFile(p); err == nil {
		if string(old) == string(data) {
			return nil
		}
		bk := filepath.Join(s.app.Paths.System, "MiiBackups", "MiiDatabase-"+time.Now().Format("2006-01-02")+".dat")
		if !fileExists(bk) {
			_ = os.MkdirAll(filepath.Dir(bk), 0o755)
			_ = os.WriteFile(bk, old, 0o644)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(p, data); err != nil {
		return err
	}
	s.app.Log.Printf("miis: %d Mii(s) of the playing user written to the engine's Mii database", len(c.Miis))
	return nil
}

// miiJSON is used by the API to pass a collection plus the field limits to the interface.
func miiLimits() map[string]uint32 {
	out := map[string]uint32{}
	for _, f := range miiFields {
		out[f.Key] = f.Max
	}
	return out
}

var _ = json.Marshal
