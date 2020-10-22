package eth2node

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/protolambda/zrnt/eth2/beacon"
)

func (n *Eth2Node) scheduleShardProposalsMaybe(slot Slot) {
	proposers := n.computeShardProposers(slot)
	for shard, proposer := range proposers {
		if _, ok := n.localValidators[proposer]; ok {
			go n.executeShardBlockProposal(slot, Shard(shard), proposer)
		}
	}
}

// proposers, 1 per shard, for all shards
func (n *Eth2Node) computeShardProposers(slot Slot) []ValidatorIndex {
	out := make([]ValidatorIndex, n.conf.SHARD_COUNT, n.conf.SHARD_COUNT)
	h := sha256.New()
	for shard := Shard(0); shard < Shard(n.conf.SHARD_COUNT); shard++ {
		// Mock seed, since there's no beacon chain. Just decide on a proposer by using the time and shard as seed.
		h.Reset()
		binary.Write(h, binary.LittleEndian, uint64(slot))
		binary.Write(h, binary.LittleEndian, uint64(shard))
		var seed beacon.Root
		copy(seed[:], h.Sum(nil))
		// Take the committee, shuffle lookup for 0, to get any random committee member as proposer.
		// Not the actual proposer logic, but good enough for testing
		committeee := n.shard2Vals[shard]
		proposerCommIndex := beacon.PermuteIndex(n.conf.SHUFFLE_ROUND_COUNT, 0, uint64(len(committeee)), seed)
		proposer := committeee[proposerCommIndex]

		out[shard] = proposer
	}
	return out
}

func (n *Eth2Node) executeShardBlockProposal(slot Slot, shard Shard, proposer ValidatorIndex) {
	// TODO create shard block
	// TODO put it on the shard committee topic
	// TODO put its header on the shard headers topic
}
