/*
 * LF: Global Fully Replicated Key/Value Store
 * Copyright (C) 2018-2019  ZeroTier, Inc.  https://www.zerotier.com/
 *
 * Licensed under the terms of the MIT license (see LICENSE.txt).
 */

package lf

import (
	"bytes"
	"crypto/aes"
	"crypto/ecdsa"
	"crypto/elliptic"
	secrand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/sha3"
)

//////////////////////////////////////////////////////////////////////////////

// TestCore tests various core functions and helpers.
func TestCore(out io.Writer) bool {
	// This checks to make sure the Sum method of hashes fills arrays as expected.
	// This is sort of an ambiguous behavior in the API docs, so we want to detect
	// if the actual behavior changes. If it does we'll have to change a few spots.
	testStr := []byte("My hovercraft is full of eels.")
	fmt.Fprintf(out, "Testing hash slice filling behavior (API behavior check)... ")
	ref := sha256.Sum256(testStr)
	th := sha256.New()
	_, err := th.Write(testStr)
	if err != nil {
		panic(err)
	}
	var thout [32]byte
	th.Sum(thout[:0])
	ref2 := sha3.Sum512(testStr)
	th2 := sha3.New512()
	_, err = th2.Write(testStr)
	if err != nil {
		panic(err)
	}
	var thout2 [64]byte
	th2.Sum(thout2[:0])
	if bytes.Equal(thout[:], ref[:]) && bytes.Equal(thout2[:], ref2[:]) {
		fmt.Fprintf(out, "OK\n")
	} else {
		fmt.Fprintf(out, "FAILED\n")
		return false
	}

	fmt.Fprintf(out, "Testing Blob serialize/deserialize... ")
	var bbbuf [1024]byte
	for i := 1; i < 1024; i++ {
		bb := bbbuf[0:i]
		secrand.Read(bb)
		bj, err := json.Marshal(Blob(bb))
		if err != nil {
			fmt.Fprintf(out, "FAILED (marshal %d)\n", i)
			return false
		}
		var bb2 Blob
		err = json.Unmarshal(bj, &bb2)
		if err != nil {
			fmt.Fprintf(out, "FAILED (unmarshal %d, %s)\n", i, err.Error())
			return false
		}
		if !bytes.Equal(bb, bb2) {
			fmt.Fprintf(out, "FAILED (unmarshal %d, values not equal)\n", i)
			return false
		}
	}
	var bFromStr Blob
	err = bFromStr.UnmarshalJSON([]byte("\"Supercalifragilisticexpealidocious!\""))
	if err != nil || string(bFromStr) != "Supercalifragilisticexpealidocious!" {
		fmt.Fprintf(out, "FAILED (unmarshal from string)\n")
		return false
	}
	fmt.Fprintf(out, "OK\n")

	fmt.Fprintf(out, "Testing Shandwich256... ")
	t0 := Shandwich256(testStr)
	t1h := NewShandwich256()
	t1h.Write(testStr)
	t1 := t1h.Sum(nil)
	if bytes.Equal(t0[:], t1) && hex.EncodeToString(t0[:]) == "fcb43f704eb65e06be713636021d4168e9b355f9a8df24e14177f7ddc1105fea" {
		fmt.Fprintf(out, "OK\n")
	} else {
		fmt.Fprintf(out, "FAILED %x\n", t0)
		return false
	}

	fmt.Fprintf(out, "Testing deterministic owner generation from seed... ")
	op384, _ := NewOwnerFromSeed(OwnerTypeNistP384, []byte("lol"))
	if hex.EncodeToString(op384.Bytes()) != "0af36dd928ebaceb810601e5410f6cdde98ee88dc94d84dc8817e9e19e66119447641d3defcc555194f596078d329897a1" {
		fmt.Fprintf(out, "FAILED %x\n", op384.Bytes())
		return false
	}
	o25519, _ := NewOwnerFromSeed(OwnerTypeEd25519, []byte("lol"))
	if hex.EncodeToString(o25519.Bytes()) != "95fe40b0b3a3e06e3d79d7e4630ed78be5d38d30b98b7e27cd469b2304d82012" {
		fmt.Fprintf(out, "FAILED %x\n", o25519.Bytes())
		return false
	}
	fmt.Fprintf(out, "OK\n")

	curves := []elliptic.Curve{elliptic.P384(), ECCCurveBrainpoolP160T1}
	for ci := range curves {
		curve := curves[ci]

		fmt.Fprintf(out, "Testing %s ECDSA...\n", curve.Params().Name)
		priv, err := ecdsa.GenerateKey(curve, secrand.Reader)
		if err != nil {
			fmt.Fprintf(out, "  FAILED (generate): %s\n", err.Error())
			return false
		}
		pub, err := ECDSACompressPublicKey(&priv.PublicKey)
		if err != nil {
			fmt.Fprintf(out, "  FAILED (compress): %s\n", err.Error())
			return false
		}
		fmt.Fprintf(out, "  Public Key: [%d] %x\n", len(pub), pub)
		pub2, err := ECDSADecompressPublicKey(curve, pub)
		if err != nil {
			fmt.Fprintf(out, "  FAILED (decompress): %s\n", err.Error())
			return false
		}
		if pub2.X.Cmp(priv.PublicKey.X) != 0 || pub2.Y.Cmp(priv.PublicKey.Y) != 0 {
			fmt.Fprintf(out, "  FAILED (decompress): results are not the same!\n")
			return false
		}

		var junk [32]byte
		secrand.Read(junk[:])
		sig, err := ECDSASign(priv, junk[:])
		if err != nil {
			fmt.Fprintf(out, "  FAILED (sign): %s\n", err.Error())
			return false
		}
		fmt.Fprintf(out, "  Signature: [%d] %x\n", len(sig), sig)
		if !ECDSAVerify(&priv.PublicKey, junk[:], sig) {
			fmt.Fprintf(out, "  FAILED (verify): verify failed for correct message\n")
			return false
		}
		junk[1]++
		if ECDSAVerify(&priv.PublicKey, junk[:], sig) {
			fmt.Fprintf(out, "  FAILED (verify): verify succeeded for incorrect message\n")
			return false
		}
		junk[1]--
		sig[2]++
		if ECDSAVerify(&priv.PublicKey, junk[:], sig) {
			fmt.Fprintf(out, "  FAILED (verify): verify succeeded for incorrect signature (but correct message)\n")
			return false
		}

		for i := 0; i < 32; i++ {
			secrand.Read(junk[:])
			sig, _ := ECDSASignEmbedRecoveryIndex(priv, junk[:])
			if i == 0 {
				fmt.Fprintf(out, "  Key Recoverable Signature: [%d] %x\n  Testing key recovery... ", len(sig), sig)
			}
			pub := ECDSARecover(curve, junk[:], sig)
			if pub == nil {
				fmt.Fprintf(out, "FAILED (ECDSARecover returned nil)\n")
			}
			if pub.X.Cmp(priv.PublicKey.X) != 0 || pub.Y.Cmp(priv.PublicKey.Y) != 0 {
				pcomp, _ := ECDSACompressPublicKey(pub)
				fmt.Fprintf(out, "FAILED (ECDSARecover returned wrong key: %x)\n", pcomp)
			}
		}
		fmt.Fprintf(out, "OK\n")
	}

	fmt.Fprintf(out, "Testing Selector... ")
	var testSelectors [256]Selector
	var testSelectorClaimHash [32]byte
	secrand.Read(testSelectorClaimHash[:])
	for k := range testSelectors {
		testSelectors[k].set([]byte("name"), []byte(fmt.Sprintf("%.16x", k)), testSelectorClaimHash[:])
		ts2, err := newSelectorFromBytes(testSelectors[k].bytes())
		if err != nil || !bytes.Equal(ts2.Ordinal, testSelectors[k].Ordinal) || !bytes.Equal(ts2.Claim[:], testSelectors[k].Claim[:]) {
			fmt.Fprintln(out, "FAILED (marshal/unmarshal)")
			return false
		}
	}
	for k := 1; k < len(testSelectors); k++ {
		sk := testSelectors[k].key(testSelectorClaimHash[:])
		if bytes.Compare(testSelectors[k-1].key(testSelectorClaimHash[:]), sk) >= 0 {
			fmt.Fprintf(out, "FAILED (compare %d not < %d)\n", k-1, k)
			return false
		}
	}
	var selTest Selector
	selTest.set([]byte("name"), []byte("ord"), testSelectorClaimHash[:])
	if !bytes.Equal(MakeSelectorKey([]byte("name"), []byte("ord")), selTest.key(testSelectorClaimHash[:])) {
		fmt.Fprintf(out, "FAILED (keys from key() vs MakeSelectorKey() are not equal)\n")
		return false
	}
	fmt.Fprintf(out, "OK\n")

	fmt.Fprintf(out, "Testing Record marshal/unmarshal... ")
	for k := 0; k < 32; k++ {
		var testLinks [][32]byte
		for i := 0; i < 3; i++ {
			var tmp [32]byte
			secrand.Read(tmp[:])
			testLinks = append(testLinks, tmp)
		}
		var testValue [32]byte
		secrand.Read(testValue[:])
		owner, err := NewOwner(OwnerTypeEd25519)
		if err != nil {
			fmt.Fprintf(out, "FAILED (create owner): %s\n", err.Error())
			return false
		}
		rec, err := NewRecord(testValue[:], testLinks, []byte("test"), [][]byte{[]byte("test0")}, [][]byte{[]byte("0000")}, nil, uint64(k), nil, 0, owner)
		if err != nil {
			fmt.Fprintf(out, "FAILED (create record): %s\n", err.Error())
			return false
		}
		var testBuf0 bytes.Buffer
		err = rec.MarshalTo(&testBuf0)
		if err != nil {
			fmt.Fprintf(out, "FAILED (marshal record): %s\n", err.Error())
			return false
		}
		var rec2 Record
		err = rec2.UnmarshalFrom(&testBuf0)
		if err != nil {
			fmt.Fprintf(out, "FAILED (unmarshal record): %s\n", err.Error())
			return false
		}
		h0, h1 := rec.Hash(), rec2.Hash()
		if !bytes.Equal(h0[:], h1[:]) {
			fmt.Fprintf(out, "FAILED (hashes are not equal)\n")
			return false
		}
	}
	fmt.Fprintf(out, "OK\n")

	fmt.Fprintf(out, "Testing Record will full proof of work (generate, verify)... ")
	var testLinks [][32]byte
	for i := 0; i < 3; i++ {
		var tmp [32]byte
		secrand.Read(tmp[:])
		testLinks = append(testLinks, tmp)
	}
	var testValue [32]byte
	secrand.Read(testValue[:])
	owner, err := NewOwner(OwnerTypeEd25519)
	if err != nil {
		fmt.Fprintf(out, "FAILED (create owner): %s\n", err.Error())
		return false
	}
	wg := NewWharrgarblr(RecordDefaultWharrgarblMemory, 0)
	rec, err := NewRecord(testValue[:], testLinks, []byte("test"), [][]byte{[]byte("full record test")}, [][]byte{[]byte("0000")}, nil, TimeSec(), wg, 0, owner)
	if err != nil {
		fmt.Fprintf(out, "FAILED (new record creation): %s\n", err.Error())
		return false
	}
	err = rec.Validate()
	if err != nil {
		fmt.Fprintf(out, "FAILED (validate): %s\n", err.Error())
		return false
	}
	fmt.Fprintf(out, "OK\n")

	return true
}

