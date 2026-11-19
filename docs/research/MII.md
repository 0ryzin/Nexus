# Miis in NEXUS (0.8.1)

## What a game reads
Games use the Switch's Mii service (nn::mii). In the NEXUS engine (Lichii/Eden core,
`src/core/hle/service/mii`) that service reads one file: the system save 8000000000000030,
`nand/system/save/8000000000000030/MiiDatabase.dat`, at the moment a game opens the database.

## The format (from the engine's code, byte-checked)
- **CoreData** 0x30 bytes: seven little-endian u32 bit-field words (`StoreDataBitFields`,
  `core_data.h`) + the nickname (10 UTF-16LE characters).
- **StoreData** 0x44 bytes: CoreData, the create ID (random RFC 4122 v4 UUID), the data CRC
  (CRC-16/CCITT over CoreData + create ID, stored byte-swapped) and the device CRC
  (`CalculateDeviceCrc16` over the author ID; the engine's is the ASCII "Eden Default UID").
- **NintendoFigurineDatabase** 0x1A98 bytes: "NFDB", 100 StoreData slots, version 1, count, CRC of
  everything before it. The engine rejects a count of 100 and any CRC or range error
  (`CheckIntegrity`, `CoreData::IsValid`) and then **empties the database**, so NEXUS validates the
  same ranges first and never writes more than 99.
- Proof: `testdata/mii_engine_reference.bin` was produced by the engine's own `store_data.cpp`,
  `core_data.cpp`, `raw_data.cpp` and `mii_database.cpp`, compiled on their own (base Miis and a
  database). `TestMiiMatchesEngine` requires NEXUS to produce the identical bytes.

## What NEXUS does
- Each user has `Profiles/<user>/miis.json` (their Miis + the default).
- Before a game starts, the playing user's Miis are written to the engine's database, default
  first (a daily backup of the previous database goes to `System/MiiBackups`). A user without Miis
  leaves the database untouched, so Miis a game created stay.
- Import: a MiiDatabase.dat file, or the one on this device ("From this device's games").
- Real-console Miis carry another author ID: their device CRC differs. NEXUS keeps their data
  and writes them with the engine's author ID, as the engine itself would.
- Not done: Mii parts artwork (Nintendo's; not shipped). The editor's picture is NEXUS's own
  simplified drawing; parts are numbered. Colours are the classic Mii palettes the engine maps.
- Not verified in a real game yet (Mario Kart 8 Deluxe's Mii racer is the test).
