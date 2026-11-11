package main

// Cryptography for NEXUS Device Linking (experimental, see devicelink.go).
//
// Pairing uses SPAKE2 (RFC 9382) on P-256 with the RFC's M and N points: both devices prove they
// know the short pairing code without revealing it, and an eavesdropper or a fake device cannot
// test guesses offline — every guess needs a live attempt, and the primary device allows only a
// few per code. The shared key then protects the exchange of the devices' long-term Ed25519
// identities, which pin every later connection (mutual TLS 1.3, certificates checked by public key).

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"time"
)

// RFC 9382, section 6: M and N for P-256 (compressed SEC1).
var (
	spakeM = mustPoint("02886e2f97ace46e55ba9dd7242579f2993b64e16ef3dcab95afd497333d8fa12f")
	spakeN = mustPoint("03d8bbd6c639c62937b04d997f38c3770719c629d7014d49a24b4f98baa1292b49")
)

type ecPoint struct{ X, Y *big.Int }

func mustPoint(h string) ecPoint {
	b, _ := hex.DecodeString(h)
	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), b)
	if x == nil {
		panic("bad SPAKE2 constant")
	}
	return ecPoint{x, y}
}

func p256() elliptic.Curve { return elliptic.P256() }

func (p ecPoint) bytes() []byte { return elliptic.Marshal(p256(), p.X, p.Y) } //nolint:staticcheck

func pointFrom(b []byte) (ecPoint, error) {
	x, y := elliptic.Unmarshal(p256(), b) //nolint:staticcheck // also checks the point is on the curve
	if x == nil {
		return ecPoint{}, errors.New("invalid point")
	}
	return ecPoint{x, y}, nil
}

func scalarMult(p ecPoint, k []byte) ecPoint {
	x, y := p256().ScalarMult(p.X, p.Y, k) //nolint:staticcheck
	return ecPoint{x, y}
}
func baseMult(k []byte) ecPoint {
	x, y := p256().ScalarBaseMult(k) //nolint:staticcheck
	return ecPoint{x, y}
}
func addPoints(a, b ecPoint) ecPoint {
	x, y := p256().Add(a.X, a.Y, b.X, b.Y) //nolint:staticcheck
	return ecPoint{x, y}
}
func negPoint(a ecPoint) ecPoint {
	return ecPoint{a.X, new(big.Int).Sub(p256().Params().P, a.Y)}
}

func randScalar() ([]byte, error) {
	n := p256().Params().N
	for {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		k := new(big.Int).SetBytes(b)
		if k.Sign() > 0 && k.Cmp(n) < 0 {
			return k.FillBytes(make([]byte, 32)), nil
		}
	}
}

// spakeW derives the SPAKE2 password scalar from the pairing code, bound to the account so a code
// for one account is useless for another.
func spakeW(code, account string) []byte {
	key, _ := hkdf.Key(sha256.New, []byte(normalizeCode(code)), []byte("NEXUS device link v1 "+account), "spake2 w", 48)
	w := new(big.Int).SetBytes(key)
	w.Mod(w, p256().Params().N)
	return w.FillBytes(make([]byte, 32))
}

// spakeParty is one side of a SPAKE2 exchange. The new device is A (uses M), the trusted device B (uses N).
type spakeParty struct {
	isA      bool
	idA, idB string
	w, x     []byte
	msg      []byte // our public share (pA or pB)
}

func newSpake(isA bool, code, account, idA, idB string) (*spakeParty, error) {
	x, err := randScalar()
	if err != nil {
		return nil, err
	}
	s := &spakeParty{isA: isA, idA: idA, idB: idB, w: spakeW(code, account), x: x}
	mask := spakeN
	if isA {
		mask = spakeM
	}
	s.msg = addPoints(scalarMult(mask, s.w), baseMult(x)).bytes()
	return s, nil
}

// finish takes the peer's share and returns the session key and both confirmation MACs.
func (s *spakeParty) finish(peer []byte) (ke, macA, macB []byte, err error) {
	pt, err := pointFrom(peer)
	if err != nil {
		return nil, nil, nil, err
	}
	mask := spakeM
	if s.isA {
		mask = spakeN
	}
	k := scalarMult(addPoints(pt, negPoint(scalarMult(mask, s.w))), s.x)
	if k.X.Sign() == 0 && k.Y.Sign() == 0 {
		return nil, nil, nil, errors.New("invalid exchange")
	}
	pA, pB := s.msg, peer
	if !s.isA {
		pA, pB = peer, s.msg
	}
	var tt []byte
	for _, part := range [][]byte{[]byte(s.idA), []byte(s.idB), pA, pB, k.bytes(), s.w} {
		tt = binary.LittleEndian.AppendUint64(tt, uint64(len(part)))
		tt = append(tt, part...)
	}
	h := sha256.Sum256(tt)
	ke, ka := h[:16], h[16:]
	kc, _ := hkdf.Key(sha256.New, ka, nil, "ConfirmationKeys", 64)
	ma := hmac.New(sha256.New, kc[:32])
	ma.Write(pB)
	mb := hmac.New(sha256.New, kc[32:])
	mb.Write(pA)
	sk, _ := hkdf.Key(sha256.New, ke, tt, "NEXUS link session key", 32)
	return sk, ma.Sum(nil), mb.Sum(nil), nil
}

// sealBox / openBox: AES-256-GCM with a random nonce, for the messages inside the pairing exchange.
func sealBox(key, plain, ad []byte) ([]byte, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plain, ad), nil
}

func openBox(key, box, ad []byte) ([]byte, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	if len(box) < g.NonceSize() {
		return nil, errors.New("short message")
	}
	return g.Open(nil, box[:g.NonceSize()], box[g.NonceSize():], ad)
}

// deviceCert makes the TLS certificate of this device from its Ed25519 identity key. Peers do not
// use certificate authorities: they compare the public key with the one stored at pairing.
func deviceCert(priv ed25519.PrivateKey, name string) (tls.Certificate, error) {
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "NEXUS device " + name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, priv.Public(), priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, nil
}

// peerKey returns the Ed25519 public key a TLS peer presented.
func peerKey(cs tls.ConnectionState) (ed25519.PublicKey, error) {
	if len(cs.PeerCertificates) == 0 {
		return nil, errors.New("no certificate")
	}
	k, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("not a NEXUS device certificate")
	}
	return k, nil
}

// ephemeralCert: a throw-away certificate for the pairing connection, whose security comes from
// SPAKE2, not from the certificate.
func ephemeralCert() (tls.Certificate, error) {
	k, err := ecdsa.GenerateKey(p256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}, nil
}
