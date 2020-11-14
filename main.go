package main

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/protolambda/eth2-das/eth2node"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"net"
	"time"
)

var testcases = map[string]interface{}{
	"das": run.InitializedTestCaseFn(das),
}

func main() {
	run.InvokeMap(testcases)
}

func das(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	ctx := context.Background()
	// TODO parameters
	conf := &eth2node.Config{
		FAST_INDICES:                16,
		SLOW_INDICES:                4,
		MAX_SAMPLES_PER_SHARD_BLOCK: 16,
		POINTS_PER_SAMPLE:           16,
		SLOTS_PER_FAST_ROTATION_MAX: 32,
		SLOTS_PER_SLOW_ROTATION:     2048,
		SLOT_OFFSET_PER_SLOW_INDEX:  512,
		SHARD_COUNT:                 64,
		SECONDS_PER_SLOT:            12,
		VALIDATOR_COUNT:             150000,
		ForkDigest:                  [4]byte{0xaa, 0xbb, 0xcc, 0xdd},
		TARGET_PEERS_PER_DAS_SUB:    6,
		PEER_COUNT_LO:               120,
		PEER_COUNT_HI:               200,
		GENESIS_TIME:                uint64(time.Now().Unix()), // TODO
		SHUFFLE_ROUND_COUNT:         90,
		// TODO gossipsub score tuning (ignored for now)
		VERT_SUBNET_TOPIC_SCORE_PARAMS:   nil,
		HORZ_SUBNET_TOPIC_SCORE_PARAMS:   nil,
		SHARD_HEADERS_TOPIC_SCORE_PARAMS: nil,
		GOSSIP_GLOBAL_SCORE_PARAMS:       nil,
		GOSSIP_GLOBAL_SCORE_THRESHOLDS:   nil,
	}
	// TODO: use Testground sync to learn all peer IDs and their addresses
	disc := &eth2node.MockDiscovery{
		Peers: make(map[peer.ID][]ma.Multiaddr),
	}
	n, err := eth2node.New(ctx, conf, disc, runenv.SLogger())
	if err != nil {
		return errors.Wrap(err, "failed to start eth2 node")
	}

	// Select a subset of validators based on global sequence number of this node.
	// (TODO: alternatively use testground comms)
	nodeCount := uint64(100)
	start := conf.VALIDATOR_COUNT * uint64(initCtx.GlobalSeq) / nodeCount
	end := conf.VALIDATOR_COUNT * uint64(initCtx.GlobalSeq+1) / nodeCount
	count := end - start
	indices := make([]beacon.ValidatorIndex, count, count)
	for i := uint64(0); i < count; i++ {
		indices[i] = beacon.ValidatorIndex(start + i)
	}
	// TODO: generate interop BLS keys for validators maybe?
	n.RegisterValidators(indices...)

	if err := n.Start(net.IPv4zero, 9000); err != nil {
		return errors.Wrap(err, "failed to start node")
	}

	// TODO configure test time
	time.Sleep(time.Minute * 10)

	if err := n.Close(); err != nil {
		return errors.Wrap(err, "failed to close gracefully")
	}

	return nil
}
