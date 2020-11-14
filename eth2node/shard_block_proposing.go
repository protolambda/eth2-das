package eth2node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/pkg/errors"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"go.uber.org/zap"
	"io"
	"math/rand"
	"time"
)

func (n *Eth2Node) scheduleShardProposalsMaybe(slot Slot) {
	proposers := n.computeShardProposers(slot)
	for shard, proposer := range proposers {
		if _, ok := n.localValidators[proposer]; ok {
			n.log.With("proposer", proposer, "slot", slot, "shard", shard).Info("proposing shard block")
			go func() {
				if err := n.executeShardBlockProposal(slot, Shard(shard), proposer); err != nil {
					n.log.With(zap.Error(err)).Errorf("proposer %d error for slot %d", proposer, slot)
				}
			}()
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

func (n *Eth2Node) executeShardBlockProposal(slot Slot, shard Shard, proposer ValidatorIndex) error {
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
			return errors.Wrap(err, "proposer failed to encode header")
		}
		if err := n.shardHeaders.Publish(ctx, buf.Bytes()); err != nil {
			return errors.Wrap(err, "proposer failed to publish header")
		}
	}

	// Publish block to horizontal net
	{
		var buf bytes.Buffer
		if err := block.Serialize(codec.NewEncodingWriter(&buf)); err != nil {
			return errors.Wrap(err, "proposer failed to encode block")
		}
		if err := n.horizontalSubnets[shard].Publish(ctx, buf.Bytes()); err != nil {
			return errors.Wrap(err, "proposer failed to publish block to horizontal net")
		}
	}

	// Publish samples to vertical nets
	{
		samples, err := n.conf.MakeSamples(block.Message.Body)
		if err != nil {
			return errors.Wrap(err, "proposer failed to make samples")
		}
		// TODO: how long should the node try to spend on getting a publishing round done before skipping?
		ctx, _ := context.WithTimeout(n.subProcesses.ctx, 2*time.Second*time.Duration(n.conf.SECONDS_PER_SLOT))

		for i, sample := range samples {
			go func(i int, sample ShardBlockDataChunk) { // TODO: high parallelism here, maybe too much, might need to change it
				var buf bytes.Buffer
				if err := sample.Serialize(codec.NewEncodingWriter(&buf)); err != nil {
					n.log.With(zap.Error(err)).Error("failed to encode sample for vert net")
					return
				}
				err := n.verticalSubnets[i].Publish(ctx, buf.Bytes())
				if err != nil {
					n.log.With(zap.Error(err)).Error("failed to publish to vert net")
					return
				}
			}(i, sample)
		}
	}
	return nil
}
