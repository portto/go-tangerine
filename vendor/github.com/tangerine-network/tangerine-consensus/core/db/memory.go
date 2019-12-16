// Copyright 2018 The dexon-consensus Authors
// This file is part of the dexon-consensus library.
//
// The dexon-consensus library is free software: you can redistribute it
// and/or modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.
//
// The dexon-consensus library is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser
// General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the dexon-consensus library. If not, see
// <http://www.gnu.org/licenses/>.

package db

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"github.com/portto/tangerine-consensus/common"
	"github.com/portto/tangerine-consensus/core/crypto/dkg"
	"github.com/portto/tangerine-consensus/core/types"
)

type blockSeqIterator struct {
	idx int
	db  *MemBackedDB
}

// NextBlock implemenets BlockIterator.NextBlock method.
func (seq *blockSeqIterator) NextBlock() (types.Block, error) {
	curIdx := seq.idx
	seq.idx++
	return seq.db.getBlockByIndex(curIdx)
}

// MemBackedDB is a memory backed DB implementation.
type MemBackedDB struct {
	blocksLock               sync.RWMutex
	blockHashSequence        common.Hashes
	blocksByHash             map[common.Hash]*types.Block
	compactionChainTipLock   sync.RWMutex
	compactionChainTipHash   common.Hash
	compactionChainTipHeight uint64
	dkgPrivateKeysLock       sync.RWMutex
	dkgPrivateKeys           map[uint64]*dkgPrivateKey
	dkgProtocolLock          sync.RWMutex
	dkgProtocolInfo          *DKGProtocolInfo
	persistantFilePath       string
}

// NewMemBackedDB initialize a memory-backed database.
func NewMemBackedDB(persistantFilePath ...string) (
	dbInst *MemBackedDB, err error) {
	dbInst = &MemBackedDB{
		blockHashSequence: common.Hashes{},
		blocksByHash:      make(map[common.Hash]*types.Block),
		dkgPrivateKeys:    make(map[uint64]*dkgPrivateKey),
	}
	if len(persistantFilePath) == 0 || len(persistantFilePath[0]) == 0 {
		return
	}
	dbInst.persistantFilePath = persistantFilePath[0]
	buf, err := ioutil.ReadFile(dbInst.persistantFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			// Something unexpected happened.
			return
		}
		// It's expected behavior that file doesn't exists, we should not
		// report error on it.
		err = nil
		return
	}

	// Init this instance by file content, it's a temporary way
	// to export those private field for JSON encoding.
	toLoad := struct {
		Sequence common.Hashes
		ByHash   map[common.Hash]*types.Block
	}{}
	err = json.Unmarshal(buf, &toLoad)
	if err != nil {
		return
	}
	dbInst.blockHashSequence = toLoad.Sequence
	dbInst.blocksByHash = toLoad.ByHash
	return
}

// HasBlock returns wheter or not the DB has a block identified with the hash.
func (m *MemBackedDB) HasBlock(hash common.Hash) bool {
	m.blocksLock.RLock()
	defer m.blocksLock.RUnlock()

	_, ok := m.blocksByHash[hash]
	return ok
}

// GetBlock returns a block given a hash.
func (m *MemBackedDB) GetBlock(hash common.Hash) (types.Block, error) {
	m.blocksLock.RLock()
	defer m.blocksLock.RUnlock()

	return m.internalGetBlock(hash)
}

func (m *MemBackedDB) internalGetBlock(hash common.Hash) (types.Block, error) {
	b, ok := m.blocksByHash[hash]
	if !ok {
		return types.Block{}, ErrBlockDoesNotExist
	}
	return *b, nil
}

// PutBlock inserts a new block into the database.
func (m *MemBackedDB) PutBlock(block types.Block) error {
	if m.HasBlock(block.Hash) {
		return ErrBlockExists
	}

	m.blocksLock.Lock()
	defer m.blocksLock.Unlock()

	m.blockHashSequence = append(m.blockHashSequence, block.Hash)
	m.blocksByHash[block.Hash] = &block
	return nil
}

// UpdateBlock updates a block in the database.
func (m *MemBackedDB) UpdateBlock(block types.Block) error {
	if !m.HasBlock(block.Hash) {
		return ErrBlockDoesNotExist
	}

	m.blocksLock.Lock()
	defer m.blocksLock.Unlock()

	m.blocksByHash[block.Hash] = &block
	return nil
}

