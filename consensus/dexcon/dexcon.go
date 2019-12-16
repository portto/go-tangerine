// Copyright 2017 The DEXON Authors
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

package dexcon

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/portto/go-tangerine/common"
	"github.com/portto/go-tangerine/consensus"
	"github.com/portto/go-tangerine/core/state"
	"github.com/portto/go-tangerine/core/types"
	"github.com/portto/go-tangerine/core/vm"
	"github.com/portto/go-tangerine/log"
	"github.com/portto/go-tangerine/rpc"
	dexCore "github.com/portto/tangerine-consensus/core"
)

type GovernanceStateFetcher interface {
	GetConfigState(round uint64) (*vm.GovernanceState, error)
	DKGSetNodeKeyAddresses(round uint64) (map[common.Address]struct{}, error)
}

// Dexcon is a delegated proof-of-stake consensus engine.
type Dexcon struct {
	govStateFetcer GovernanceStateFetcher
}

// New creates a Clique proof-of-authority consensus engine with the initial
// signers set to the ones provided by the user.
func New() *Dexcon {
	return &Dexcon{}
}

// SetGovStateFetcher sets the config fetcher for Dexcon. The reason this is not
// passed in the New() method is to bypass cycle dependencies when initializing
// dex backend.
func (d *Dexcon) SetGovStateFetcher(fetcher GovernanceStateFetcher) {
	d.govStateFetcer = fetcher
}

// Author implements consensus.Engine, returning the Ethereum address recovered
// from the signature in the header's extra-data section.
func (d *Dexcon) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (d *Dexcon) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (d *Dexcon) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort, results := make(chan struct{}), make(chan error)
	go func() {
		for range headers {
			results <- nil
		}
	}()
	return abort, results
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (d *Dexcon) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	return nil
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (d *Dexcon) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	return nil
}

// VerifyUncles implements consensus.Engine, always returning an error for any
// uncles as this consensus mechanism doesn't permit uncles.
func (d *Dexcon) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (d *Dexcon) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header for running the transactions on top.
func (d *Dexcon) Prepare(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

func (d *Dexcon) inExtendedRound(header *types.Header, state *state.StateDB) bool {
	gs := vm.GovernanceState{state}
	rgs, err := d.govStateFetcer.GetConfigState(header.Round)
	if err != nil {
		panic(err)
	}

	roundEnd := gs.RoundHeight(new(big.Int).SetUint64(header.Round)).Uint64() + rgs.RoundLength().Uint64()

	// Round 0 starts and height 0 instead of height 1.
	if header.Round == 0 {
		roundEnd += 1
	}
	return header.Number.Uint64() >= roundEnd
}

func (d *Dexcon) calculateBlockReward(round uint64) *big.Int {
	gs, err := d.govStateFetcer.GetConfigState(round)
	if err != nil {
		panic(err)
	}
	config := gs.Configuration()

	blocksPerRound := config.RoundLength
	roundInterval := new(big.Float).Mul(
		big.NewFloat(float64(blocksPerRound)),
		big.NewFloat(float64(config.MinBlockInterval)))

	// blockReard = miningVelocity * totalStaked * roundInterval / aYear / numBlocksInCurRound
	numerator, _ := new(big.Float).Mul(
		new(big.Float).Mul(
			big.NewFloat(float64(config.MiningVelocity)),
			new(big.Float).SetInt(gs.TotalStaked())),
		roundInterval).Int(nil)

	reward := new(big.Int).Div(numerator,
		new(big.Int).Mul(
			big.NewInt(86400*1000*365),
			big.NewInt(int64(blocksPerRound))))

	return reward
}

// Finalize implements consensus.Engine, ensuring no uncles are set, nor block
// rewards given, and returns the final block.
func (d *Dexcon) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	gs := vm.GovernanceState{state}

	height := gs.RoundHeight(new(big.Int).SetUint64(header.Round))

	// The first block of a round is found.
	if header.Round > 0 && height.Uint64() == 0 {
		gs.PushRoundHeight(header.Number)

		if header.Round > dexCore.DKGDelayRound {
			// Check for dead node and disqualify them.
			// A dead node node is defined as: a notary set node that did not propose
			// any block in the past round.
			addrs, err := d.govStateFetcer.DKGSetNodeKeyAddresses(header.Round - 1)
			if err != nil {
				panic(err)
			}

			gcs, err := d.govStateFetcer.GetConfigState(header.Round - 1)
			if err != nil {
				panic(err)
			}

			for addr := range addrs {
				offset := gcs.NodesOffsetByNodeKeyAddress(addr)
				if offset.Cmp(big.NewInt(0)) < 0 {
					panic(fmt.Errorf("invalid notary set found, addr = %s", addr.String()))
				}

				node := gcs.Node(offset)
				lastHeight := gs.LastProposedHeight(node.Owner)
				prevRoundHeight := gs.RoundHeight(big.NewInt(int64(header.Round - 1)))

				if lastHeight.Uint64() < prevRoundHeight.Uint64() {
					log.Debug("Disqualify node", "round", header.Round, "nodePubKey", hex.EncodeToString(node.PublicKey))
					err = gs.Disqualify(node)
					if err != nil {
						log.Error("Failed to disqualify node", "err", err)
					}
				}
			}
		}
	}

	// Distribute block reward and halving condition.
	reward := new(big.Int)

	// If this is not an empty block and we are not in extended round, calculate
	// the block reward.
	if header.Coinbase != (common.Address{}) && !d.inExtendedRound(header, state) {
		reward = d.calculateBlockReward(header.Round)
	}

	header.Reward = reward
	state.AddBalance(header.Coinbase, reward)
	gs.IncTotalSupply(reward)

	// Check if halving checkpoint reached.
	config := gs.Configuration()
	if gs.TotalSupply().Cmp(config.NextHalvingSupply) >= 0 {
		gs.MiningHalved()
	}

	if header.Coinbase != (common.Address{}) {
		// Record last proposed height.
		gs.PutLastProposedHeight(header.Coinbase, header.Number)
	}

	header.Root = state.IntermediateRoot(true)
	return types.NewBlock(header, txs, uncles, receipts), nil
}

// Seal implements consensus.Engine, attempting to create a sealed block using
// the local signing credentials.
func (d *Dexcon) Seal(chain consensus.ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	return nil
}

// SealHash returns the hash of a block prior to it being sealed.
func (d *Dexcon) SealHash(header *types.Header) (hash common.Hash) {
	return common.Hash{}
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (d *Dexcon) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	return big.NewInt(0)
}

// Close implements consensus.Engine. It's a noop for clique as there is are no background threads.
func (d *Dexcon) Close() error {
	return nil
}

// APIs implements consensus.Engine, returning the user facing RPC API to allow
// controlling the signer voting.
func (d *Dexcon) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{}
}