//////////////////////////////////////////////////////////////////////////////

// TestWharrgarbl tests and runs benchmarks on the Wharrgarbl proof of work.
func TestWharrgarbl(out io.Writer) bool {
	var startTime, iterations, runTime uint64
	testWharrgarblSamples := 16
	var junk [32]byte
	var wout [WharrgarblOutputSize]byte

	// Have to do this here to generate the table
	wg := NewWharrgarblr(RecordDefaultWharrgarblMemory, 0)

	// Test Wharrgarbl's internal collision hash to make sure it's generating consistent results across platforms
	junk = sha256.Sum256([]byte("asdfasdf"))
	tc0, _ := aes.NewCipher(junk[:])
	tc1, _ := aes.NewCipher(junk[:])
	var testIn [16]byte
	for i := 0; i < 16; i++ {
		testIn[i] = byte(i)
	}
	th := wharrgarblHash(tc0, tc1, make([]byte, 16), &testIn)
	fmt.Fprintf(out, "Testing Wharrgarbl keyed 64-bit hash function... %.16x ", th)
	if th == 0xd4d965ceec0098a1 {
		fmt.Fprintf(out, "OK\n")
	} else {
		fmt.Fprintf(out, "FAILED\n")
		return false
	}

	fmt.Fprintf(out, "Wharrgarbl cost and score:\n")
	for s := uint(1); s <= RecordMaxSize; s *= 2 {
		fmt.Fprintf(out, "  %5d: cost: %.8x score: %.8x\n", s, recordWharrgarblCost(s), recordWharrgarblScore(recordWharrgarblCost(s)))
	}

	fmt.Fprintf(out, "Testing and benchmarking Wharrgarbl proof of work algorithm...\n")
	for rs := uint(256); rs <= 4096; rs += 256 {
		diff := recordWharrgarblCost(rs)
		iterations = 0
		startTime = TimeMs()
		for k := 0; k < testWharrgarblSamples; k++ {
			var ii uint64
			wout, ii = wg.Compute(junk[:], diff)
			iterations += ii
		}
		runTime = (TimeMs() - startTime) / uint64(testWharrgarblSamples)
		iterations /= uint64(testWharrgarblSamples)
		if WharrgarblVerify(wout[:], junk[:]) == 0 {
			fmt.Fprintf(out, "  %.8x: FAILED (verify)\n", diff)
			return false
		}
		fmt.Fprintf(out, "  %.8x: %d milliseconds %d iterations (difficulty for %d bytes)\n", diff, runTime, iterations, rs)
	}

	return true
}

