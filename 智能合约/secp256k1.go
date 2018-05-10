package main

import (
	"bytes"
	crand "crypto/rand" //secure, system random number generator
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

type SECP256K1 struct {
}

var __secp256k1_ SECP256K1

func NewSecp256k1() *SECP256K1 {
	return &SECP256K1{}
}

//intenal, may fail
//may return nil
func (secp *SECP256K1) pubkeyFromSeckey(seckey []byte) ([]byte, error) {
	if len(seckey) != 32 {
		return nil, errors.New("seckey length invalid")
	}

	if __secp256k1_ec.SeckeyIsValid(seckey) != 1 {

		return nil, errors.New("always ensure seckey is valid")
	}

	var pubkey []byte = __secp256k1_ec.GeneratePublicKey(seckey) //always returns true
	if pubkey == nil {
		return nil, errors.New("ERROR: impossible, ec.BaseMultiply always returns true")

	}
	if len(pubkey) != 33 {
		return nil, errors.New("ERROR: impossible, invalid pubkey length")
	}

	if ret := __secp256k1_ec.PubkeyIsValid(pubkey); ret != 1 {
		return nil, fmt.Errorf("ERROR: pubkey invald, ret=%s", ret)
	}

	if ret := secp.VerifyPubkey(pubkey); ret != 1 {

		//log.Printf("seckey= %s", hex.EncodeToString(seckey))
		//log.Printf("pubkey= %s", hex.EncodeToString(pubkey))
		return nil, fmt.Errorf("ERROR: pubkey verification failed, for deterministic. ret=%d", ret)

	}

	return pubkey, nil
}

func (secp *SECP256K1) GenerateKeyPair() ([]byte, []byte, error) {
	const seckey_len = 32
	const tryMax = 1000
	var tryCount = 0

new_seckey:
	tryCount++
	if tryCount > tryMax {
		return nil, nil, fmt.Errorf("GenerateKeyPair: try %d times,all failed.", tryMax)
	}

	var seckey []byte = secp.RandByte(seckey_len)
	if __secp256k1_ec.SeckeyIsValid(seckey) != 1 {
		goto new_seckey
	}

	pubkey, err := secp.pubkeyFromSeckey(seckey)
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateKeyPair: pubkeyFromSeckey failed, err=%s", err)
	}
	if pubkey == nil {
		return nil, nil, errors.New("GenerateKeyPair: pubkeyFromSeckey failed, pubkey nil.")
	}

	return pubkey, seckey, nil
}

//must succeed
//TODO; hash on fail
//TOO: must match, result of private key from deterministic gen?
//deterministic gen will always return a valid private key
func (secp *SECP256K1) PubkeyFromSeckey(seckey []byte) ([]byte, error) {
	if len(seckey) != 32 {
		return nil, errors.New("PubkeyFromSeckey: invalid length")
	}

	pubkey, err := secp.pubkeyFromSeckey(seckey)
	if err != nil {
		return nil, fmt.Errorf("PubkeyFromSeckey: pubkey generation failed,err=%s", err)
	}
	if pubkey == nil {
		return nil, errors.New("PubkeyFromSeckey: pubkey nil.")
	}

	return pubkey, nil
}

func (secp *SECP256K1) UncompressPubkey(pubkey []byte) ([]byte, error) {
	if secp.VerifyPubkey(pubkey) != 1 {
		return nil, errors.New("UncompressPubkey: VerifyPubkey failed.")
	}

	var pub_xy Secp256k1XY
	if pub_xy.ParsePubkey(pubkey) == false {
		return nil, errors.New("UncompressPubkey: ParsePubkey failed.")
	}

	var pubkey2 []byte = pub_xy.BytesUncompressed() //uncompressed
	if pubkey2 == nil {
		return nil, errors.New("UncompressPubkey: BytesUncompressed failed.")
	}

	return pubkey2, nil
}

//returns nil on error
//should only need pubkey, not private key
//deprecate for _UncompressedPubkey
func (secp *SECP256K1) UncompressedPubkeyFromSeckey(seckey []byte) ([]byte, error) {

	if len(seckey) != 32 {
		return nil, errors.New("PubkeyFromSeckey: invalid length")
	}

	pubkey, err := secp.PubkeyFromSeckey(seckey)
	if err != nil || pubkey == nil {
		return nil, fmt.Errorf("Generating seckey from pubkey, failed. err=%s", err)
	}

	uncompressed_pubkey, err := secp.UncompressPubkey(pubkey)
	if err != nil || uncompressed_pubkey == nil {
		return nil, fmt.Errorf("decompression failed, err=%s", err)
	}

	return uncompressed_pubkey, nil
}

//generates deterministic keypair with weak SHA256 hash of seed
//internal use only
//be extremely careful with golang slice semantics
func (secp *SECP256K1) generateDeterministicKeyPair(seed []byte) ([]byte, []byte, error) {
	if seed == nil || len(seed) != 32 {
		return nil, nil, errors.New("generateDeterministicKeyPair:seed invalid.")
	}

	const seckey_len = 32
	var seckey []byte = make([]byte, seckey_len)

new_seckey:
	seed = secp.SumSHA256(seed[0:32])
	copy(seckey[0:32], seed[0:32])

	if bytes.Equal(seckey, seed) == false {
		return nil, nil, errors.New("generateDeterministicKeyPair:copy failed.")
	}
	if __secp256k1_ec.SeckeyIsValid(seckey) != 1 {
		goto new_seckey //regen
	}

	var pubkey []byte = __secp256k1_ec.GeneratePublicKey(seckey)

	if pubkey == nil {
		return nil, nil, errors.New("generateDeterministicKeyPair: pubkey nil.")
	}
	/*
		if len(pubkey) != 33 {
			errors.New("ERROR: impossible, pubkey length wrong")
		}

		if ret := __secp256k1_ec.PubkeyIsValid(pubkey); ret != 1 {
			fmt.Errorf("ERROR: pubkey invalid, ret=%i", ret)
		}

		if ret := secp.VerifyPubkey(pubkey); ret != 1 {
			log.Printf("seckey= %s", hex.EncodeToString(seckey))
			log.Printf("pubkey= %s", hex.EncodeToString(pubkey))

			fmt.Errorf("ERROR: pubkey is invalid, for deterministic. ret=%i", ret)
			goto new_seckey
		}
	*/

	return pubkey, seckey, nil
}

//double SHA256, salted with ECDH operation in curve
func (secp *SECP256K1) Secp256k1Hash(hash []byte) []byte {
	hash = secp.SumSHA256(hash)
	_, seckey, _ := secp.generateDeterministicKeyPair(hash)                 //seckey1 is usually sha256 of hash
	pubkey, _, _ := secp.generateDeterministicKeyPair(secp.SumSHA256(hash)) //SumSHA256(hash) equals seckey usually
	ecdh, _ := secp.ECDH(pubkey, seckey)                                    //raise pubkey to power of seckey in curve
	return secp.SumSHA256(append(hash, ecdh...))                            //append signature to sha256(seed) and hash
}

//generate a single secure key
func (secp *SECP256K1) GenerateDeterministicKeyPair(seed []byte) ([]byte, []byte, error) {
	_, pubkey, seckey, err := secp.DeterministicKeyPairIterator(seed)
	return pubkey, seckey, err
}

//Iterator for deterministic keypair generation. Returns SHA256, Pubkey, Seckey
//Feed SHA256 back into function to generate sequence of seckeys
//If private key is diclosed, should not be able to compute future or past keys in sequence
func (secp *SECP256K1) DeterministicKeyPairIterator(seed_in []byte) ([]byte, []byte, []byte, error) {
	seed1 := secp.Secp256k1Hash(seed_in) //make it difficult to derive future seckeys from previous seckeys
	seed2 := secp.SumSHA256(append(seed_in, seed1...))
	pubkey, seckey, err := secp.generateDeterministicKeyPair(seed2) //this is our seckey
	return seed1, pubkey, seckey, err
}

//Rename SignHash
func (secp *SECP256K1) Sign(msg []byte, seckey []byte) ([]byte, error) {

	if len(seckey) != 32 {
		return nil, errors.New("Sign, Invalid seckey length")
	}
	if __secp256k1_ec.SeckeyIsValid(seckey) != 1 {
		return nil, errors.New("Sign, Invalid seckey")
	}
	if msg == nil {
		return nil, errors.New("Sign, Invalid seckey")
	}
	var nonce []byte = secp.RandByte(32)
	if nonce == nil {
		return nil, errors.New("Sign, Invalid nonce")
	}
	var sig []byte = make([]byte, 65)
	var recid int

	var cSig Secp256k1Signature

	var seckey1 Secp256k1Number
	var msg1 Secp256k1Number
	var nonce1 Secp256k1Number

	seckey1.SetBytes(seckey)
	msg1.SetBytes(msg)
	nonce1.SetBytes(nonce)

	ret := cSig.Sign(&seckey1, &msg1, &nonce1, &recid)

	if ret != 1 {
		return nil, errors.New("Sign, signature operation failed")
	}

	sig_bytes := cSig.Bytes()
	for i := 0; i < 64; i++ {
		sig[i] = sig_bytes[i]
	}
	if len(sig_bytes) != 64 {
		return nil, fmt.Errorf("Sign, Invalid signature byte count: %s", len(sig_bytes))
	}
	sig[64] = byte(int(recid))

	if int(recid) > 4 {
		return nil, fmt.Errorf("Sign, Invalid recid")
	}

	return sig, nil
}

//generate signature in repeatable way
func (secp *SECP256K1) SignDeterministic(msg []byte, seckey []byte, nonce_seed []byte) ([]byte, error) {
	nonce_seed2 := secp.SumSHA256(nonce_seed) //deterministicly generate nonce

	var sig []byte = make([]byte, 65)
	var recid int

	var cSig Secp256k1Signature

	var seckey1 Secp256k1Number
	var msg1 Secp256k1Number
	var nonce1 Secp256k1Number

	seckey1.SetBytes(seckey)
	msg1.SetBytes(msg)
	nonce1.SetBytes(nonce_seed2)

	ret := cSig.Sign(&seckey1, &msg1, &nonce1, &recid)
	if ret != 1 {
		return nil, fmt.Errorf("SignDeterministic, Sign failed.")
	}

	sig_bytes := cSig.Bytes()
	for i := 0; i < 64; i++ {
		sig[i] = sig_bytes[i]
	}

	sig[64] = byte(recid)

	if len(sig_bytes) != 64 {
		return nil, fmt.Errorf("SignDeterministic, Invalid signature byte count: %s", len(sig_bytes))
	}

	if int(recid) > 4 {
		return nil, fmt.Errorf("SignDeterministic, Invalid recid")
	}

	return sig, nil

}

