/*
 * Copyright 2019, Offchain Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package checkpoint

import (
	"bytes"
	"github.com/offchainlabs/arb-avm/loader"
	"github.com/offchainlabs/arb-avm/protocol"
	"github.com/offchainlabs/arb-avm/value"
	"testing"
)

func TestOpen(t *testing.T) {
	cp, err := NewCheckpointer(nil, true)
	if err != nil {
		t.Error(err)
	}
	err = cp.Close()
	if err != nil {
		t.Error(err)
	}
}

func TestValues(t *testing.T) {
	cp, err := NewCheckpointer(nil, true)
	if err != nil {
		t.Error(err)
	}

	val38 := value.NewInt64Value(38)
	err = cp.AddRefToValue(val38)
	hash38 := val38.Hash()
	res38, err2 := cp.RestoreValueFromHash(hash38)

	if err2 != nil {
		t.Error(err2)
	}
	if !value.Eq(val38, res38) {
		t.Errorf("Save/restore int(38) failed")
	}

	tup1, err := value.NewTupleFromSlice([]value.Value{val38, val38, value.NewEmptyTuple()})
	if err != nil {
		t.Error(err)
	}
	tup2, err := value.NewTupleFromSlice([]value.Value{tup1, val38, val38, tup1, val38, tup1})
	if err != nil {
		t.Error(err)
	}

	hash2 := tup2.Hash()
	err = cp.AddRefToValue(tup2)

	res2, err2 := cp.RestoreValueFromHash(hash2)
	if err2 != nil {
		t.Error(err2)
	}
	if !value.Eq(tup2, res2) {
		t.Errorf("Save/restore of tuple failed")
	}

	if err := cp.Close(); err != nil {
		t.Error(err)
	}
}

const dotAOfile = "fibonacci.ao"

func TestMachines(t *testing.T) {
	machine, err := loader.LoadMachineFromFile(dotAOfile, false)
	if err != nil {
		t.Error(err)
	}
	_ = machine.Run(10)

	cp, err := NewCheckpointer(machine, true)
	if err != nil {
		t.Error(err)
	}

	if err := cp.SaveMachine([]byte("test"), machine); err != nil {
		t.Error(err)
	}

	restMach, err := cp.RestoreMachine([]byte("test"))
	if err != nil {
		t.Error(err)
	}

	if restMach.Hash() != machine.Hash() {
		t.Errorf("restored machine hash doesn't match original")
	}

	_ = machine.Run(10)
	if err := cp.SaveMachine([]byte("test"), machine); err != nil {
		t.Error(err)
	}

	restMach, err = cp.RestoreMachine([]byte("test"))
	if err != nil {
		t.Error(err)
	}

	if restMach.Hash() != machine.Hash() {
		t.Errorf("restored machine hash doesn't match original")
	}

	if err := cp.Close(); err != nil {
		t.Error(err)
	}
}

func TestMachinesAcrossRestart(t *testing.T) {
	machine, err := loader.LoadMachineFromFile(dotAOfile, false)
	if err != nil {
		t.Error(err)
	}

	cp, err := NewCheckpointer(machine, true)
	if err != nil {
		t.Error(err)
	}

	if err := cp.SaveMachine([]byte("test"), machine); err != nil {
		t.Error(err)
	}
	if err := cp.Close(); err != nil {
		t.Error(err)
	}

	cp, err = NewCheckpointer(nil, false) // restart, keeping old checkpoint file
	if err != nil {
		t.Error(err)
	}

	restMach, err := cp.RestoreMachine([]byte("test"))
	if err != nil {
		t.Error(err)
	}

	if restMach.Hash() != machine.Hash() {
		t.Errorf("restored machine hash doesn't match original")
	}

	if err := cp.Close(); err != nil {
		t.Error(err)
	}
}

func TestVersionedCp(t *testing.T) {
	machine, err := loader.LoadMachineFromFile(dotAOfile, false)
	if err != nil {
		t.Error(err)
	}
	cp, err := NewCheckpointer(machine, true)
	if err != nil {
		t.Error(err)
	}
	vcp, err := NewVersionedCheckpointer(cp)
	if err != nil {
		t.Error(err)
	}

	minV, maxV := vcp.KnownVersions()
	if minV != 0 {
		t.Errorf("unexpected minVersionNum")
	}
	if maxV != -1 {
		t.Errorf("unexpected maxVersionNum")
	}

	_ = machine.Run(10)
	vnum, err := vcp.SaveVersion(machine, nil)
	if err != nil {
		t.Error(err)
	}
	if vnum != 0 {
		t.Errorf("unexpected version number return")
	}
	_ = machine.Run(10)
	vnum, err = vcp.SaveVersion(machine, []byte("some state"))
	if err != nil {
		t.Error(err)
	}
	if vnum != 1 {
		t.Errorf("unexpected version number return")
	}
	minV, maxV = vcp.KnownVersions()
	if minV != 0 {
		t.Errorf("unexpected minVersionNum")
	}
	if maxV != 1 {
		t.Errorf("unexpected maxVersionNum")
	}
}

func TestEventChainCp(t *testing.T) {
	machine, err := loader.LoadMachineFromFile(dotAOfile, false)
	if err != nil {
		t.Error(err)
	}
	inbox := value.NewEmptyTuple()
	cp, err := NewCheckpointer(machine, true)
	if err != nil {
		t.Error(err)
	}
	key := []byte("This is a string name key")
	timeBounds := [2]uint64{0, 17}
	balanceTracker := protocol.NewBalanceTracker()
	ecc, err := NewEventChainCheckpointer(cp, key, machine, timeBounds, balanceTracker)
	if err != nil {
		t.Error(err)
	}

	sigs := []byte{3, 1, 4, 1, 5, 9, 2, 6}
	for i := uint64(0); i < 6; i++ {
		_ = machine.Run(10)
		err = ecc.RecordIntentToSign(i, machine, inbox)
		if err != nil {
			t.Error(err)
		}
		err = ecc.RecordSignatures(i, sigs)
		if err != nil {
			t.Error(err)
		}
	}

	err = ecc.Discard()
	if err != nil {
		t.Error(err)
	}
}

func TestEventChainRestore(t *testing.T) {
	machine, err := loader.LoadMachineFromFile(dotAOfile, false)
	if err != nil {
		t.Error(err)
	}
	inbox := value.NewEmptyTuple()
	cp, err := NewCheckpointer(machine, true)
	if err != nil {
		t.Error(err)
	}
	keySuffix := []byte("This is a string name key")
	timeBounds := [2]uint64{0, 17}
	balanceTracker := protocol.NewBalanceTracker()
	ecc, err := NewEventChainCheckpointer(cp, keySuffix, machine, timeBounds, balanceTracker)
	if err != nil {
		t.Error(err)
	}

	sigs := []byte{3, 1, 4, 1, 5, 9, 2, 6}
	maxSeqNum := uint64(6)
	machineHashes := make([][32]byte, 0)
	inboxHashes := make([][32]byte, 0)
	for i := uint64(0); i < maxSeqNum; i++ {
		_ = machine.Run(10)
		machineHashes = append(machineHashes, machine.Hash())
		inboxHashes = append(inboxHashes, inbox.Hash())
		err = ecc.RecordIntentToSign(i, machine, inbox)
		if err != nil {
			t.Error(err)
		}
		err = ecc.RecordSignatures(i, sigs)
		if err != nil {
			t.Error(err)
		}
	}

	if err := cp.Close(); err != nil {
		t.Error(err)
	}

	cp, err = NewCheckpointer(nil, false) // restart, keeping old checkpoint file
	if err != nil {
		t.Error(err)
	}
	ecc, err = RestoreEventChainCheckpointer(cp, keySuffix)
	if err != nil {
		t.Error(err)
	}

	seqNumToRestore := uint64(4)
	restoredMachine, restoredInbox, restoredSigs, err := ecc.RestoreFromSeqNum(seqNumToRestore)
	if err != nil {
		t.Error(err)
	}
	restMachHash := restoredMachine.Hash()
	if !bytes.Equal(restMachHash[:], machineHashes[seqNumToRestore][:]) {
		t.Errorf("EvChain restored machine hash mismatch")
	}
	restInboxHash := restoredInbox.Hash()
	if !bytes.Equal(restInboxHash[:], inboxHashes[seqNumToRestore][:]) {
		t.Errorf("EvChain restored inbox mismatch")
	}
	if !bytes.Equal(restoredSigs, sigs) {
		t.Errorf("EvChain restored signatures mismatch")
	}

	err = ecc.Discard()
	if err != nil {
		t.Error(err)
	}
}
