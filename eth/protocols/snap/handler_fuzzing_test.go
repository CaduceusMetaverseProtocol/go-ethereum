// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package snap

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	fuzz "github.com/google/gofuzz"
)

func FuzzARange(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(data, &GetAccountRangePacket{}, GetAccountRangeMsg)
	})
}

func FuzzSRange(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(data, &GetStorageRangesPacket{}, GetStorageRangesMsg)
	})
}

func FuzzByteCodes(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(data, &GetByteCodesPacket{}, GetByteCodesMsg)
	})
}

func FuzzTrieNodes(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(data, &GetTrieNodesPacket{}, GetTrieNodesMsg)
	})
}

func doFuzz(input []byte, obj interface{}, code int) {
	bc := getChain()
	defer bc.Stop()
	fuzz.NewFromGoFuzz(input).Fuzz(obj)
	var data []byte
	switch p := obj.(type) {
	case *GetTrieNodesPacket:
		p.Root = trieRoot
		data, _ = rlp.EncodeToBytes(obj)
	default:
		data, _ = rlp.EncodeToBytes(obj)
	}
	cli := &dummyRW{
		code: uint64(code),
		data: data,
	}
	peer := NewFakePeer(65, "gazonk01", cli)
	err := HandleMessage(&dummyBackend{bc}, peer)
	switch {
	case err == nil && cli.writeCount != 1:
		panic(fmt.Sprintf("Expected 1 response, got %d", cli.writeCount))
	case err != nil && cli.writeCount != 0:
		panic(fmt.Sprintf("Expected 0 response, got %d", cli.writeCount))
	}
}

var trieRoot common.Hash

func getChain() *core.BlockChain {
	ga := make(types.GenesisAlloc, 1000)
	var a = make([]byte, 20)
	var mkStorage = func(k, v int) (common.Hash, common.Hash) {
		var kB = make([]byte, 32)
		var vB = make([]byte, 32)
		binary.LittleEndian.PutUint64(kB, uint64(k))
		binary.LittleEndian.PutUint64(vB, uint64(v))
		return common.BytesToHash(kB), common.BytesToHash(vB)
	}
	storage := make(map[common.Hash]common.Hash)
	for i := 0; i < 10; i++ {
		k, v := mkStorage(i, i)
		storage[k] = v
	}
	for i := 0; i < 1000; i++ {
		binary.LittleEndian.PutUint64(a, uint64(i+0xff))
		acc := types.Account{Balance: big.NewInt(int64(i))}
		if i%2 == 1 {
			acc.Storage = storage
		}
		ga[common.BytesToAddress(a)] = acc
	}
	gspec := &core.Genesis{
		Config: params.TestChainConfig,
		Alloc:  ga,
	}
	_, blocks, _ := core.GenerateChainWithGenesis(gspec, ethash.NewFaker(), 2, func(i int, gen *core.BlockGen) {})
	options := &core.BlockChainConfig{
		TrieCleanLimit: 0,
		TrieDirtyLimit: 0,
		TrieTimeLimit:  5 * time.Minute,
		NoPrefetch:     true,
		SnapshotLimit:  100,
		SnapshotWait:   true,
	}
	trieRoot = blocks[len(blocks)-1].Root()
	bc, _ := core.NewBlockChain(rawdb.NewMemoryDatabase(), gspec, ethash.NewFaker(), options)
	if _, err := bc.InsertChain(blocks); err != nil {
		panic(err)
	}
	return bc
}

type dummyBackend struct {
	chain *core.BlockChain
}

func (d *dummyBackend) Chain() *core.BlockChain       { return d.chain }
func (d *dummyBackend) RunPeer(*Peer, Handler) error  { return nil }
func (d *dummyBackend) PeerInfo(enode.ID) interface{} { return "Foo" }
func (d *dummyBackend) Handle(*Peer, Packet) error    { return nil }

type dummyRW struct {
	code       uint64
	data       []byte
	writeCount int
}

func (d *dummyRW) ReadMsg() (p2p.Msg, error) {
	return p2p.Msg{
		Code:       d.code,
		Payload:    bytes.NewReader(d.data),
		ReceivedAt: time.Now(),
		Size:       uint32(len(d.data)),
	}, nil
}

func (d *dummyRW) WriteMsg(msg p2p.Msg) error {
	d.writeCount++
	return nil
}