//Rename ChkSeckeyValidity
func (secp *SECP256K1) VerifySeckey(seckey []byte) int {
	if len(seckey) != 32 {
		return -1
	}

	//does conversion internally if less than order of curve
	if __secp256k1_ec.SeckeyIsValid(seckey) != 1 {
		return -2
	}

	//seckey is just 32 bit integer
	//assume all seckey are valid
	//no. must be less than order of curve
	//note: converts internally
	return 1
}

/*
* Validate a public key.
*  Returns: 1: valid public key
*           0: invalid public key
 */

//Rename ChkPubkeyValidity
// returns 1 on success
func (secp *SECP256K1) VerifyPubkey(pubkey []byte) int {
	if len(pubkey) != 33 {
		//log.Printf("Seck256k1, VerifyPubkey, pubkey length invalid")
		return -1
	}

	if __secp256k1_ec.PubkeyIsValid(pubkey) != 1 {
		return -3 //tests parse and validity
	}

	var pubkey1 Secp256k1XY
	ret := pubkey1.ParsePubkey(pubkey)

	if ret == false {
		return -2 //invalid, parse fail
	}
	//fails for unknown reason
	//TODO: uncomment
	if pubkey1.IsValid() == false {
		return -4 //invalid, validation fail
	}
	return 1 //valid
}

//Rename ChkSignatureValidity
func (secp *SECP256K1) VerifySignatureValidity(sig []byte) int {
	//64+1
	if len(sig) != 65 {
		return -1
	}
	//malleability check:
	//highest bit of 32nd byte must be 1
	//0x7f us 126 or 0b01111111
	if (sig[32] >> 7) == 1 {
		return -2
	}
	//recovery id check
	if sig[64] >= 4 {
		return -3
	}
	return 1
}

//for compressed signatures, does not need pubkey
//Rename SignatureChk
func (secp *SECP256K1) VerifySignature(msg []byte, sig []byte, pubkey1 []byte) int {
	if msg == nil || sig == nil || pubkey1 == nil {
		return -1
	}
	if len(sig) != 65 {
		return -2
	}
	if len(pubkey1) != 33 {
		return -3
	}

	//malleability check:
	//to enforce malleability, highest bit of S must be 1
	//S starts at 32nd byte
	//0x80 is 0b10000000 or 128 and masks highest bit
	if (sig[32] >> 7) == 1 {
		return -4
	}

	if sig[64] >= 4 {
		return -5
	}

	pubkey2, err := secp.RecoverPubkey(msg, sig) //if pubkey recovered, signature valid

	if err != nil || pubkey2 == nil {
		return -6
	}

	if len(pubkey2) != 33 {
		return -7
	}

	if bytes.Equal(pubkey1, pubkey2) != true {
		return -8
	}

	return 1 //valid signature
}

/*
//SignatureErrorString returns error string for signature failure
func (secp *SECP256K1) SignatureErrorString(msg []byte, sig []byte, pubkey1 []byte) string {

	if msg == nil || len(sig) != 65 || len(pubkey1) != 33 {
		errors.New()
	}

	if (sig[32] >> 7) == 1 {
		return "signature fails malleability requirement"
	}

	if sig[64] >= 4 {
		return "signature recovery byte is invalid, must be 0 to 3"
	}

	pubkey2 := secp.RecoverPubkey(msg, sig) //if pubkey recovered, signature valid
	if pubkey2 == nil {
		return "pubkey from signature failed"
	}

	if bytes.Equal(pubkey1, pubkey2) == false {
		return "input pubkey and recovered pubkey do not match"
	}

	return "No Error!"
}
*/

//recovers the public key from the signature
//recovery of pubkey means correct signature
func (secp *SECP256K1) RecoverPubkey(msg []byte, sig []byte) ([]byte, error) {
	if len(sig) != 65 {
		return nil, errors.New("RecoverPubkey: sig invalid.")
	}

	var recid int = int(sig[64])

	pubkey, ret := __secp256k1_ec.RecoverPublicKey(
		sig[0:64],
		msg,
		recid)

	if ret != 1 {
		return nil, errors.New("RecoverPubkey: RecoverPublicKey failed.")
	}
	//var pubkey2 []byte = pubkey1.Bytes() //compressed

	if pubkey == nil {
		return nil, errors.New("RecoverPubkey: pubkey nil.")
	}
	if len(pubkey) != 33 {
		return nil, errors.New("RecoverPubkey: pubkey length wrong.")
	}

	return pubkey, nil
	//nonce1.SetBytes(nonce_seed)

}

//raise a pubkey to the power of a seckey
func (secp *SECP256K1) ECDH(pub []byte, sec []byte) ([]byte, error) {
	if len(sec) != 32 {
		return nil, errors.New("ECDH: sec wrong.")
	}

	if len(pub) != 33 {
		return nil, errors.New("ECDH: pub wrong.")
	}

	if secp.VerifySeckey(sec) != 1 {
		return nil, errors.New("ECDH: Invalid Seckey.")
	}

	if ret := secp.VerifyPubkey(pub); ret != 1 {
		return nil, fmt.Errorf("ECDH: VerifyPubkey, failed, err=.", ret)
	}

	pubkey_out := __secp256k1_ec.Multiply(pub, sec)
	if pubkey_out == nil {
		return nil, fmt.Errorf("ECDH: pubkey_out nil.")
	}
	if len(pubkey_out) != 33 {
		return nil, fmt.Errorf("ECDH: pubkey_out invalid.")
	}
	return pubkey_out, nil
}

var (
	__secp256k1_sha256Hash hash.Hash = sha256.New()
)

func (secp *SECP256K1) SumSHA256(b []byte) []byte {
	__secp256k1_sha256Hash.Reset()
	__secp256k1_sha256Hash.Write(b)
	sum := __secp256k1_sha256Hash.Sum(nil)
	return sum[:]
}

/*
Entropy pool needs
- state (an array of bytes)
- a compression function (two 256 bit blocks to single block)
- a mixing function across the pool

- Xor is safe, as it cannot make value less random
-- apply compression function, then xor with current value
--

*/

type Secp256k1EntropyPool struct {
	Ent [32]byte //256 bit accumulator

}

//mixes in 256 bits, outputs 256 bits
func (self *Secp256k1EntropyPool) Mix256(in []byte) (out []byte) {

	//hash input
	val1 := __secp256k1_.SumSHA256(in)
	//return value
	val2 := __secp256k1_.SumSHA256(append(val1, self.Ent[:]...))
	//next ent value
	val3 := __secp256k1_.SumSHA256(append(val1, val2...))

	for i := 0; i < 32; i++ {
		self.Ent[i] = val3[i]
		val3[i] = 0x00
	}

	return val2
}

//take in N bytes, salts, return N
func (self *Secp256k1EntropyPool) Mix(in []byte) []byte {
	var length int = len(in) - len(in)%32 + 32
	var buff []byte = make([]byte, length, length)
	for i := 0; i < len(in); i++ {
		buff[i] = in[i]
	}
	var iterations int = (len(in) / 32) + 1
	for i := 0; i < iterations; i++ {
		tmp := self.Mix256(buff[32*i : 32+32*i]) //32 byte slice
		for j := 0; j < 32; j++ {
			buff[i*32+j] = tmp[j]
		}
	}
	return buff[:len(in)]
}

/*
Note:

- On windows cryto/rand uses CrytoGenRandom which uses RC4 which is insecure
- Android random number generator is known to be insecure.
- Linux uses /dev/urandom , which is thought to be secure and uses entropy pool

Therefore the output is salted.
*/

/*
Note:

Should allow pseudo-random mode for repeatability for certain types of tests

*/

//var _rand *mrand.Rand //pseudorandom number generator
var __secp256k1_ent Secp256k1EntropyPool

//seed pseudo random number generator with
// hash of system time in nano seconds
// hash of system environmental variables
// hash of process id
func init() {
	var seed1 []byte = []byte(strconv.FormatUint(uint64(time.Now().UnixNano()), 16))
	var seed2 []byte = []byte(strings.Join(os.Environ(), ""))
	var seed3 []byte = []byte(strconv.FormatUint(uint64(os.Getpid()), 16))

	seed4 := make([]byte, 256)
	io.ReadFull(crand.Reader, seed4) //system secure random number generator

	//mrand.Rand_rand = mrand.New(mrand.NewSource(int64(time.Now().UnixNano()))) //pseudo random
	//seed entropy pool
	__secp256k1_ent.Mix256(seed1)
	__secp256k1_ent.Mix256(seed2)
	__secp256k1_ent.Mix256(seed3)
	__secp256k1_ent.Mix256(seed4)
}

//Secure Random number generator for forwards security
//On Unix-like systems, Reader reads from /dev/urandom.
//On Windows systems, Reader uses the CryptGenRandom API.
//Pseudo-random sequence, seeded from program start time, environmental variables,
//and process id is mixed in for forward security. Future version should use entropy pool
func (secp *SECP256K1) RandByte(n int) []byte {
	buff := make([]byte, n)
	ret, err := io.ReadFull(crand.Reader, buff) //system secure random number generator
	if len(buff) != ret || err != nil {
		return nil
	}

	//XORing in sequence, cannot reduce security (even if sequence is bad/known/non-random)

	buff2 := __secp256k1_ent.Mix(buff)
	for i := 0; i < n; i++ {
		buff[i] ^= buff2[i]
	}
	return buff
}

type Secp256k1EC struct {
}

var __secp256k1_ec Secp256k1EC

/**********************************************************************/
/****************************** -ec- **********************************/
/**********************************************************************/
func (ec *Secp256k1EC) ecdsa_verify(pubkey, sig, msg []byte) int {
	var m Secp256k1Number
	var s Secp256k1Signature
	m.SetBytes(msg)

	var q Secp256k1XY
	if !q.ParsePubkey(pubkey) {
		return -1
	}

	//if s.ParseBytes(sig) < 0 {
	//	return -2
	//}
	if len(pubkey) != 32 {
		return -2
	}
	if len(sig) != 64 {
		return -3
	}

	if !s.Verify(&q, &m) {
		return 0
	}
	return 1
}

func (ec *Secp256k1EC) Verify(k, s, m []byte) bool {
	return ec.ecdsa_verify(k, s, m) == 1
}

func (ec *Secp256k1EC) DecompressPoint(X []byte, off bool, Y []byte) {
	var rx, ry, c, x2, x3 Secp256k1Field
	rx.SetB32(X)
	rx.Sqr(&x2)
	rx.Mul(&x3, &x2)
	c.SetInt(7)
	c.SetAdd(&x3)
	c.Sqrt(&ry)
	ry.Normalize()
	if ry.IsOdd() != off {
		ry.Negate(&ry, 1)
	}
	ry.Normalize()
	ry.GetB32(Y)
	return
}

