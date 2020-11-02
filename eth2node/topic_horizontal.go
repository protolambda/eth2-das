package eth2node

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/zap"
)

func (n *Eth2Node) subscribeHorzSubnets() {
	// no changing of committees implemented in prototype.
	// Just used current committee shuffling in node state

	// Find all shards the node is participating in
	shards := make(map[Shard]struct{})
	for val := range n.localValidators {
		shard := n.val2Shard[val]
		shards[shard] = struct{}{}
	}
	n.log.With("validating_shards", shards).Info("validating on shards")
	for shard := range shards {
		topic := n.horizontalSubnets[shard]
		sub, err := topic.Subscribe()
		if err != nil {
			n.log.With(zap.Error(err)).Error("failed to subscribe to shard")
		}
		n.horizontalSubs[shard] = sub
		go n.horzHandleSubnet(Shard(shard), sub)
	}
}

func (n *Eth2Node) horzSubnetValidator(index Shard) pubsub.ValidatorEx {
	return func(ctx context.Context, p peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
		// TODO subnet validation
		return pubsub.ValidationAccept
	}
}

func (n *Eth2Node) horzHandleSubnet(index Shard, sub *pubsub.Subscription) {
	{
		msg, err := sub.Next(n.subProcesses.ctx)
		if err != nil {
			if err == n.subProcesses.ctx.Err() {
				return
			}
			n.log.With(zap.Error(err)).With("subnet", index).Error("failed to read from subnet subscription")
			sub.Cancel()
			return
		}
		n.log.With("from", msg.ReceivedFrom, "length", len(msg.Data)).Debug("received message")

		// TODO
		// Each node that receives the shard block on a shard subnet,
		// divides it into CHUNK_SIZE chunks ordered from chunk 0 to chunk MAX_BLOCK_SIZE / CHUNK_SIZE.
		// The node then takes the chunks corresponding to the node’s node_indices and broadcasts each on its particular subnet

	}
}
