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

package utils

import (
	"errors"
	"sync"

	"github.com/portto/tangerine-consensus/common"
	"github.com/portto/tangerine-consensus/core/crypto"
	"github.com/portto/tangerine-consensus/core/types"
)

var (
	// ErrNodeSetNotReady means we got nil empty node set.
	ErrNodeSetNotReady = errors.New("node set is not ready")
	// ErrCRSNotReady means we got empty CRS.
	ErrCRSNotReady = errors.New("crs is not ready")
	// ErrConfigurationNotReady means we go nil configuration.
	ErrConfigurationNotReady = errors.New("configuration is not ready")
)

type sets struct {
	crs       common.Hash
	nodeSet   *types.NodeSet
	notarySet map[types.NodeID]struct{}
}

// NodeSetCacheInterface interface specifies interface used by NodeSetCache.
type NodeSetCacheInterface interface {
	// Configuration returns the configuration at a given round.
	// Return the genesis configuration if round == 0.
	Configuration(round uint64) *types.Config

	// CRS returns the CRS for a given round.
	// Return the genesis CRS if round == 0.
	CRS(round uint64) common.Hash

	// NodeSet returns the node set at a given round.
	// Return the genesis node set if round == 0.
	NodeSet(round uint64) []crypto.PublicKey
}

// NodeSetCache caches node set information.
//
// NOTE: this module doesn't handle DKG resetting and can only be used along
//       with utils.RoundEvent.
type NodeSetCache struct {
	lock    sync.RWMutex
	nsIntf  NodeSetCacheInterface
	rounds  map[uint64]*sets
	keyPool map[types.NodeID]*struct {
		pubKey crypto.PublicKey
		refCnt int
	}
}

// NewNodeSetCache constructs an NodeSetCache instance.
func NewNodeSetCache(nsIntf NodeSetCacheInterface) *NodeSetCache {
	return &NodeSetCache{
		nsIntf: nsIntf,
		rounds: make(map[uint64]*sets),
		keyPool: make(map[types.NodeID]*struct {
			pubKey crypto.PublicKey
			refCnt int
		}),
	}
}

// Exists checks if a node is in node set of that round.
func (cache *NodeSetCache) Exists(
	round uint64, nodeID types.NodeID) (exists bool, err error) {

	nIDs, exists := cache.get(round)
	if !exists {
		if nIDs, err = cache.update(round); err != nil {
			return
		}
	}
	_, exists = nIDs.nodeSet.IDs[nodeID]
	return
}

// GetPublicKey return public key for that node:
func (cache *NodeSetCache) GetPublicKey(
	nodeID types.NodeID) (key crypto.PublicKey, exists bool) {

	cache.lock.RLock()
	defer cache.lock.RUnlock()

	rec, exists := cache.keyPool[nodeID]
	if exists {
		key = rec.pubKey
	}
	return
}

// GetNodeSet returns IDs of nodes set of this round as map.
func (cache *NodeSetCache) GetNodeSet(round uint64) (*types.NodeSet, error) {
	IDs, exists := cache.get(round)
	if !exists {
		var err error
		if IDs, err = cache.update(round); err != nil {
			return nil, err
		}
	}
	return IDs.nodeSet.Clone(), nil
}

// GetNotarySet returns of notary set of this round.
func (cache *NodeSetCache) GetNotarySet(
	round uint64) (map[types.NodeID]struct{}, error) {
	IDs, err := cache.getOrUpdate(round)
	if err != nil {
		return nil, err
	}
	return cache.cloneMap(IDs.notarySet), nil
}

// Purge a specific round.
func (cache *NodeSetCache) Purge(rID uint64) {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	nIDs, exist := cache.rounds[rID]
	if !exist {
		return
	}
	for nID := range nIDs.nodeSet.IDs {
		rec := cache.keyPool[nID]
		if rec.refCnt--; rec.refCnt == 0 {
			delete(cache.keyPool, nID)
		}
	}
	delete(cache.rounds, rID)
}

// Touch updates the internal cache of round.
func (cache *NodeSetCache) Touch(round uint64) (err error) {
	_, err = cache.update(round)
	return
}

func (cache *NodeSetCache) cloneMap(
	nIDs map[types.NodeID]struct{}) map[types.NodeID]struct{} {
	nIDsCopy := make(map[types.NodeID]struct{}, len(nIDs))
	for k := range nIDs {
		nIDsCopy[k] = struct{}{}
	}
	return nIDsCopy
}

func (cache *NodeSetCache) getOrUpdate(round uint64) (nIDs *sets, err error) {
	s, exists := cache.get(round)
	if !exists {
		if s, err = cache.update(round); err != nil {
			return
		}
	}
	nIDs = s
	return
}

// update node set for that round.
//
// This cache would maintain 10 rounds before the updated round and purge
// rounds not in this range.
func (cache *NodeSetCache) update(round uint64) (nIDs *sets, err error) {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	// Get information for the requested round.
	keySet := cache.nsIntf.NodeSet(round)
	if keySet == nil {
		err = ErrNodeSetNotReady
		return
	}
	crs := cache.nsIntf.CRS(round)
	if (crs == common.Hash{}) {
		err = ErrCRSNotReady
		return
	}
	// Cache new round.
	nodeSet := types.NewNodeSet()
	for _, key := range keySet {
		nID := types.NewNodeID(key)
		nodeSet.Add(nID)
		if rec, exists := cache.keyPool[nID]; exists {
			rec.refCnt++
		} else {
			cache.keyPool[nID] = &struct {
				pubKey crypto.PublicKey
				refCnt int
			}{key, 1}
		}
	}
	cfg := cache.nsIntf.Configuration(round)
	if cfg == nil {
		err = ErrConfigurationNotReady
		return
	}
	nIDs = &sets{
		crs:       crs,
		nodeSet:   nodeSet,
		notarySet: make(map[types.NodeID]struct{}),
	}
	nIDs.notarySet = nodeSet.GetSubSet(
		int(cfg.NotarySetSize), types.NewNotarySetTarget(crs))
	cache.rounds[round] = nIDs
	// Purge older rounds.
	for rID, nIDs := range cache.rounds {
		nodeSet := nIDs.nodeSet
		if round-rID <= 5 {
			continue
		}
		for nID := range nodeSet.IDs {
			rec := cache.keyPool[nID]
			if rec.refCnt--; rec.refCnt == 0 {
				delete(cache.keyPool, nID)
			}
		}
		delete(cache.rounds, rID)
	}
	return
}

func (cache *NodeSetCache) get(round uint64) (nIDs *sets, exists bool) {
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	nIDs, exists = cache.rounds[round]
	return
}