//nil on error
//returns error code
func (ec *Secp256k1EC) RecoverPublicKey(sig_byte []byte, h []byte, recid int) ([]byte, int) {

	var pubkey Secp256k1XY

	if len(sig_byte) != 64 {
		return nil, -7
	}

	var sig Secp256k1Signature
	sig.ParseBytes(sig_byte[0:64])

	//var sig Signature
	var msg Secp256k1Number

	if sig.R.Sign() <= 0 || sig.R.Cmp(&__Secp256k1_TheCurve.Order.Int) >= 0 {
		if sig.R.Sign() == 0 {
			return nil, -1
		}
		if sig.R.Sign() <= 0 {
			return nil, -2
		}
		if sig.R.Cmp(&__Secp256k1_TheCurve.Order.Int) >= 0 {
			return nil, -3
		}
		return nil, -4
	}
	if sig.S.Sign() <= 0 || sig.S.Cmp(&__Secp256k1_TheCurve.Order.Int) >= 0 {
		return nil, -5
	}

	msg.SetBytes(h)
	if !sig.Recover(&pubkey, &msg, recid) {
		return nil, -6
	}

	return pubkey.Bytes(), 1
}

// Standard EC multiplacation k(xy)
// xy - is the standarized public key format (33 or 65 bytes long)
// out - should be the buffer for 33 bytes (1st byte will be set to either 02 or 03)
// TODO: change out to return type
func (ec *Secp256k1EC) Multiply(xy, k []byte) []byte {
	var pk Secp256k1XY
	var xyz Secp256k1XYZ
	var na, nzero Secp256k1Number
	if !pk.ParsePubkey(xy) {
		return nil
	}
	xyz.SetXY(&pk)
	na.SetBytes(k)
	xyz.ECmult(&xyz, &na, &nzero)
	pk.SetXYZ(&xyz)

	if pk.IsValid() == false {
		return nil
	}
	return pk.GetPublicKey()
}

func (ec *Secp256k1EC) _pubkey_test(pk Secp256k1XY) int {

	if pk.IsValid() == false {
		return -1
	}
	var pk2 Secp256k1XY
	retb := pk2.ParsePubkey(pk.Bytes())
	if retb == false {
		return -2
	}
	if pk2.IsValid() == false {
		return -3
	}
	if ec.PubkeyIsValid(pk2.Bytes()) != 1 {
		return -4
	}

	return 0

}

func (ec *Secp256k1EC) BaseMultiply(k []byte) []byte {
	var r Secp256k1XYZ
	var n Secp256k1Number
	var pk Secp256k1XY
	n.SetBytes(k)
	secp256k1XYZECmultGen(&r, &n)
	pk.SetXYZ(&r)
	if pk.IsValid() == false {
		//should not occur
		return nil
	}

	if ec._pubkey_test(pk) != 0 {
		return nil
	}

	return pk.Bytes()
}

// out = G*k + xy
// TODO: switch to returning output as []byte
// nil on error
// 33 byte out
func (ec *Secp256k1EC) BaseMultiplyAdd(xy, k []byte) []byte {
	var r Secp256k1XYZ
	var n Secp256k1Number
	var pk Secp256k1XY
	if !pk.ParsePubkey(xy) {
		return nil
	}
	n.SetBytes(k)
	secp256k1XYZECmultGen(&r, &n)
	r.AddXY(&r, &pk)
	pk.SetXYZ(&r)

	if ec._pubkey_test(pk) != 0 {
		return nil
	}
	return pk.Bytes()
}

//returns nil on failure
//crash rather than fail
func (ec *Secp256k1EC) GeneratePublicKey(k []byte) []byte {

	//errors.New()
	if len(k) != 32 {
		log.Println("GeneratePublicKey err 1")
		return nil
	}
	var r Secp256k1XYZ
	var n Secp256k1Number
	var pk Secp256k1XY

	//must not be zero
	//must not be negative
	//must be less than order of curve
	n.SetBytes(k)
	if n.Sign() <= 0 || n.Cmp(&__Secp256k1_TheCurve.Order.Int) >= 0 {
		log.Println("GeneratePublicKey err 2")
		return nil
	}
	secp256k1XYZECmultGen(&r, &n)
	pk.SetXYZ(&r)
	if pk.IsValid() == false {
		//should not occur
		log.Println("GeneratePublicKey err 3")
		return nil
	}
	if ec._pubkey_test(pk) != 0 {
		log.Println("GeneratePublicKey err 4")
		return nil
	}
	return pk.Bytes()
}

//1 on success
//must not be zero
// must not be negative
//must be less than order of curve
func (ec *Secp256k1EC) SeckeyIsValid(seckey []byte) int {
	if len(seckey) != 32 {
		return 0
	}
	var n Secp256k1Number
	n.SetBytes(seckey)
	//must not be zero
	//must not be negative
	//must be less than order of curve
	if n.Sign() <= 0 {
		return -1
	}
	if n.Cmp(&__Secp256k1_TheCurve.Order.Int) >= 0 {
		return -2
	}
	return 1
}

//returns 1 on success
func (ec *Secp256k1EC) PubkeyIsValid(pubkey []byte) int {
	if len(pubkey) != 33 {
		return -3
	}
	var pub_test Secp256k1XY
	err := pub_test.ParsePubkey(pubkey)
	if err == false {
		//errors.New("PubkeyIsValid, ERROR: pubkey parse fail, bad pubkey from private key")
		return -1
	}
	if bytes.Equal(pub_test.Bytes(), pubkey) == false {
		return -2
	}
	//this fails
	//if pub_test.IsValid() == false {
	//	return -2
	//}
	return 1
}

/**********************************************************************/
/****************************** -field- *******************************/
/**********************************************************************/

type Secp256k1Field struct {
	n [10]uint32
}

func (a *Secp256k1Field) String() string {
	var tmp [32]byte
	b := *a
	b.Normalize()
	b.GetB32(tmp[:])
	return hex.EncodeToString(tmp[:])
}

func (a *Secp256k1Field) Print(lab string) {
	fmt.Println(lab+":", a.String())
}

func (a *Secp256k1Field) GetBig() (r *big.Int) {
	a.Normalize()
	r = new(big.Int)
	var tmp [32]byte
	a.GetB32(tmp[:])
	r.SetBytes(tmp[:])
	return
}

func (r *Secp256k1Field) SetB32(a []byte) {
	r.n[0] = 0
	r.n[1] = 0
	r.n[2] = 0
	r.n[3] = 0
	r.n[4] = 0
	r.n[5] = 0
	r.n[6] = 0
	r.n[7] = 0
	r.n[8] = 0
	r.n[9] = 0
	for i := uint(0); i < 32; i++ {
		for j := uint(0); j < 4; j++ {
			limb := (8*i + 2*j) / 26
			shift := (8*i + 2*j) % 26
			r.n[limb] |= (uint32)((a[31-i]>>(2*j))&0x3) << shift
		}
	}
}

func (r *Secp256k1Field) SetBytes(a []byte) error {
	if len(a) > 32 {
		return errors.New("too many bytes to set")
	}
	if len(a) == 32 {
		r.SetB32(a)
	} else {
		var buf [32]byte
		copy(buf[32-len(a):], a)
		r.SetB32(buf[:])
	}

	return nil
}

func (r *Secp256k1Field) SetHex(s string) {
	d, _ := hex.DecodeString(s)
	r.SetBytes(d)
}

func (a *Secp256k1Field) IsOdd() bool {
	return (a.n[0] & 1) != 0
}

func (a *Secp256k1Field) IsZero() bool {
	return (a.n[0] == 0 && a.n[1] == 0 && a.n[2] == 0 && a.n[3] == 0 && a.n[4] == 0 && a.n[5] == 0 && a.n[6] == 0 && a.n[7] == 0 && a.n[8] == 0 && a.n[9] == 0)
}

func (r *Secp256k1Field) SetInt(a uint32) {
	r.n[0] = a
	r.n[1] = 0
	r.n[2] = 0
	r.n[3] = 0
	r.n[4] = 0
	r.n[5] = 0
	r.n[6] = 0
	r.n[7] = 0
	r.n[8] = 0
	r.n[9] = 0
}

func (r *Secp256k1Field) Normalize() {
	c := r.n[0]
	t0 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[1]
	t1 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[2]
	t2 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[3]
	t3 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[4]
	t4 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[5]
	t5 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[6]
	t6 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[7]
	t7 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[8]
	t8 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[9]
	t9 := c & 0x03FFFFF
	c >>= 22

	// The following code will not modify the t's if c is initially 0.
	d := c*0x3D1 + t0
	t0 = d & 0x3FFFFFF
	d = (d >> 26) + t1 + c*0x40
	t1 = d & 0x3FFFFFF
	d = (d >> 26) + t2
	t2 = d & 0x3FFFFFF
	d = (d >> 26) + t3
	t3 = d & 0x3FFFFFF
	d = (d >> 26) + t4
	t4 = d & 0x3FFFFFF
	d = (d >> 26) + t5
	t5 = d & 0x3FFFFFF
	d = (d >> 26) + t6
	t6 = d & 0x3FFFFFF
	d = (d >> 26) + t7
	t7 = d & 0x3FFFFFF
	d = (d >> 26) + t8
	t8 = d & 0x3FFFFFF
	d = (d >> 26) + t9
	t9 = d & 0x03FFFFF

	// Subtract p if result >= p
	low := (uint64(t1) << 26) | uint64(t0)
	//mask := uint64(-(int64)((t9 < 0x03FFFFF) | (t8 < 0x3FFFFFF) | (t7 < 0x3FFFFFF) | (t6 < 0x3FFFFFF) | (t5 < 0x3FFFFFF) | (t4 < 0x3FFFFFF) | (t3 < 0x3FFFFFF) | (t2 < 0x3FFFFFF) | (low < 0xFFFFEFFFFFC2F)))
	var mask uint64
	if (t9 < 0x03FFFFF) ||
		(t8 < 0x3FFFFFF) ||
		(t7 < 0x3FFFFFF) ||
		(t6 < 0x3FFFFFF) ||
		(t5 < 0x3FFFFFF) ||
		(t4 < 0x3FFFFFF) ||
		(t3 < 0x3FFFFFF) ||
		(t2 < 0x3FFFFFF) ||
		(low < 0xFFFFEFFFFFC2F) {
		mask = 0xFFFFFFFFFFFFFFFF
	}
	t9 &= uint32(mask)
	t8 &= uint32(mask)
	t7 &= uint32(mask)
	t6 &= uint32(mask)
	t5 &= uint32(mask)
	t4 &= uint32(mask)
	t3 &= uint32(mask)
	t2 &= uint32(mask)
	low -= ((mask ^ 0xFFFFFFFFFFFFFFFF) & 0xFFFFEFFFFFC2F)

	// push internal variables back
	r.n[0] = uint32(low) & 0x3FFFFFF
	r.n[1] = uint32(low>>26) & 0x3FFFFFF
	r.n[2] = t2
	r.n[3] = t3
	r.n[4] = t4
	r.n[5] = t5
	r.n[6] = t6
	r.n[7] = t7
	r.n[8] = t8
	r.n[9] = t9
}

