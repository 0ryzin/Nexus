package main

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// TestMiiMatchesEngine: testdata/mii_engine_reference.bin was written by the engine's own Mii code
// (store_data.cpp, core_data.cpp, mii_database.cpp of the NEXUS engine, compiled on their own):
// the female base Mii's CoreData, the male base Mii's StoreData (with its random create ID) and
// a database holding that Mii. NEXUS must produce the same bytes, checksums included.
func TestMiiMatchesEngine(t *testing.T) {
	ref, err := os.ReadFile("testdata/mii_engine_reference.bin")
	if err != nil {
		t.Fatal(err)
	}
	fem := Mii{Name: "no name", Fields: baseMiiFields(true)}
	if c := miiCore(&fem); !bytes.Equal(c[:], ref[:0x30]) {
		t.Fatalf("female CoreData\nnexus  %x\nengine %x", c, ref[:0x30])
	}
	store := ref[0x30 : 0x30+0x44]
	male := Mii{Name: "no name", Fields: baseMiiFields(false), ID: strings.ToUpper(hex.EncodeToString(store[0x30:0x40]))}
	s, err := miiStore(&male)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s[:], store) {
		t.Fatalf("male StoreData\nnexus  %x\nengine %x", s, store)
	}
	db, err := buildMiiDatabase([]Mii{male})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(db, ref[0x30+0x44:]) {
		t.Fatal("database differs from the engine's")
	}
	back, skipped, err := parseMiiDatabase(db)
	if err != nil || skipped != 0 || len(back) != 1 || back[0].ID != male.ID || back[0].Name != "no name" {
		t.Fatalf("read back: %+v %d %v", back, skipped, err)
	}
	for k, v := range male.Fields {
		if back[0].Fields[k] != v {
			t.Fatalf("%s: %d != %d", k, back[0].Fields[k], v)
		}
	}
}

func TestMiiValidation(t *testing.T) {
	m := Mii{Name: "Tom", Fields: baseMiiFields(false)}
	if err := validateMii(&m); err != nil {
		t.Fatal(err)
	}
	m.Fields["hairType"] = 132
	if validateMii(&m) == nil {
		t.Fatal("hair 132 accepted")
	}
	m.Fields["hairType"] = 0
	m.Name = "ElevenChars"
	if validateMii(&m) == nil {
		t.Fatal("11-character name accepted")
	}
	many := make([]Mii, 100)
	if _, err := buildMiiDatabase(many); err == nil {
		t.Fatal("100 Miis accepted (the engine calls that damaged)")
	}
}

// TestMiiCollectionsPerUser: each user's Miis are their own; the default goes first into the
// engine's database for that user's game; deleting the default picks another.
func TestMiiCollectionsPerUser(t *testing.T) {
	a, _ := newTestApp(t, "")
	tom, _ := a.Profiles.Create("Tom", Avatar{Kind: "preset", Preset: "star", Color: "#ff0000"})
	ann, _ := a.Profiles.Create("Ann", Avatar{Kind: "preset", Preset: "star", Color: "#00ff00"})
	t1, err := a.Miis.Save(tom.ID, Mii{Name: "Tom", Fields: baseMiiFields(false)})
	if err != nil {
		t.Fatal(err)
	}
	t2, _ := a.Miis.Save(tom.ID, Mii{Name: "Racer", Fields: baseMiiFields(false)})
	a.Miis.Save(ann.ID, Mii{Name: "Ann", Fields: baseMiiFields(true)})
	if err := a.Miis.SetDefault(tom.ID, t2.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Miis.PrepareForGame(tom.ID); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(a.Miis.engineDBPath())
	got, _, err := parseMiiDatabase(b)
	if err != nil || len(got) != 2 || got[0].Name != "Racer" || got[1].Name != "Tom" {
		t.Fatalf("Tom's game sees %+v %v", got, err)
	}
	a.Miis.PrepareForGame(ann.ID)
	b, _ = os.ReadFile(a.Miis.engineDBPath())
	got, _, _ = parseMiiDatabase(b)
	if len(got) != 1 || got[0].Name != "Ann" {
		t.Fatalf("Ann's game sees %+v", got)
	}
	// edit keeps the ID
	t1.Fields["hairColor"] = 5
	t1.Name = "Tommy"
	if e, err := a.Miis.Save(tom.ID, t1); err != nil || e.ID != t1.ID {
		t.Fatal(err)
	}
	a.Miis.Delete(tom.ID, t2.ID)
	if c := a.Miis.Get(tom.ID); c.Default != t1.ID || len(c.Miis) != 1 || c.Miis[0].Name != "Tommy" {
		t.Fatalf("after delete: %+v", c)
	}
	// import: the same Mii again adds nothing
	b, _ = buildMiiDatabase(a.Miis.Get(tom.ID).Miis)
	if added, _, err := a.Miis.Import(tom.ID, b); err != nil || added != 0 {
		t.Fatalf("re-import added %d (%v)", added, err)
	}
	if added, _, _ := a.Miis.Import(ann.ID, b); added != 1 {
		t.Fatal("import into another user")
	}
}