//////////////////////////////////////////////////////////////////////////////

const testDatabaseInstances = 3
const testDatabaseRecords = 32768
const testDatabaseOwners = 16

// TestDatabase tests the database using a large set of randomly generated records.
func TestDatabase(testBasePath string, out io.Writer) bool {
	var err error
	var dbs [testDatabaseInstances]db

	testBasePath = path.Join(testBasePath, strconv.FormatInt(int64(os.Getpid()), 10))

	fmt.Fprintf(out, "Creating and opening %d databases in \"%s\"... ", testDatabaseInstances, testBasePath)
	for i := range dbs {
		p := path.Join(testBasePath, strconv.FormatInt(int64(i), 10))
		os.MkdirAll(p, 0755)
		err = dbs[i].open(p, [logLevelCount]*log.Logger{nil, nil, nil, nil, nil}, func(doff uint64, dlen uint, hash *[32]byte) {})
		if err != nil {
			fmt.Fprintf(out, "FAILED: %s\n", err.Error())
			return false
		}
	}
	fmt.Fprintf(out, "OK\n")

	defer func() {
		for i := range dbs {
			dbs[i].close()
		}
	}()

	fmt.Fprintf(out, "Generating %d owner public/private key pairs... ", testDatabaseOwners)
	var owners [testDatabaseOwners]*Owner
	for i := range owners {
		owners[i], err = NewOwner(OwnerTypeEd25519)
		if err != nil {
			fmt.Fprintf(out, "FAILED: %s\n", err.Error())
			return false
		}
	}
	fmt.Fprintf(out, "OK\n")

	fmt.Fprintf(out, "Generating %d random linked records... ", testDatabaseRecords)
	var values, selectors, ordinals, selectorKeys [testDatabaseRecords][]byte
	var records [testDatabaseRecords]*Record
	ts := TimeSec()
	testMaskingKey := []byte("maskingkey")
	for ri := 0; ri < testDatabaseRecords; ri++ {
		var linkTo []uint
		for i := 0; i < 3 && i < ri; i++ {
			lt := uint(rand.Int31()) % uint(ri)
			for j := 0; j < (ri * 2); j++ {
				if sliceContainsUInt(linkTo, lt) {
					lt = (lt + 1) % uint(ri)
				} else {
					linkTo = append(linkTo, lt)
					break
				}
			}
		}
		var links [][32]byte
		for i := range linkTo {
			links = append(links, *(records[linkTo[i]].Hash()))
		}

		ownerIdx := ri % testDatabaseOwners
		ts++
		values[ri] = []byte(strconv.FormatUint(ts, 10))
		selectors[ri] = []byte(fmt.Sprintf("%.16x", ownerIdx))
		ordinals[ri] = []byte(fmt.Sprintf("%.16x", ri))
		records[ri], err = NewRecord(
			values[ri],
			links,
			testMaskingKey,
			[][]byte{selectors[ri]},
			[][]byte{ordinals[ri]},
			nil,
			ts,
			nil,
			0,
			owners[ownerIdx])
		if err != nil {
			fmt.Fprintf(out, "FAILED: %s\n", err.Error())
			return false
		}

		valueDec, _ := records[ri].GetValue(testMaskingKey)
		if !bytes.Equal(values[ri], valueDec) {
			fmt.Fprintf(out, "FAILED: record value unmask failed!\n")
			return false
		}
		valueDec = nil
		valueDec, _ = records[ri].GetValue([]byte("not maskingkey"))
		if bytes.Equal(values[ri], valueDec) {
			fmt.Fprintf(out, "FAILED: record value unmask succeeded with wrong key!\n")
			return false
		}

		selectorKeys[ri] = records[ri].SelectorKey(0)
	}
	fmt.Fprintf(out, "OK\n")

	fmt.Fprintf(out, "Inserting records into all three databases...\n")
	for dbi := 0; dbi < testDatabaseInstances; dbi++ {
		for ri := 0; ri < testDatabaseRecords; ri++ {
			a := uint(rand.Int31()) % uint(testDatabaseRecords)
			b := uint(rand.Int31()) % uint(testDatabaseRecords)
			if a != b {
				records[a], records[b] = records[b], records[a]
			}
		}
		for ri := 0; ri < testDatabaseRecords; ri++ {
			err = dbs[dbi].putRecord(records[ri])
			if err != nil {
				fmt.Fprintf(out, "  #%d FAILED: %s\n", dbi, err.Error())
				return false
			}
		}
		fmt.Fprintf(out, "  #%d OK\n", dbi)
	}

	fmt.Fprintf(out, "Waiting for graph traversal and weight reconciliation... ")
	for dbi := 0; dbi < testDatabaseInstances; dbi++ {
		for dbs[dbi].hasPending() {
			time.Sleep(time.Second / 2)
		}
	}
	fmt.Fprintf(out, "OK\n")

	fmt.Fprintf(out, "Checking database CRC64s...\n")
	var c64s [testDatabaseInstances]uint64
	for dbi := 0; dbi < testDatabaseInstances; dbi++ {
		c64s[dbi] = dbs[dbi].crc64()
		if dbi == 0 || c64s[dbi-1] == c64s[dbi] {
			fmt.Fprintf(out, "  OK %.16x\n", c64s[dbi])
		} else {
			fmt.Fprintf(out, "  FAILED %.16x != %.16x\n", c64s[dbi], c64s[dbi-1])
			return false
		}
	}
	fmt.Fprintf(out, "All databases reached the same final state for hashes, weights, and links.\n")

	fmt.Fprintf(out, "Testing database queries by selector and selector range...\n")
	var gotRecordCount uint32
	wg := new(sync.WaitGroup)
	wg.Add(testDatabaseInstances)
	for dbi2 := 0; dbi2 < testDatabaseInstances; dbi2++ {
		dbi := dbi2
		go func() {
			defer wg.Done()
			rb := make([]byte, 0, 4096)
			for ri := 0; ri < testDatabaseRecords; ri++ {
				err = dbs[dbi].query(0, 9223372036854775807, [][2][]byte{{selectorKeys[ri], selectorKeys[ri]}}, func(ts, weightL, weightH, doff, dlen uint64, id *[32]byte, owner []byte) bool {
					rdata, err := dbs[dbi].getDataByOffset(doff, uint(dlen), rb[:0])
					if err != nil {
						fmt.Fprintf(out, "  FAILED to retrieve (selector key: %x) (%s)\n", selectorKeys[ri], err.Error())
						return false
					}
					rec, err := NewRecordFromBytes(rdata)
					if err != nil {
						fmt.Fprintf(out, "  FAILED to unmarshal (selector key: %x) (%s)\n", selectorKeys[ri], err.Error())
						return false
					}
					valueDec, err := rec.GetValue(testMaskingKey)
					if err != nil {
						fmt.Fprintf(out, "  FAILED to unmask value (selector key: %x) (%s)\n", selectorKeys[ri], err.Error())
						return false
					}
					if !bytes.Equal(valueDec, values[ri]) {
						fmt.Fprintf(out, "  FAILED to unmask value (selector key: %x) (values do not match)", selectorKeys[ri])
						return false
					}
					rc := atomic.AddUint32(&gotRecordCount, 1)
					if (rc % 1000) == 0 {
						fmt.Fprintf(out, "  ... %d records\n", rc)
					}
					return true
				})
			}
		}()
	}
	wg.Wait()
	if gotRecordCount != (testDatabaseRecords * testDatabaseInstances) {
		fmt.Fprintf(out, "  FAILED non-range query test: got %d records, expected %d\n", gotRecordCount, testDatabaseRecords*testDatabaseInstances)
	}
	fmt.Fprintf(out, "  Non-range query test OK (%d records from %d parallel databases)\n", gotRecordCount, testDatabaseInstances)
	gotRecordCount = 0
	wg = new(sync.WaitGroup)
	wg.Add(testDatabaseInstances)
	for dbi2 := 0; dbi2 < testDatabaseInstances; dbi2++ {
		dbi := dbi2
		go func() {
			defer wg.Done()
			rb := make([]byte, 0, 4096)
			for oi := 0; oi < testDatabaseOwners; oi++ {
				sk0 := MakeSelectorKey([]byte(fmt.Sprintf("%.16x", oi)), []byte("0000000000000000"))
				sk1 := MakeSelectorKey([]byte(fmt.Sprintf("%.16x", oi)), []byte("ffffffffffffffff"))
				err = dbs[dbi].query(0, 9223372036854775807, [][2][]byte{{sk0, sk1}}, func(ts, weightL, weightH, doff, dlen uint64, id *[32]byte, owner []byte) bool {
					_, err := dbs[dbi].getDataByOffset(doff, uint(dlen), rb[:0])
					if err != nil {
						fmt.Fprintf(out, "  FAILED to retrieve (selector key range %x-%x) (%s)\n", sk0, sk1, err.Error())
						return false
					}
					rc := atomic.AddUint32(&gotRecordCount, 1)
					if (rc % 1000) == 0 {
						fmt.Fprintf(out, "  ... %d records\n", rc)
					}
					return true
				})
			}
		}()
	}
	wg.Wait()
	if gotRecordCount != (testDatabaseRecords * testDatabaseInstances) {
		fmt.Fprintf(out, "  FAILED ordinal range query test: got %d records, expected %d\n", gotRecordCount, testDatabaseRecords*testDatabaseInstances)
	}
	fmt.Fprintf(out, "  Ordinal range query test OK (%d records from %d parallel databases)\n", gotRecordCount, testDatabaseInstances)

	return true
}