func (a *Secp256k1Field) GetB32(r []byte) {
	var i, j, c, limb, shift uint32
	for i = 0; i < 32; i++ {
		c = 0
		for j = 0; j < 4; j++ {
			limb = (8*i + 2*j) / 26
			shift = (8*i + 2*j) % 26
			c |= ((a.n[limb] >> shift) & 0x3) << (2 * j)
		}
		r[31-i] = byte(c)
	}
}

func (a *Secp256k1Field) Equals(b *Secp256k1Field) bool {
	return (a.n[0] == b.n[0] && a.n[1] == b.n[1] && a.n[2] == b.n[2] && a.n[3] == b.n[3] && a.n[4] == b.n[4] &&
		a.n[5] == b.n[5] && a.n[6] == b.n[6] && a.n[7] == b.n[7] && a.n[8] == b.n[8] && a.n[9] == b.n[9])
}

func (r *Secp256k1Field) SetAdd(a *Secp256k1Field) {
	r.n[0] += a.n[0]
	r.n[1] += a.n[1]
	r.n[2] += a.n[2]
	r.n[3] += a.n[3]
	r.n[4] += a.n[4]
	r.n[5] += a.n[5]
	r.n[6] += a.n[6]
	r.n[7] += a.n[7]
	r.n[8] += a.n[8]
	r.n[9] += a.n[9]
}

func (r *Secp256k1Field) MulInt(a uint32) {
	r.n[0] *= a
	r.n[1] *= a
	r.n[2] *= a
	r.n[3] *= a
	r.n[4] *= a
	r.n[5] *= a
	r.n[6] *= a
	r.n[7] *= a
	r.n[8] *= a
	r.n[9] *= a
}

func (a *Secp256k1Field) Negate(r *Secp256k1Field, m uint32) {
	r.n[0] = 0x3FFFC2F*(m+1) - a.n[0]
	r.n[1] = 0x3FFFFBF*(m+1) - a.n[1]
	r.n[2] = 0x3FFFFFF*(m+1) - a.n[2]
	r.n[3] = 0x3FFFFFF*(m+1) - a.n[3]
	r.n[4] = 0x3FFFFFF*(m+1) - a.n[4]
	r.n[5] = 0x3FFFFFF*(m+1) - a.n[5]
	r.n[6] = 0x3FFFFFF*(m+1) - a.n[6]
	r.n[7] = 0x3FFFFFF*(m+1) - a.n[7]
	r.n[8] = 0x3FFFFFF*(m+1) - a.n[8]
	r.n[9] = 0x03FFFFF*(m+1) - a.n[9]
}

/* New algo by peterdettman - https://github.com/sipa/TheCurve/pull/19 */
func (a *Secp256k1Field) Inv(r *Secp256k1Field) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 Secp256k1Field
	var j int

	a.Sqr(&x2)
	x2.Mul(&x2, a)

	x2.Sqr(&x3)
	x3.Mul(&x3, a)

	x3.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Mul(&x6, &x3)

	x6.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Mul(&x9, &x3)

	x9.Sqr(&x11)
	x11.Sqr(&x11)
	x11.Mul(&x11, &x2)

	x11.Sqr(&x22)
	for j = 1; j < 11; j++ {
		x22.Sqr(&x22)
	}
	x22.Mul(&x22, &x11)

	x22.Sqr(&x44)
	for j = 1; j < 22; j++ {
		x44.Sqr(&x44)
	}
	x44.Mul(&x44, &x22)

	x44.Sqr(&x88)
	for j = 1; j < 44; j++ {
		x88.Sqr(&x88)
	}
	x88.Mul(&x88, &x44)

	x88.Sqr(&x176)
	for j = 1; j < 88; j++ {
		x176.Sqr(&x176)
	}
	x176.Mul(&x176, &x88)

	x176.Sqr(&x220)
	for j = 1; j < 44; j++ {
		x220.Sqr(&x220)
	}
	x220.Mul(&x220, &x44)

	x220.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Mul(&x223, &x3)

	x223.Sqr(&t1)
	for j = 1; j < 23; j++ {
		t1.Sqr(&t1)
	}
	t1.Mul(&t1, &x22)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Mul(&t1, a)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Mul(&t1, &x2)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Mul(r, a)
}

/* New algo by peterdettman - https://github.com/sipa/TheCurve/pull/19 */
func (a *Secp256k1Field) Sqrt(r *Secp256k1Field) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 Secp256k1Field
	var j int

	a.Sqr(&x2)
	x2.Mul(&x2, a)

	x2.Sqr(&x3)
	x3.Mul(&x3, a)

	x3.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Mul(&x6, &x3)

	x6.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Mul(&x9, &x3)

	x9.Sqr(&x11)
	x11.Sqr(&x11)
	x11.Mul(&x11, &x2)

	x11.Sqr(&x22)
	for j = 1; j < 11; j++ {
		x22.Sqr(&x22)
	}
	x22.Mul(&x22, &x11)

	x22.Sqr(&x44)
	for j = 1; j < 22; j++ {
		x44.Sqr(&x44)
	}
	x44.Mul(&x44, &x22)

	x44.Sqr(&x88)
	for j = 1; j < 44; j++ {
		x88.Sqr(&x88)
	}
	x88.Mul(&x88, &x44)

	x88.Sqr(&x176)
	for j = 1; j < 88; j++ {
		x176.Sqr(&x176)
	}
	x176.Mul(&x176, &x88)

	x176.Sqr(&x220)
	for j = 1; j < 44; j++ {
		x220.Sqr(&x220)
	}
	x220.Mul(&x220, &x44)

	x220.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Mul(&x223, &x3)

	x223.Sqr(&t1)
	for j = 1; j < 23; j++ {
		t1.Sqr(&t1)
	}
	t1.Mul(&t1, &x22)
	for j = 0; j < 6; j++ {
		t1.Sqr(&t1)
	}
	t1.Mul(&t1, &x2)
	t1.Sqr(&t1)
	t1.Sqr(r)
}

func (a *Secp256k1Field) InvVar(r *Secp256k1Field) {
	var b [32]byte
	var c Secp256k1Field
	c = *a
	c.Normalize()
	c.GetB32(b[:])
	var n Secp256k1Number
	n.SetBytes(b[:])
	n.mod_inv(&n, &__Secp256k1_TheCurve.p)
	r.SetBytes(n.Bytes())
}

func (a *Secp256k1Field) Mul(r, b *Secp256k1Field) {
	var c, d uint64
	var t0, t1, t2, t3, t4, t5, t6 uint64
	var t7, t8, t9, t10, t11, t12, t13 uint64
	var t14, t15, t16, t17, t18, t19 uint64

	c = uint64(a.n[0]) * uint64(b.n[0])
	t0 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[1]) +
		uint64(a.n[1])*uint64(b.n[0])
	t1 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[2]) +
		uint64(a.n[1])*uint64(b.n[1]) +
		uint64(a.n[2])*uint64(b.n[0])
	t2 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[3]) +
		uint64(a.n[1])*uint64(b.n[2]) +
		uint64(a.n[2])*uint64(b.n[1]) +
		uint64(a.n[3])*uint64(b.n[0])
	t3 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[4]) +
		uint64(a.n[1])*uint64(b.n[3]) +
		uint64(a.n[2])*uint64(b.n[2]) +
		uint64(a.n[3])*uint64(b.n[1]) +
		uint64(a.n[4])*uint64(b.n[0])
	t4 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[5]) +
		uint64(a.n[1])*uint64(b.n[4]) +
		uint64(a.n[2])*uint64(b.n[3]) +
		uint64(a.n[3])*uint64(b.n[2]) +
		uint64(a.n[4])*uint64(b.n[1]) +
		uint64(a.n[5])*uint64(b.n[0])
	t5 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[6]) +
		uint64(a.n[1])*uint64(b.n[5]) +
		uint64(a.n[2])*uint64(b.n[4]) +
		uint64(a.n[3])*uint64(b.n[3]) +
		uint64(a.n[4])*uint64(b.n[2]) +
		uint64(a.n[5])*uint64(b.n[1]) +
		uint64(a.n[6])*uint64(b.n[0])
	t6 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[7]) +
		uint64(a.n[1])*uint64(b.n[6]) +
		uint64(a.n[2])*uint64(b.n[5]) +
		uint64(a.n[3])*uint64(b.n[4]) +
		uint64(a.n[4])*uint64(b.n[3]) +
		uint64(a.n[5])*uint64(b.n[2]) +
		uint64(a.n[6])*uint64(b.n[1]) +
		uint64(a.n[7])*uint64(b.n[0])
	t7 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[8]) +
		uint64(a.n[1])*uint64(b.n[7]) +
		uint64(a.n[2])*uint64(b.n[6]) +
		uint64(a.n[3])*uint64(b.n[5]) +
		uint64(a.n[4])*uint64(b.n[4]) +
		uint64(a.n[5])*uint64(b.n[3]) +
		uint64(a.n[6])*uint64(b.n[2]) +
		uint64(a.n[7])*uint64(b.n[1]) +
		uint64(a.n[8])*uint64(b.n[0])
	t8 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[9]) +
		uint64(a.n[1])*uint64(b.n[8]) +
		uint64(a.n[2])*uint64(b.n[7]) +
		uint64(a.n[3])*uint64(b.n[6]) +
		uint64(a.n[4])*uint64(b.n[5]) +
		uint64(a.n[5])*uint64(b.n[4]) +
		uint64(a.n[6])*uint64(b.n[3]) +
		uint64(a.n[7])*uint64(b.n[2]) +
		uint64(a.n[8])*uint64(b.n[1]) +
		uint64(a.n[9])*uint64(b.n[0])
	t9 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[1])*uint64(b.n[9]) +
		uint64(a.n[2])*uint64(b.n[8]) +
		uint64(a.n[3])*uint64(b.n[7]) +
		uint64(a.n[4])*uint64(b.n[6]) +
		uint64(a.n[5])*uint64(b.n[5]) +
		uint64(a.n[6])*uint64(b.n[4]) +
		uint64(a.n[7])*uint64(b.n[3]) +
		uint64(a.n[8])*uint64(b.n[2]) +
		uint64(a.n[9])*uint64(b.n[1])
	t10 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[2])*uint64(b.n[9]) +
		uint64(a.n[3])*uint64(b.n[8]) +
		uint64(a.n[4])*uint64(b.n[7]) +
		uint64(a.n[5])*uint64(b.n[6]) +
		uint64(a.n[6])*uint64(b.n[5]) +
		uint64(a.n[7])*uint64(b.n[4]) +
		uint64(a.n[8])*uint64(b.n[3]) +
		uint64(a.n[9])*uint64(b.n[2])
	t11 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[3])*uint64(b.n[9]) +
		uint64(a.n[4])*uint64(b.n[8]) +
		uint64(a.n[5])*uint64(b.n[7]) +
		uint64(a.n[6])*uint64(b.n[6]) +
		uint64(a.n[7])*uint64(b.n[5]) +
		uint64(a.n[8])*uint64(b.n[4]) +
		uint64(a.n[9])*uint64(b.n[3])
	t12 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[4])*uint64(b.n[9]) +
		uint64(a.n[5])*uint64(b.n[8]) +
		uint64(a.n[6])*uint64(b.n[7]) +
		uint64(a.n[7])*uint64(b.n[6]) +
		uint64(a.n[8])*uint64(b.n[5]) +
		uint64(a.n[9])*uint64(b.n[4])
	t13 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[5])*uint64(b.n[9]) +
		uint64(a.n[6])*uint64(b.n[8]) +
		uint64(a.n[7])*uint64(b.n[7]) +
		uint64(a.n[8])*uint64(b.n[6]) +
		uint64(a.n[9])*uint64(b.n[5])
	t14 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[6])*uint64(b.n[9]) +
		uint64(a.n[7])*uint64(b.n[8]) +
		uint64(a.n[8])*uint64(b.n[7]) +
		uint64(a.n[9])*uint64(b.n[6])
	t15 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[7])*uint64(b.n[9]) +
		uint64(a.n[8])*uint64(b.n[8]) +
		uint64(a.n[9])*uint64(b.n[7])
	t16 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[8])*uint64(b.n[9]) +
		uint64(a.n[9])*uint64(b.n[8])
	t17 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[9])*uint64(b.n[9])
	t18 = c & 0x3FFFFFF
	c = c >> 26
	t19 = c

	c = t0 + t10*0x3D10
	t0 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t1 + t10*0x400 + t11*0x3D10
	t1 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t2 + t11*0x400 + t12*0x3D10
	t2 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t3 + t12*0x400 + t13*0x3D10
	r.n[3] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t4 + t13*0x400 + t14*0x3D10
	r.n[4] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t5 + t14*0x400 + t15*0x3D10
	r.n[5] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t6 + t15*0x400 + t16*0x3D10
	r.n[6] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t7 + t16*0x400 + t17*0x3D10
	r.n[7] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t8 + t17*0x400 + t18*0x3D10
	r.n[8] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t9 + t18*0x400 + t19*0x1000003D10
	r.n[9] = uint32(c) & 0x03FFFFF
	c = c >> 22
	d = t0 + c*0x3D1
	r.n[0] = uint32(d) & 0x3FFFFFF
	d = d >> 26
	d = d + t1 + c*0x40
	r.n[1] = uint32(d) & 0x3FFFFFF
	d = d >> 26
	r.n[2] = uint32(t2 + d)
}

