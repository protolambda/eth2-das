package eth2node

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/protolambda/zrnt/eth2/beacon"
)

// shardCommitteeShuffling computes mock shard committee assignments for each shard.
func (n *Eth2Node) shardCommitteeShuffling(seedNum uint64) (shard2Vals [][]ValidatorIndex, val2Shard []Shard) {
	// Don't use beacon chain, just do randomness based on slot, something simple
	h := sha256.New()
	binary.Write(h, binary.LittleEndian, seedNum)
	var seed beacon.Root
	copy(seed[:], h.Sum(nil))

	// init array with all indices. Everyone is active in this experiment
	out := make([]ValidatorIndex, n.conf.VALIDATOR_COUNT, n.conf.VALIDATOR_COUNT)
	for i := ValidatorIndex(0); i < ValidatorIndex(len(out)); i++ {
		out[i] = i
	}

	// shuffle in-place
	beacon.ShuffleList(n.conf.SHUFFLE_ROUND_COUNT, out, seed)

	shard2Vals = make([][]ValidatorIndex, n.conf.SHARD_COUNT, n.conf.SHARD_COUNT)
	val2Shard = make([]Shard, n.conf.VALIDATOR_COUNT, n.conf.VALIDATOR_COUNT)
	// keep this simple, just split between all shards. No inactive shards or double work.
	for i := uint64(0); i < n.conf.SHARD_COUNT; i++ {
		start := n.conf.VALIDATOR_COUNT * i / n.conf.SHARD_COUNT
		end := n.conf.VALIDATOR_COUNT * (i + 1) / n.conf.SHARD_COUNT
		shard2Vals[i] = out[start:end]
		for _, val := range out[start:end] {
			val2Shard[val] = Shard(i)
		}
	}
	return shard2Vals, val2Shard
}
