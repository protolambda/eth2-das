package eth2node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"io"
	"math/rand"
	"time"
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
	// create shard block, with mock data
	dataSize := n.conf.MAX_DATA_SIZE
	data := make([]byte, dataSize, dataSize)
	seed := int64(uint64(slot)*n.conf.SHARD_COUNT + uint64(shard))
	rng := rand.New(rand.NewSource(seed))
	if _, err := io.ReadFull(rng, data); err != nil {
		panic(fmt.Errorf("failed to create random mock data: %v", err))
	}
	block := SignedShardBlock{
		Message: ShardBlock{
			ShardParentRoot:  Root{}, // TODO
			BeaconParentRoot: Root{}, // TODO
			Slot:             slot,
			Shard:            shard,
			ProposerIndex:    proposer,
			Body:             ShardBlockData(data),
		},
		Signature: BLSSignature{}, // TODO
	}
	header := SignedShardBlockHeader{
		Message: ShardBlockHeader{
			ShardParentRoot:  Root{}, // TODO
			BeaconParentRoot: Root{}, // TODO
			Slot:             slot,
			Shard:            shard,
			ProposerIndex:    proposer,
			BodyRoot:         Root{}, // TODO
		},
		Signature: BLSSignature{}, // TODO
	}

	// try publishing everything for the extension of 2/3 of a slot. Give up afterwards.
	slotDuration := time.Second * time.Duration(n.conf.SECONDS_PER_SLOT)
	ctx, _ := context.WithTimeout(n.subProcesses.ctx, 2*slotDuration/3)
	// Publish header to global net
	{
		var buf bytes.Buffer
		if err := header.Serialize(codec.NewEncodingWriter(&buf)); err != nil {

		}
		if err := n.shardHeaders.Publish(ctx, buf.Bytes()); err != nil {

		}
	}

	// Publish block to horizontal net
	{
		var buf bytes.Buffer
		if err := block.Serialize(codec.NewEncodingWriter(&buf)); err != nil {

		}
		if err := n.horizontalSubnets[shard].Publish(ctx, buf.Bytes()); err != nil {

		}
	}

	// Publish samples to vertical nets
	{
		samples, err := n.conf.MakeSamples(block.Message.Body)
		if err != nil {

		}
		for i, sample := range samples {
			// TODO publish
		}
	}
}