func (a *Secp256k1Field) Sqr(r *Secp256k1Field) {
	var c, d uint64
	var t0, t1, t2, t3, t4, t5, t6 uint64
	var t7, t8, t9, t10, t11, t12, t13 uint64
	var t14, t15, t16, t17, t18, t19 uint64

	c = uint64(a.n[0]) * uint64(a.n[0])
	t0 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[1])
	t1 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[2]) +
		uint64(a.n[1])*uint64(a.n[1])
	t2 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[3]) +
		(uint64(a.n[1])*2)*uint64(a.n[2])
	t3 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[4]) +
		(uint64(a.n[1])*2)*uint64(a.n[3]) +
		uint64(a.n[2])*uint64(a.n[2])
	t4 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[5]) +
		(uint64(a.n[1])*2)*uint64(a.n[4]) +
		(uint64(a.n[2])*2)*uint64(a.n[3])
	t5 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[6]) +
		(uint64(a.n[1])*2)*uint64(a.n[5]) +
		(uint64(a.n[2])*2)*uint64(a.n[4]) +
		uint64(a.n[3])*uint64(a.n[3])
	t6 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[7]) +
		(uint64(a.n[1])*2)*uint64(a.n[6]) +
		(uint64(a.n[2])*2)*uint64(a.n[5]) +
		(uint64(a.n[3])*2)*uint64(a.n[4])
	t7 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[8]) +
		(uint64(a.n[1])*2)*uint64(a.n[7]) +
		(uint64(a.n[2])*2)*uint64(a.n[6]) +
		(uint64(a.n[3])*2)*uint64(a.n[5]) +
		uint64(a.n[4])*uint64(a.n[4])
	t8 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[0])*2)*uint64(a.n[9]) +
		(uint64(a.n[1])*2)*uint64(a.n[8]) +
		(uint64(a.n[2])*2)*uint64(a.n[7]) +
		(uint64(a.n[3])*2)*uint64(a.n[6]) +
		(uint64(a.n[4])*2)*uint64(a.n[5])
	t9 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[1])*2)*uint64(a.n[9]) +
		(uint64(a.n[2])*2)*uint64(a.n[8]) +
		(uint64(a.n[3])*2)*uint64(a.n[7]) +
		(uint64(a.n[4])*2)*uint64(a.n[6]) +
		uint64(a.n[5])*uint64(a.n[5])
	t10 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[2])*2)*uint64(a.n[9]) +
		(uint64(a.n[3])*2)*uint64(a.n[8]) +
		(uint64(a.n[4])*2)*uint64(a.n[7]) +
		(uint64(a.n[5])*2)*uint64(a.n[6])
	t11 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[3])*2)*uint64(a.n[9]) +
		(uint64(a.n[4])*2)*uint64(a.n[8]) +
		(uint64(a.n[5])*2)*uint64(a.n[7]) +
		uint64(a.n[6])*uint64(a.n[6])
	t12 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[4])*2)*uint64(a.n[9]) +
		(uint64(a.n[5])*2)*uint64(a.n[8]) +
		(uint64(a.n[6])*2)*uint64(a.n[7])
	t13 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[5])*2)*uint64(a.n[9]) +
		(uint64(a.n[6])*2)*uint64(a.n[8]) +
		uint64(a.n[7])*uint64(a.n[7])
	t14 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[6])*2)*uint64(a.n[9]) +
		(uint64(a.n[7])*2)*uint64(a.n[8])
	t15 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[7])*2)*uint64(a.n[9]) +
		uint64(a.n[8])*uint64(a.n[8])
	t16 = c & 0x3FFFFFF
	c = c >> 26
	c = c + (uint64(a.n[8])*2)*uint64(a.n[9])
	t17 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[9])*uint64(a.n[9])
	t18 = c & 0x3FFFFFF
	c = c >> 26
	t19 = c

	c = t0 + t10*0x3D10
	t0 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t1 + t10*0x400 + t11*0x3D10
	t1 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t2 + t11*0x400 + t12*0x3D10
	t2 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t3 + t12*0x400 + t13*0x3D10
	r.n[3] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t4 + t13*0x400 + t14*0x3D10
	r.n[4] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t5 + t14*0x400 + t15*0x3D10
	r.n[5] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t6 + t15*0x400 + t16*0x3D10
	r.n[6] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t7 + t16*0x400 + t17*0x3D10
	r.n[7] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t8 + t17*0x400 + t18*0x3D10
	r.n[8] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t9 + t18*0x400 + t19*0x1000003D10
	r.n[9] = uint32(c) & 0x03FFFFF
	c = c >> 22
	d = t0 + c*0x3D1
	r.n[0] = uint32(d) & 0x3FFFFFF
	d = d >> 26
	d = d + t1 + c*0x40
	r.n[1] = uint32(d) & 0x3FFFFFF
	d = d >> 26
	r.n[2] = uint32(t2 + d)
}

/**********************************************************************/
/****************************** -num- *********************************/
/**********************************************************************/
var (
	__Secp256k1_BigInt1 *big.Int = new(big.Int).SetInt64(1)
)

type Secp256k1Number struct {
	big.Int
}

func (a *Secp256k1Number) Print(label string) {
	fmt.Println(label, hex.EncodeToString(a.Bytes()))
}

func (r *Secp256k1Number) mod_mul(a, b, m *Secp256k1Number) {
	r.Mul(&a.Int, &b.Int)
	r.Mod(&r.Int, &m.Int)
	return
}

func (r *Secp256k1Number) mod_inv(a, b *Secp256k1Number) {
	r.ModInverse(&a.Int, &b.Int)
	return
}

func (r *Secp256k1Number) mod(a *Secp256k1Number) {
	r.Mod(&r.Int, &a.Int)
	return
}

func (a *Secp256k1Number) SetHex(s string) {
	a.SetString(s, 16)
}

//SetBytes and GetBytes are inherited by default
//added
//func (a *Number) SetBytes(b []byte) {
//	a.SetBytes(b)
//}

func (num *Secp256k1Number) mask_bits(bits uint) {
	mask := new(big.Int).Lsh(__Secp256k1_BigInt1, bits)
	mask.Sub(mask, __Secp256k1_BigInt1)
	num.Int.And(&num.Int, mask)
}

func (a *Secp256k1Number) split_exp(r1, r2 *Secp256k1Number) {
	var bnc1, bnc2, bnn2, bnt1, bnt2 Secp256k1Number

	bnn2.Int.Rsh(&__Secp256k1_TheCurve.Order.Int, 1)

	bnc1.Mul(&a.Int, &__Secp256k1_TheCurve.a1b2.Int)
	bnc1.Add(&bnc1.Int, &bnn2.Int)
	bnc1.Div(&bnc1.Int, &__Secp256k1_TheCurve.Order.Int)

	bnc2.Mul(&a.Int, &__Secp256k1_TheCurve.b1.Int)
	bnc2.Add(&bnc2.Int, &bnn2.Int)
	bnc2.Div(&bnc2.Int, &__Secp256k1_TheCurve.Order.Int)

	bnt1.Mul(&bnc1.Int, &__Secp256k1_TheCurve.a1b2.Int)
	bnt2.Mul(&bnc2.Int, &__Secp256k1_TheCurve.a2.Int)
	bnt1.Add(&bnt1.Int, &bnt2.Int)
	r1.Sub(&a.Int, &bnt1.Int)

	bnt1.Mul(&bnc1.Int, &__Secp256k1_TheCurve.b1.Int)
	bnt2.Mul(&bnc2.Int, &__Secp256k1_TheCurve.a1b2.Int)
	r2.Sub(&bnt1.Int, &bnt2.Int)
}

func (a *Secp256k1Number) split(rl, rh *Secp256k1Number, bits uint) {
	rl.Int.Set(&a.Int)
	rh.Int.Rsh(&rl.Int, bits)
	rl.mask_bits(bits)
}

func (num *Secp256k1Number) rsh(bits uint) {
	num.Rsh(&num.Int, bits)
}

func (num *Secp256k1Number) inc() {
	num.Add(&num.Int, __Secp256k1_BigInt1)
}

func (num *Secp256k1Number) rsh_x(bits uint) (res int) {
	res = int(new(big.Int).And(&num.Int, new(big.Int).SetUint64((1<<bits)-1)).Uint64())
	num.Rsh(&num.Int, bits)
	return
}

