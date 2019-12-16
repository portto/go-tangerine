// Copyright 2019 The dexon-consensus Authors
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

package dexcon

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/portto/go-tangerine/common"
	"github.com/portto/go-tangerine/core/state"
	"github.com/portto/go-tangerine/core/vm"
	"github.com/portto/go-tangerine/crypto"
	"github.com/portto/go-tangerine/ethdb"
	"github.com/portto/go-tangerine/params"
)

type govStateFetcher struct {
	statedb *state.StateDB
}

func (g *govStateFetcher) GetConfigState(_ uint64) (*vm.GovernanceState, error) {
	return &vm.GovernanceState{g.statedb}, nil
}

func (g *govStateFetcher) DKGSetNodeKeyAddresses(round uint64) (map[common.Address]struct{}, error) {
	return make(map[common.Address]struct{}), nil
}

type DexconTestSuite struct {
	suite.Suite

	config  *params.DexconConfig
	memDB   *ethdb.MemDatabase
	stateDB *state.StateDB
	s       *vm.GovernanceState
}

func (d *DexconTestSuite) SetupTest() {
	memDB := ethdb.NewMemDatabase()
	stateDB, err := state.New(common.Hash{}, state.NewDatabase(memDB))
	if err != nil {
		panic(err)
	}
	d.memDB = memDB
	d.stateDB = stateDB
	d.s = &vm.GovernanceState{stateDB}

	config := params.TestnetChainConfig.Dexcon
	config.LockupPeriod = 1000
	config.NextHalvingSupply = new(big.Int).Mul(big.NewInt(1e18), big.NewInt(2.5e9))
	config.LastHalvedAmount = new(big.Int).Mul(big.NewInt(1e18), big.NewInt(1.5e9))
	config.MiningVelocity = 0.1875
	config.RoundLength = 3600
	config.MinBlockInterval = 1000

	d.config = config

	// Give governance contract balance so it will not be deleted because of being an empty state object.
	stateDB.AddBalance(vm.GovernanceContractAddress, big.NewInt(1))

	// Genesis CRS.
	crs := crypto.Keccak256Hash([]byte(config.GenesisCRSText))
	d.s.SetCRS(crs)

	// Round 0 height.
	d.s.PushRoundHeight(big.NewInt(0))

	// Governance configuration.
	d.s.UpdateConfiguration(config)

	d.stateDB.Commit(true)
}

func (d *DexconTestSuite) TestBlockRewardCalculation() {
	consensus := New()
	consensus.SetGovStateFetcher(&govStateFetcher{d.stateDB})

	d.s.IncTotalStaked(big.NewInt(1e18))

	// blockReard = miningVelocity * totalStaked * roundInterval / aYear / numBlocksInCurRound
	// 0.1875 * 1e18 * 3600 * 1000 / (86400 * 1000 * 365 * 3600) = 5945585996.96
	d.Require().Equal(big.NewInt(5945585996), consensus.calculateBlockReward(0))
}

func TestDexcon(t *testing.T) {
	suite.Run(t, new(DexconTestSuite))
}
