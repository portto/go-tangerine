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

package core

import (
	"fmt"
	"sync"

	"github.com/portto/tangerine-consensus/common"
	"github.com/portto/tangerine-consensus/core/types"
)

type blockConfirmedEvent struct {
	block *types.Block
}

type blockDeliveredEvent struct {
	blockHash     common.Hash
	blockPosition types.Position
	rand          []byte
}

// nonBlocking implements these interfaces and is a decorator for
// them that makes the methods to be non-blocking.
//  - Application
//  - Debug
//  - It also provides nonblockig for db update.
type nonBlocking struct {
	app          Application
	debug        Debug
	eventChan    chan interface{}
	events       []interface{}
	eventsChange *sync.Cond
	running      sync.WaitGroup
}

func newNonBlocking(app Application, debug Debug) *nonBlocking {
	nonBlockingModule := &nonBlocking{
		app:          app,
		debug:        debug,
		eventChan:    make(chan interface{}, 6),
		events:       make([]interface{}, 0, 100),
		eventsChange: sync.NewCond(&sync.Mutex{}),
	}
	go nonBlockingModule.run()
	return nonBlockingModule
}

func (nb *nonBlocking) addEvent(event interface{}) {
	nb.eventsChange.L.Lock()
	defer nb.eventsChange.L.Unlock()
	nb.events = append(nb.events, event)
	nb.eventsChange.Broadcast()
}

func (nb *nonBlocking) run() {
	// This go routine consume the first event from events and call the
	// corresponding methods of Application/Debug/db.
	for {
		var event interface{}
		func() {
			nb.eventsChange.L.Lock()
			defer nb.eventsChange.L.Unlock()
			for len(nb.events) == 0 {
				nb.eventsChange.Wait()
			}
			event = nb.events[0]
			nb.events = nb.events[1:]
			nb.running.Add(1)
		}()
		switch e := event.(type) {
		case blockConfirmedEvent:
			nb.app.BlockConfirmed(*e.block)
		case blockDeliveredEvent:
			nb.app.BlockDelivered(e.blockHash, e.blockPosition, e.rand)
		default:
			fmt.Printf("Unknown event %v.", e)
		}
		nb.running.Done()
		nb.eventsChange.Broadcast()
	}
}

// wait will wait for all event in events finishes.
func (nb *nonBlocking) wait() {
	nb.eventsChange.L.Lock()
	defer nb.eventsChange.L.Unlock()
	for len(nb.events) > 0 {
		nb.eventsChange.Wait()
	}
	nb.running.Wait()
}

// PreparePayload cannot be non-blocking.
func (nb *nonBlocking) PreparePayload(position types.Position) ([]byte, error) {
	return nb.app.PreparePayload(position)
}

// PrepareWitness cannot be non-blocking.
func (nb *nonBlocking) PrepareWitness(height uint64) (types.Witness, error) {
	return nb.app.PrepareWitness(height)
}

// VerifyBlock cannot be non-blocking.
func (nb *nonBlocking) VerifyBlock(block *types.Block) types.BlockVerifyStatus {
	return nb.app.VerifyBlock(block)
}

// BlockConfirmed is called when a block is confirmed and added to lattice.
func (nb *nonBlocking) BlockConfirmed(block types.Block) {
	nb.addEvent(blockConfirmedEvent{&block})
}

// BlockDelivered is called when a block is add to the compaction chain.
func (nb *nonBlocking) BlockDelivered(blockHash common.Hash,
	blockPosition types.Position, rand []byte) {
	nb.addEvent(blockDeliveredEvent{
		blockHash:     blockHash,
		blockPosition: blockPosition,
		rand:          rand,
	})
}