func (num *Secp256k1Number) IsOdd() bool {
	return num.Bit(0) != 0
}

func (num *Secp256k1Number) get_bin(le int) []byte {
	bts := num.Bytes()
	if len(bts) > le {
		return nil
	}
	if len(bts) == le {
		return bts
	}
	return append(make([]byte, le-len(bts)), bts...)
}

/**********************************************************************/
/****************************** secp256k1 *****************************/
/**********************************************************************/
const __Secp256k1_WINDOW_A = 5
const __Secp256k1_WINDOW_G = 14
const __Secp256k1_FORCE_LOW_S = true // At the output of the Sign() function

var __Secp256k1_TheCurve struct {
	Order, half_order    Secp256k1Number
	G                    Secp256k1XY
	beta                 Secp256k1Field
	lambda, a1b2, b1, a2 Secp256k1Number
	p                    Secp256k1Number
}

func __secp256k1_init_contants() {
	__Secp256k1_TheCurve.Order.SetBytes([]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE,
		0xBA, 0xAE, 0xDC, 0xE6, 0xAF, 0x48, 0xA0, 0x3B, 0xBF, 0xD2, 0x5E, 0x8C, 0xD0, 0x36, 0x41, 0x41})

	__Secp256k1_TheCurve.half_order.SetBytes([]byte{
		0X7F, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF, 0XFF,
		0X5D, 0X57, 0X6E, 0X73, 0X57, 0XA4, 0X50, 0X1D, 0XDF, 0XE9, 0X2F, 0X46, 0X68, 0X1B, 0X20, 0XA0})

	__Secp256k1_TheCurve.p.SetBytes([]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE, 0xFF, 0xFF, 0xFC, 0x2F})

	__Secp256k1_TheCurve.G.X.SetB32([]byte{
		0x79, 0xBE, 0x66, 0x7E, 0xF9, 0xDC, 0xBB, 0xAC, 0x55, 0xA0, 0x62, 0x95, 0xCE, 0x87, 0x0B, 0x07,
		0x02, 0x9B, 0xFC, 0xDB, 0x2D, 0xCE, 0x28, 0xD9, 0x59, 0xF2, 0x81, 0x5B, 0x16, 0xF8, 0x17, 0x98})

	__Secp256k1_TheCurve.G.Y.SetB32([]byte{
		0x48, 0x3A, 0xDA, 0x77, 0x26, 0xA3, 0xC4, 0x65, 0x5D, 0xA4, 0xFB, 0xFC, 0x0E, 0x11, 0x08, 0xA8,
		0xFD, 0x17, 0xB4, 0x48, 0xA6, 0x85, 0x54, 0x19, 0x9C, 0x47, 0xD0, 0x8F, 0xFB, 0x10, 0xD4, 0xB8})

	__Secp256k1_TheCurve.lambda.SetBytes([]byte{
		0x53, 0x63, 0xad, 0x4c, 0xc0, 0x5c, 0x30, 0xe0, 0xa5, 0x26, 0x1c, 0x02, 0x88, 0x12, 0x64, 0x5a,
		0x12, 0x2e, 0x22, 0xea, 0x20, 0x81, 0x66, 0x78, 0xdf, 0x02, 0x96, 0x7c, 0x1b, 0x23, 0xbd, 0x72})

	__Secp256k1_TheCurve.beta.SetB32([]byte{
		0x7a, 0xe9, 0x6a, 0x2b, 0x65, 0x7c, 0x07, 0x10, 0x6e, 0x64, 0x47, 0x9e, 0xac, 0x34, 0x34, 0xe9,
		0x9c, 0xf0, 0x49, 0x75, 0x12, 0xf5, 0x89, 0x95, 0xc1, 0x39, 0x6c, 0x28, 0x71, 0x95, 0x01, 0xee})

	__Secp256k1_TheCurve.a1b2.SetBytes([]byte{
		0x30, 0x86, 0xd2, 0x21, 0xa7, 0xd4, 0x6b, 0xcd, 0xe8, 0x6c, 0x90, 0xe4, 0x92, 0x84, 0xeb, 0x15})

	__Secp256k1_TheCurve.b1.SetBytes([]byte{
		0xe4, 0x43, 0x7e, 0xd6, 0x01, 0x0e, 0x88, 0x28, 0x6f, 0x54, 0x7f, 0xa9, 0x0a, 0xbf, 0xe4, 0xc3})

	__Secp256k1_TheCurve.a2.SetBytes([]byte{
		0x01, 0x14, 0xca, 0x50, 0xf7, 0xa8, 0xe2, 0xf3, 0xf6, 0x57, 0xc1, 0x10, 0x8d, 0x9d, 0x44, 0xcf, 0xd8})
}

func init() {
	__secp256k1_init_contants()
}

/**********************************************************************/
/****************************** -sig- *********************************/
/**********************************************************************/
type Secp256k1Signature struct {
	R, S Secp256k1Number
}

func (s *Secp256k1Signature) Print(lab string) {
	fmt.Println(lab+".R:", hex.EncodeToString(s.R.Bytes()))
	fmt.Println(lab+".S:", hex.EncodeToString(s.S.Bytes()))
}

func (r *Secp256k1Signature) Verify(pubkey *Secp256k1XY, message *Secp256k1Number) (ret bool) {
	var r2 Secp256k1Number
	ret = r.recompute(&r2, pubkey, message) && r.R.Cmp(&r2.Int) == 0
	return
}

func (sig *Secp256k1Signature) recompute(r2 *Secp256k1Number, pubkey *Secp256k1XY, message *Secp256k1Number) (ret bool) {
	var sn, u1, u2 Secp256k1Number

	sn.mod_inv(&sig.S, &__Secp256k1_TheCurve.Order)
	u1.mod_mul(&sn, message, &__Secp256k1_TheCurve.Order)
	u2.mod_mul(&sn, &sig.R, &__Secp256k1_TheCurve.Order)

	var pr, pubkeyj Secp256k1XYZ
	pubkeyj.SetXY(pubkey)

	pubkeyj.ECmult(&pr, &u2, &u1)
	if !pr.IsInfinity() {
		var xr Secp256k1Field
		pr.get_x(&xr)
		xr.Normalize()
		var xrb [32]byte
		xr.GetB32(xrb[:])
		r2.SetBytes(xrb[:])
		r2.Mod(&r2.Int, &__Secp256k1_TheCurve.Order.Int)
		ret = true
	}

	return
}

//TODO: return type, or nil on failure
func (sig *Secp256k1Signature) Recover(pubkey *Secp256k1XY, m *Secp256k1Number, recid int) (ret bool) {
	var rx, rn, u1, u2 Secp256k1Number
	var fx Secp256k1Field
	var X Secp256k1XY
	var xj, qj Secp256k1XYZ

	rx.Set(&sig.R.Int)
	if (recid & 2) != 0 {
		rx.Add(&rx.Int, &__Secp256k1_TheCurve.Order.Int)
		if rx.Cmp(&__Secp256k1_TheCurve.p.Int) >= 0 {
			return false
		}
	}

	fx.SetB32(rx.get_bin(32))

	X.SetXO(&fx, (recid&1) != 0)
	if !X.IsValid() {
		return false
	}

	xj.SetXY(&X)
	rn.mod_inv(&sig.R, &__Secp256k1_TheCurve.Order)
	u1.mod_mul(&rn, m, &__Secp256k1_TheCurve.Order)
	u1.Sub(&__Secp256k1_TheCurve.Order.Int, &u1.Int)
	u2.mod_mul(&rn, &sig.S, &__Secp256k1_TheCurve.Order)
	xj.ECmult(&qj, &u2, &u1)
	pubkey.SetXYZ(&qj)

	return true
}

func (sig *Secp256k1Signature) Sign(seckey, message, nonce *Secp256k1Number, recid *int) int {
	var r Secp256k1XY
	var rp Secp256k1XYZ
	var n Secp256k1Number
	var b [32]byte

	secp256k1XYZECmultGen(&rp, nonce)
	r.SetXYZ(&rp)
	r.X.Normalize()
	r.Y.Normalize()
	r.X.GetB32(b[:])
	sig.R.SetBytes(b[:])
	if recid != nil {
		*recid = 0
		if sig.R.Cmp(&__Secp256k1_TheCurve.Order.Int) >= 0 {
			*recid |= 2
		}
		if r.Y.IsOdd() {
			*recid |= 1
		}
	}
	sig.R.mod(&__Secp256k1_TheCurve.Order)
	n.mod_mul(&sig.R, seckey, &__Secp256k1_TheCurve.Order)
	n.Add(&n.Int, &message.Int)
	n.mod(&__Secp256k1_TheCurve.Order)
	sig.S.mod_inv(nonce, &__Secp256k1_TheCurve.Order)
	sig.S.mod_mul(&sig.S, &n, &__Secp256k1_TheCurve.Order)
	if sig.S.Sign() == 0 {
		return 0
	}
	if sig.S.IsOdd() {
		sig.S.Sub(&__Secp256k1_TheCurve.Order.Int, &sig.S.Int)
		if recid != nil {
			*recid ^= 1
		}
	}

	if __Secp256k1_FORCE_LOW_S && sig.S.Cmp(&__Secp256k1_TheCurve.half_order.Int) == 1 {
		sig.S.Sub(&__Secp256k1_TheCurve.Order.Int, &sig.S.Int)
		if recid != nil {
			*recid ^= 1
		}
	}

	return 1
}

/*
//uncompressed Signature Parsing in DER
func (r *Signature) ParseBytes(sig []byte) int {
	if sig[0] != 0x30 || len(sig) < 5 {
		return -1
	}

	lenr := int(sig[3])
	if lenr == 0 || 5+lenr >= len(sig) || sig[lenr+4] != 0x02 {
		return -1
	}

	lens := int(sig[lenr+5])
	if lens == 0 || int(sig[1]) != lenr+lens+4 || lenr+lens+6 > len(sig) || sig[2] != 0x02 {
		return -1
	}

	r.R.SetBytes(sig[4 : 4+lenr])
	r.S.SetBytes(sig[6+lenr : 6+lenr+lens])
	return 6 + lenr + lens
}
*/

/*
//uncompressed Signature parsing in DER
func (sig *Signature) Bytes() []byte {
	r := sig.R.Bytes()
	if r[0] >= 0x80 {
		r = append([]byte{0}, r...)
	}
	s := sig.S.Bytes()
	if s[0] >= 0x80 {
		s = append([]byte{0}, s...)
	}
	res := new(bytes.Buffer)
	res.WriteByte(0x30)
	res.WriteByte(byte(4 + len(r) + len(s)))
	res.WriteByte(0x02)
	res.WriteByte(byte(len(r)))
	res.Write(r)
	res.WriteByte(0x02)
	res.WriteByte(byte(len(s)))
	res.Write(s)
	return res.Bytes()
}
*/

