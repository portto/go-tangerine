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
	"time"

	"github.com/portto/tangerine-consensus/common"
	"github.com/portto/tangerine-consensus/core/crypto"
	"github.com/portto/tangerine-consensus/core/types"
	typesDKG "github.com/portto/tangerine-consensus/core/types/dkg"
)

// Application describes the application interface that interacts with DEXON
// consensus core.
type Application interface {
	// PreparePayload is called when consensus core is preparing a block.
	PreparePayload(position types.Position) ([]byte, error)

	// PrepareWitness will return the witness data no lower than consensusHeight.
	PrepareWitness(consensusHeight uint64) (types.Witness, error)

	// VerifyBlock verifies if the block is valid.
	VerifyBlock(block *types.Block) types.BlockVerifyStatus

	// BlockConfirmed is called when a block is confirmed and added to lattice.
	BlockConfirmed(block types.Block)

	// BlockDelivered is called when a block is added to the compaction chain.
	BlockDelivered(hash common.Hash, position types.Position, rand []byte)
}

// Debug describes the application interface that requires
// more detailed consensus execution.
type Debug interface {
	// BlockReceived is called when the block received in agreement.
	BlockReceived(common.Hash)
	// BlockReady is called when the block's randomness is ready.
	BlockReady(common.Hash)
}

// Network describs the network interface that interacts with DEXON consensus
// core.
type Network interface {
	// PullBlocks tries to pull blocks from the DEXON network.
	PullBlocks(hashes common.Hashes)

	// PullVotes tries to pull votes from the DEXON network.
	PullVotes(position types.Position)

	// BroadcastVote broadcasts vote to all nodes in DEXON network.
	BroadcastVote(vote *types.Vote)

	// BroadcastBlock broadcasts block to all nodes in DEXON network.
	BroadcastBlock(block *types.Block)

	// BroadcastAgreementResult broadcasts agreement result to DKG set.
	BroadcastAgreementResult(randRequest *types.AgreementResult)

	// SendDKGPrivateShare sends PrivateShare to a DKG participant.
	SendDKGPrivateShare(pub crypto.PublicKey, prvShare *typesDKG.PrivateShare)

	// BroadcastDKGPrivateShare broadcasts PrivateShare to all DKG participants.
	BroadcastDKGPrivateShare(prvShare *typesDKG.PrivateShare)

	// BroadcastDKGPartialSignature broadcasts partialSignature to all
	// DKG participants.
	BroadcastDKGPartialSignature(psig *typesDKG.PartialSignature)

	// ReceiveChan returns a channel to receive messages from DEXON network.
	ReceiveChan() <-chan types.Msg

	// ReportBadPeerChan returns a channel to report bad peer.
	ReportBadPeerChan() chan<- interface{}
}

// Governance interface specifies interface to control the governance contract.
// Note that there are a lot more methods in the governance contract, that this
// interface only define those that are required to run the consensus algorithm.
type Governance interface {
	// Configuration returns the configuration at a given round.
	// Return the genesis configuration if round == 0.
	Configuration(round uint64) *types.Config

	// CRS returns the CRS for a given round. Return the genesis CRS if
	// round == 0.
	//
	// The CRS returned is the proposed or latest reseted one, it would be
	// changed later if corresponding DKG set failed to generate group public
	// key.
	CRS(round uint64) common.Hash

	// Propose a CRS of round.
	ProposeCRS(round uint64, signedCRS []byte)

	// NodeSet returns the node set at a given round.
	// Return the genesis node set if round == 0.
	NodeSet(round uint64) []crypto.PublicKey

	// Get the begin height of a round.
	GetRoundHeight(round uint64) uint64

	//// DKG-related methods.

	// AddDKGComplaint adds a DKGComplaint.
	AddDKGComplaint(complaint *typesDKG.Complaint)

	// DKGComplaints gets all the DKGComplaints of round.
	DKGComplaints(round uint64) []*typesDKG.Complaint

	// AddDKGMasterPublicKey adds a DKGMasterPublicKey.
	AddDKGMasterPublicKey(masterPublicKey *typesDKG.MasterPublicKey)

	// DKGMasterPublicKeys gets all the DKGMasterPublicKey of round.
	DKGMasterPublicKeys(round uint64) []*typesDKG.MasterPublicKey

	// AddDKGMPKReady adds a DKG ready message.
	AddDKGMPKReady(ready *typesDKG.MPKReady)

	// IsDKGMPKReady checks if DKG's master public key preparation is ready.
	IsDKGMPKReady(round uint64) bool

	// AddDKGFinalize adds a DKG finalize message.
	AddDKGFinalize(final *typesDKG.Finalize)

	// IsDKGFinal checks if DKG is final.
	IsDKGFinal(round uint64) bool

	// AddDKGSuccess adds a DKG success message.
	AddDKGSuccess(success *typesDKG.Success)

	// IsDKGSuccess checks if DKG is success.
	IsDKGSuccess(round uint64) bool

	// ReportForkVote reports a node for forking votes.
	ReportForkVote(vote1, vote2 *types.Vote)

	// ReportForkBlock reports a node for forking blocks.
	ReportForkBlock(block1, block2 *types.Block)

	// ResetDKG resets latest DKG data and propose new CRS.
	ResetDKG(newSignedCRS []byte)

	// DKGResetCount returns the reset count for DKG of given round.
	DKGResetCount(round uint64) uint64
}

// Ticker define the capability to tick by interval.
type Ticker interface {
	// Tick would return a channel, which would be triggered until next tick.
	Tick() <-chan time.Time

	// Stop the ticker.
	Stop()

	// Retart the ticker and clear all internal data.
	Restart()
}

// Recovery interface for interacting with recovery information.
type Recovery interface {
	// ProposeSkipBlock proposes a skip block.
	ProposeSkipBlock(height uint64) error

	// Votes gets the number of votes of given height.
	Votes(height uint64) (uint64, error)
}
