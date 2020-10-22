package eth2node

import "go.uber.org/zap"

func (n *Eth2Node) subscribeShardSubnets() {
	// no changing of committees yet. Just used current committee shuffling in node state
	shards := make(map[Shard]struct{})
	for val := range n.localValidators {
		shard := n.val2Shard[val]
		shards[shard] = struct{}{}
	}
	n.log.With("validating_shards", shards).Info("validating on shards")
	for shard := range shards {
		topic := n.shardSubnets[shard]
		sub, err := topic.Subscribe()
		if err != nil {
			n.log.With(zap.Error(err)).Error("failed to subscribe to shard")
		}
		n.shardSubs[shard] = sub
		go n.shardHandleSubnet(shard, sub)
	}
}