//compressed signature parsing
func (r *Secp256k1Signature) ParseBytes(sig []byte) error {
	if len(sig) != 64 {
		return errors.New("ParseBytes: invalid sig")
	}
	r.R.SetBytes(sig[0:32])
	r.S.SetBytes(sig[32:64])
	return nil
}

//secp256k1_num_get_bin(sig64, 32, &sig.r);
//secp256k1_num_get_bin(sig64 + 32, 32, &sig.s);

//compressed signature parsing
func (sig *Secp256k1Signature) Bytes() []byte {
	r := sig.R.Bytes() //endianess
	s := sig.S.Bytes() //endianess

	for len(r) < 32 {
		r = append([]byte{0}, r...)
	}
	for len(s) < 32 {
		s = append([]byte{0}, s...)
	}

	if len(r) != 32 || len(s) != 32 {
		return nil
	}

	res := new(bytes.Buffer)
	res.Write(r)
	res.Write(s)

	//test
	if true {
		ret := res.Bytes()
		var sig2 Secp256k1Signature
		sig2.ParseBytes(ret)
		if bytes.Equal(sig.R.Bytes(), sig2.R.Bytes()) == false {
			return nil
		}
		if bytes.Equal(sig.S.Bytes(), sig2.S.Bytes()) == false {
			return nil
		}
	}

	if len(res.Bytes()) != 64 {
		return nil
	}
	return res.Bytes()
}

/**********************************************************************/
/****************************** -xy-  *********************************/
/**********************************************************************/
type Secp256k1XY struct {
	X, Y     Secp256k1Field
	Infinity bool
}

func (ge *Secp256k1XY) Print(lab string) {
	if ge.Infinity {
		fmt.Println(lab + " - Infinity")
		return
	}
	fmt.Println(lab+".X:", ge.X.String())
	fmt.Println(lab+".Y:", ge.Y.String())
}

//edited

/*
   if (size == 33 && (pub[0] == 0x02 || pub[0] == 0x03)) {
       secp256k1_fe_t x;
       secp256k1_fe_set_b32(&x, pub+1);
       return secp256k1_ge_set_xo(elem, &x, pub[0] == 0x03);
   } else if (size == 65 && (pub[0] == 0x04 || pub[0] == 0x06 || pub[0] == 0x07)) {
       secp256k1_fe_t x, y;
       secp256k1_fe_set_b32(&x, pub+1);
       secp256k1_fe_set_b32(&y, pub+33);
       secp256k1_ge_set_xy(elem, &x, &y);
       if ((pub[0] == 0x06 || pub[0] == 0x07) && secp256k1_fe_is_odd(&y) != (pub[0] == 0x07))
           return 0;
       return secp256k1_ge_is_valid(elem);
   }
*/
//All compact keys appear to be valid by construction, but may fail
//is valid check

//WARNING: for compact signatures, will succeed unconditionally
//however, elem.IsValid will fail
func (elem *Secp256k1XY) ParsePubkey(pub []byte) bool {
	if len(pub) != 33 {
		return false
	}
	if len(pub) == 33 && (pub[0] == 0x02 || pub[0] == 0x03) {
		elem.X.SetB32(pub[1:33])
		elem.SetXO(&elem.X, pub[0] == 0x03)
	} else {
		return false
	}
	//THIS FAILS
	//reenable later
	//if elem.IsValid() == false {
	//	return false
	//}

	/*
		 else if len(pub) == 65 && (pub[0] == 0x04 || pub[0] == 0x06 || pub[0] == 0x07) {
			elem.X.SetB32(pub[1:33])
			elem.Y.SetB32(pub[33:65])
			if (pub[0] == 0x06 || pub[0] == 0x07) && elem.Y.IsOdd() != (pub[0] == 0x07) {
				return false
			}
		}
	*/
	return true
}

// Returns serialized key in in compressed format: "<02> <X>",
// eventually "<03> <X>"
//33 bytes
func (pub Secp256k1XY) Bytes() []byte {
	pub.X.Normalize() // See GitHub issue #15

	var raw []byte = make([]byte, 33)
	if pub.Y.IsOdd() {
		raw[0] = 0x03
	} else {
		raw[0] = 0x02
	}
	pub.X.GetB32(raw[1:])
	return raw
}

// Returns serialized key in uncompressed format "<04> <X> <Y>"
//65 bytes
func (pub *Secp256k1XY) BytesUncompressed() (raw []byte) {
	pub.X.Normalize() // See GitHub issue #15
	pub.Y.Normalize() // See GitHub issue #15

	raw = make([]byte, 65)
	raw[0] = 0x04
	pub.X.GetB32(raw[1:33])
	pub.Y.GetB32(raw[33:65])
	return
}

func (r *Secp256k1XY) SetXY(X, Y *Secp256k1Field) {
	r.Infinity = false
	r.X = *X
	r.Y = *Y
}

/*
int static secp256k1_ecdsa_pubkey_parse(secp256k1_ge_t *elem, const unsigned char *pub, int size) {
    if (size == 33 && (pub[0] == 0x02 || pub[0] == 0x03)) {
        secp256k1_fe_t x;
        secp256k1_fe_set_b32(&x, pub+1);
        return secp256k1_ge_set_xo(elem, &x, pub[0] == 0x03);
    } else if (size == 65 && (pub[0] == 0x04 || pub[0] == 0x06 || pub[0] == 0x07)) {
        secp256k1_fe_t x, y;
        secp256k1_fe_set_b32(&x, pub+1);
        secp256k1_fe_set_b32(&y, pub+33);
        secp256k1_ge_set_xy(elem, &x, &y);
        if ((pub[0] == 0x06 || pub[0] == 0x07) && secp256k1_fe_is_odd(&y) != (pub[0] == 0x07))
            return 0;
        return secp256k1_ge_is_valid(elem);
    } else {
        return 0;
    }
}
*/

//    if (size == 33 && (pub[0] == 0x02 || pub[0] == 0x03)) {
//        secp256k1_fe_t x;
//        secp256k1_fe_set_b32(&x, pub+1);
//        return secp256k1_ge_set_xo(elem, &x, pub[0] == 0x03);

func (a *Secp256k1XY) IsValid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, c Secp256k1Field
	a.Y.Sqr(&y2)
	a.X.Sqr(&x3)
	x3.Mul(&x3, &a.X)
	c.SetInt(7)
	x3.SetAdd(&c)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}

func (r *Secp256k1XY) SetXYZ(a *Secp256k1XYZ) {
	var z2, z3 Secp256k1Field
	a.Z.InvVar(&a.Z)
	a.Z.Sqr(&z2)
	a.Z.Mul(&z3, &z2)
	a.X.Mul(&a.X, &z2)
	a.Y.Mul(&a.Y, &z3)
	a.Z.SetInt(1)
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
}

func (a *Secp256k1XY) precomp(w int) (pre []Secp256k1XY) {
	pre = make([]Secp256k1XY, (1 << (uint(w) - 2)))
	pre[0] = *a
	var X, d, tmp Secp256k1XYZ
	X.SetXY(a)
	X.Double(&d)
	for i := 1; i < len(pre); i++ {
		d.AddXY(&tmp, &pre[i-1])
		pre[i].SetXYZ(&tmp)
	}
	return
}

func (a *Secp256k1XY) Neg(r *Secp256k1XY) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}

/*
int static secp256k1_ge_set_xo(secp256k1_ge_t *r, const secp256k1_fe_t *x, int odd) {
    r->x = *x;
    secp256k1_fe_t x2; secp256k1_fe_sqr(&x2, x);
    secp256k1_fe_t x3; secp256k1_fe_mul(&x3, x, &x2);
    r->infinity = 0;
    secp256k1_fe_t c; secp256k1_fe_set_int(&c, 7);
    secp256k1_fe_add(&c, &x3);
    if (!secp256k1_fe_sqrt(&r->y, &c))
        return 0;
    secp256k1_fe_normalize(&r->y);
    if (secp256k1_fe_is_odd(&r->y) != odd)
        secp256k1_fe_negate(&r->y, &r->y, 1);
    return 1;
}
*/

func (r *Secp256k1XY) SetXO(X *Secp256k1Field, odd bool) {
	var c, x2, x3 Secp256k1Field
	r.X = *X
	X.Sqr(&x2)
	X.Mul(&x3, &x2)
	r.Infinity = false
	c.SetInt(7)
	c.SetAdd(&x3)
	c.Sqrt(&r.Y) //does not return, can fail
	if r.Y.IsOdd() != odd {
		r.Y.Negate(&r.Y, 1)
	}

	//r.X.Normalize() // See GitHub issue #15
	r.Y.Normalize()
}

func (pk *Secp256k1XY) AddXY(a *Secp256k1XY) {
	var xyz Secp256k1XYZ
	xyz.SetXY(pk)
	xyz.AddXY(&xyz, a)
	pk.SetXYZ(&xyz)
}

/*
func (pk *XY) GetPublicKey() []byte {
	var out []byte = make([]byte, 65, 65)
	pk.X.GetB32(out[1:33])
	if len(out) == 65 {
		out[0] = 0x04
		pk.Y.GetB32(out[33:65])
	} else {
		if pk.Y.IsOdd() {
			out[0] = 0x03
		} else {
			out[0] = 0x02
		}
	}
	return out
}
*/

//use compact format
//returns only 33 bytes
//same as bytes()
//TODO: deprecate, replace with .Bytes()
func (pk *Secp256k1XY) GetPublicKey() []byte {
	return pk.Bytes()
	/*
		var out []byte = make([]byte, 33, 33)
		pk.X.GetB32(out[1:33])
		if pk.Y.IsOdd() {
			out[0] = 0x03
		} else {
			out[0] = 0x02
		}
		return out
	*/
}

/**********************************************************************/
/****************************** -xyz- *********************************/
/**********************************************************************/
type Secp256k1XYZ struct {
	X, Y, Z  Secp256k1Field
	Infinity bool
}

func (gej Secp256k1XYZ) Print(lab string) {
	if gej.Infinity {
		fmt.Println(lab + " - INFINITY")
		return
	}
	fmt.Println(lab+".X", gej.X.String())
	fmt.Println(lab+".Y", gej.Y.String())
	fmt.Println(lab+".Z", gej.Z.String())
}

func (r *Secp256k1XYZ) SetXY(a *Secp256k1XY) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Z.SetInt(1)
}

func (r *Secp256k1XYZ) IsInfinity() bool {
	return r.Infinity
}

func (a *Secp256k1XYZ) IsValid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, z2, z6 Secp256k1Field
	a.Y.Sqr(&y2)
	a.X.Sqr(&x3)
	x3.Mul(&x3, &a.X)
	a.Z.Sqr(&z2)
	z2.Sqr(&z6)
	z6.Mul(&z6, &z2)
	z6.MulInt(7)
	x3.SetAdd(&z6)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}

