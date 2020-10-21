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
		K:                                     16,
		P:                                     4,
		CHUNK_SIZE:                            512,
		MAX_BLOCK_SIZE:                        524288,
		SHARD_HEADER_SIZE:                     256,
		SHARD_COUNT:                           64,
		SECONDS_PER_SLOT:                      12,
		SLOTS_PER_K_ROTATION_MAX:              32, // TODO
		SLOTS_PER_P_ROTATION:                  320,
		SLOT_OFFSET_PER_P_INDEX:               16,
		EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: 256,
		VALIDATOR_COUNT:                       150000, // TODO
		NODE_COUNT:                            10000,  // TODO
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
	start := conf.VALIDATOR_COUNT * uint64(initCtx.GlobalSeq) / conf.NODE_COUNT
	end := conf.VALIDATOR_COUNT * uint64(initCtx.GlobalSeq+1) / conf.NODE_COUNT
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