// PutCompactionChainTipInfo saves tip of compaction chain into the database.
func (m *MemBackedDB) PutCompactionChainTipInfo(
	blockHash common.Hash, height uint64) error {
	m.compactionChainTipLock.Lock()
	defer m.compactionChainTipLock.Unlock()
	if m.compactionChainTipHeight+1 != height {
		return ErrInvalidCompactionChainTipHeight
	}
	m.compactionChainTipHeight = height
	m.compactionChainTipHash = blockHash
	return nil
}

// GetCompactionChainTipInfo get the tip info of compaction chain into the
// database.
func (m *MemBackedDB) GetCompactionChainTipInfo() (
	hash common.Hash, height uint64) {
	m.compactionChainTipLock.RLock()
	defer m.compactionChainTipLock.RUnlock()
	return m.compactionChainTipHash, m.compactionChainTipHeight
}

// GetDKGPrivateKey get DKG private key of one round.
func (m *MemBackedDB) GetDKGPrivateKey(round, reset uint64) (
	dkg.PrivateKey, error) {
	m.dkgPrivateKeysLock.RLock()
	defer m.dkgPrivateKeysLock.RUnlock()
	if prv, exists := m.dkgPrivateKeys[round]; exists && prv.Reset == reset {
		return prv.PK, nil
	}
	return dkg.PrivateKey{}, ErrDKGPrivateKeyDoesNotExist
}

// PutDKGPrivateKey save DKG private key of one round.
func (m *MemBackedDB) PutDKGPrivateKey(
	round, reset uint64, prv dkg.PrivateKey) error {
	m.dkgPrivateKeysLock.Lock()
	defer m.dkgPrivateKeysLock.Unlock()
	if prv, exists := m.dkgPrivateKeys[round]; exists && prv.Reset == reset {
		return ErrDKGPrivateKeyExists
	}
	m.dkgPrivateKeys[round] = &dkgPrivateKey{
		PK:    prv,
		Reset: reset,
	}
	return nil
}

// GetDKGProtocol get DKG protocol.
func (m *MemBackedDB) GetDKGProtocol() (
	DKGProtocolInfo, error) {
	m.dkgProtocolLock.RLock()
	defer m.dkgProtocolLock.RUnlock()
	if m.dkgProtocolInfo == nil {
		return DKGProtocolInfo{}, ErrDKGProtocolDoesNotExist
	}

	return *m.dkgProtocolInfo, nil
}

// PutOrUpdateDKGProtocol save DKG protocol.
func (m *MemBackedDB) PutOrUpdateDKGProtocol(dkgProtocol DKGProtocolInfo) error {
	m.dkgProtocolLock.Lock()
	defer m.dkgProtocolLock.Unlock()
	m.dkgProtocolInfo = &dkgProtocol
	return nil
}

// Close implement Closer interface, which would release allocated resource.
func (m *MemBackedDB) Close() (err error) {
	// Save internal state to a pretty-print json file. It's a temporary way
	// to dump private file via JSON encoding.
	if len(m.persistantFilePath) == 0 {
		return
	}

	m.blocksLock.RLock()
	defer m.blocksLock.RUnlock()

	toDump := struct {
		Sequence common.Hashes
		ByHash   map[common.Hash]*types.Block
	}{
		Sequence: m.blockHashSequence,
		ByHash:   m.blocksByHash,
	}

	// Dump to JSON with 2-space indent.
	buf, err := json.Marshal(&toDump)
	if err != nil {
		return
	}

	err = ioutil.WriteFile(m.persistantFilePath, buf, 0644)
	return
}

func (m *MemBackedDB) getBlockByIndex(idx int) (types.Block, error) {
	m.blocksLock.RLock()
	defer m.blocksLock.RUnlock()

	if idx >= len(m.blockHashSequence) {
		return types.Block{}, ErrIterationFinished
	}

	hash := m.blockHashSequence[idx]
	return m.internalGetBlock(hash)
}

// GetAllBlocks implement Reader.GetAllBlocks method, which allows caller
// to retrieve all blocks in DB.
func (m *MemBackedDB) GetAllBlocks() (BlockIterator, error) {
	return &blockSeqIterator{db: m}, nil
}