func (a *Secp256k1XYZ) get_x(r *Secp256k1Field) {
	var zi2 Secp256k1Field
	a.Z.InvVar(&zi2)
	zi2.Sqr(&zi2)
	a.X.Mul(r, &zi2)
}

func (a *Secp256k1XYZ) Normalize() {
	a.X.Normalize()
	a.Y.Normalize()
	a.Z.Normalize()
}

func (a *Secp256k1XYZ) Equals(b *Secp256k1XYZ) bool {
	if a.Infinity != b.Infinity {
		return false
	}
	// TODO: is the normalize really needed here?
	a.Normalize()
	b.Normalize()
	return a.X.Equals(&b.X) && a.Y.Equals(&b.Y) && a.Z.Equals(&b.Z)
}

func (a *Secp256k1XYZ) precomp(w int) (pre []Secp256k1XYZ) {
	var d Secp256k1XYZ
	pre = make([]Secp256k1XYZ, (1 << (uint(w) - 2)))
	pre[0] = *a
	pre[0].Double(&d)
	for i := 1; i < len(pre); i++ {
		d.Add(&pre[i], &pre[i-1])
	}
	return
}

func (self *Secp256k1XYZ) ecmult_wnaf(wnaf []int, a *Secp256k1Number, w uint) (ret int) {
	var zeroes uint
	var X Secp256k1Number
	X.Set(&a.Int)

	for X.Sign() != 0 {
		for X.Bit(0) == 0 {
			zeroes++
			X.rsh(1)
		}
		word := X.rsh_x(w)
		for zeroes > 0 {
			wnaf[ret] = 0
			ret++
			zeroes--
		}
		if (word & (1 << (w - 1))) != 0 {
			X.inc()
			wnaf[ret] = (word - (1 << w))
		} else {
			wnaf[ret] = word
		}
		zeroes = w - 1
		ret++
	}
	return
}

// r = na*a + ng*G
func (a *Secp256k1XYZ) ECmult(r *Secp256k1XYZ, na, ng *Secp256k1Number) {
	var na_1, na_lam, ng_1, ng_128 Secp256k1Number

	// split na into na_1 and na_lam (where na = na_1 + na_lam*lambda, and na_1 and na_lam are ~128 bit)
	na.split_exp(&na_1, &na_lam)

	// split ng into ng_1 and ng_128 (where gn = gn_1 + gn_128*2^128, and gn_1 and gn_128 are ~128 bit)
	ng.split(&ng_1, &ng_128, 128)

	// build wnaf representation for na_1, na_lam, ng_1, ng_128
	var wnaf_na_1, wnaf_na_lam, wnaf_ng_1, wnaf_ng_128 [129]int
	bits_na_1 := a.ecmult_wnaf(wnaf_na_1[:], &na_1, __Secp256k1_WINDOW_A)
	bits_na_lam := a.ecmult_wnaf(wnaf_na_lam[:], &na_lam, __Secp256k1_WINDOW_A)
	bits_ng_1 := a.ecmult_wnaf(wnaf_ng_1[:], &ng_1, __Secp256k1_WINDOW_G)
	bits_ng_128 := a.ecmult_wnaf(wnaf_ng_128[:], &ng_128, __Secp256k1_WINDOW_G)

	// calculate a_lam = a*lambda
	var a_lam Secp256k1XYZ
	a.mul_lambda(&a_lam)

	// calculate odd multiples of a and a_lam
	pre_a_1 := a.precomp(__Secp256k1_WINDOW_A)
	pre_a_lam := a_lam.precomp(__Secp256k1_WINDOW_A)

	bits := bits_na_1
	if bits_na_lam > bits {
		bits = bits_na_lam
	}
	if bits_ng_1 > bits {
		bits = bits_ng_1
	}
	if bits_ng_128 > bits {
		bits = bits_ng_128
	}

	r.Infinity = true

	var tmpj Secp256k1XYZ
	var tmpa Secp256k1XY
	var n int

	for i := bits - 1; i >= 0; i-- {
		r.Double(r)

		if i < bits_na_1 {
			n = wnaf_na_1[i]
			if n > 0 {
				r.Add(r, &pre_a_1[((n)-1)/2])
			} else if n != 0 {
				pre_a_1[(-(n)-1)/2].Neg(&tmpj)
				r.Add(r, &tmpj)
			}
		}

		if i < bits_na_lam {
			n = wnaf_na_lam[i]
			if n > 0 {
				r.Add(r, &pre_a_lam[((n)-1)/2])
			} else if n != 0 {
				pre_a_lam[(-(n)-1)/2].Neg(&tmpj)
				r.Add(r, &tmpj)
			}
		}

		if i < bits_ng_1 {
			n = wnaf_ng_1[i]
			if n > 0 {
				r.AddXY(r, &secp256k1_pre_g[((n)-1)/2])
			} else if n != 0 {
				secp256k1_pre_g[(-(n)-1)/2].Neg(&tmpa)
				r.AddXY(r, &tmpa)
			}
		}

		if i < bits_ng_128 {
			n = wnaf_ng_128[i]
			if n > 0 {
				r.AddXY(r, &secp256k1_pre_g_128[((n)-1)/2])
			} else if n != 0 {
				secp256k1_pre_g_128[(-(n)-1)/2].Neg(&tmpa)
				r.AddXY(r, &tmpa)
			}
		}
	}
}

func (a *Secp256k1XYZ) Neg(r *Secp256k1XYZ) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Z = a.Z
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}

func (a *Secp256k1XYZ) mul_lambda(r *Secp256k1XYZ) {
	*r = *a
	r.X.Mul(&r.X, &__Secp256k1_TheCurve.beta)
}

func (a *Secp256k1XYZ) Double(r *Secp256k1XYZ) {
	var t1, t2, t3, t4, t5 Secp256k1Field

	t5 = a.Y
	t5.Normalize()
	if a.Infinity || t5.IsZero() {
		r.Infinity = true
		return
	}

	t5.Mul(&r.Z, &a.Z)
	r.Z.MulInt(2)
	a.X.Sqr(&t1)
	t1.MulInt(3)
	t1.Sqr(&t2)
	t5.Sqr(&t3)
	t3.MulInt(2)
	t3.Sqr(&t4)
	t4.MulInt(2)
	a.X.Mul(&t3, &t3)
	r.X = t3
	r.X.MulInt(4)
	r.X.Negate(&r.X, 4)
	r.X.SetAdd(&t2)
	t2.Negate(&t2, 1)
	t3.MulInt(6)
	t3.SetAdd(&t2)
	t1.Mul(&r.Y, &t3)
	t4.Negate(&t2, 2)
	r.Y.SetAdd(&t2)
	r.Infinity = false
}

func (a *Secp256k1XYZ) AddXY(r *Secp256k1XYZ, b *Secp256k1XY) {
	if a.Infinity {
		r.Infinity = b.Infinity
		r.X = b.X
		r.Y = b.Y
		r.Z.SetInt(1)
		return
	}
	if b.Infinity {
		*r = *a
		return
	}
	r.Infinity = false
	var z12, u1, u2, s1, s2 Secp256k1Field
	a.Z.Sqr(&z12)
	u1 = a.X
	u1.Normalize()
	b.X.Mul(&u2, &z12)
	s1 = a.Y
	s1.Normalize()
	b.Y.Mul(&s2, &z12)
	s2.Mul(&s2, &a.Z)
	u1.Normalize()
	u2.Normalize()

	if u1.Equals(&u2) {
		s1.Normalize()
		s2.Normalize()
		if s1.Equals(&s2) {
			a.Double(r)
		} else {
			r.Infinity = true
		}
		return
	}

	var h, i, i2, h2, h3, t Secp256k1Field
	u1.Negate(&h, 1)
	h.SetAdd(&u2)
	s1.Negate(&i, 1)
	i.SetAdd(&s2)
	i.Sqr(&i2)
	h.Sqr(&h2)
	h.Mul(&h3, &h2)
	r.Z = a.Z
	r.Z.Mul(&r.Z, &h)
	u1.Mul(&t, &h2)
	r.X = t
	r.X.MulInt(2)
	r.X.SetAdd(&h3)
	r.X.Negate(&r.X, 3)
	r.X.SetAdd(&i2)
	r.X.Negate(&r.Y, 5)
	r.Y.SetAdd(&t)
	r.Y.Mul(&r.Y, &i)
	h3.Mul(&h3, &s1)
	h3.Negate(&h3, 1)
	r.Y.SetAdd(&h3)
}

func (a *Secp256k1XYZ) Add(r, b *Secp256k1XYZ) {
	if a.Infinity {
		*r = *b
		return
	}
	if b.Infinity {
		*r = *a
		return
	}
	r.Infinity = false
	var z22, z12, u1, u2, s1, s2 Secp256k1Field

	b.Z.Sqr(&z22)
	a.Z.Sqr(&z12)
	a.X.Mul(&u1, &z22)
	b.X.Mul(&u2, &z12)
	a.Y.Mul(&s1, &z22)
	s1.Mul(&s1, &b.Z)
	b.Y.Mul(&s2, &z12)
	s2.Mul(&s2, &a.Z)
	u1.Normalize()
	u2.Normalize()
	if u1.Equals(&u2) {
		s1.Normalize()
		s2.Normalize()
		if s1.Equals(&s2) {
			a.Double(r)
		} else {
			r.Infinity = true
		}
		return
	}
	var h, i, i2, h2, h3, t Secp256k1Field

	u1.Negate(&h, 1)
	h.SetAdd(&u2)
	s1.Negate(&i, 1)
	i.SetAdd(&s2)
	i.Sqr(&i2)
	h.Sqr(&h2)
	h.Mul(&h3, &h2)
	a.Z.Mul(&r.Z, &b.Z)
	r.Z.Mul(&r.Z, &h)
	u1.Mul(&t, &h2)
	r.X = t
	r.X.MulInt(2)
	r.X.SetAdd(&h3)
	r.X.Negate(&r.X, 3)
	r.X.SetAdd(&i2)
	r.X.Negate(&r.Y, 5)
	r.Y.SetAdd(&t)
	r.Y.Mul(&r.Y, &i)
	h3.Mul(&h3, &s1)
	h3.Negate(&h3, 1)
	r.Y.SetAdd(&h3)
}

// r = a*G
//TODO: Change to returning result
//TODO: input should not be pointer
func secp256k1XYZECmultGen(r *Secp256k1XYZ, a *Secp256k1Number) {
	var n Secp256k1Number
	n.Set(&a.Int)
	r.SetXY(&secp256k1_prec[0][n.rsh_x(4)])
	for j := 1; j < 64; j++ {
		r.AddXY(r, &secp256k1_prec[j][n.rsh_x(4)])
	}
	r.AddXY(r, &secp256k1_fin)
}
