package main

import (
	"context"
	"github.com/pkg/errors"
	"github.com/protolambda/eth2-das/eth2node"
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
		EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: 256,
		VALIDATOR_COUNT:                       150000, // TODO
		NODE_COUNT:                            10000,  // TODO
	}
	n, err := eth2node.New(ctx, conf, runenv.SLogger())
	if err != nil {
		return errors.Wrap(err, "failed to start eth2 node")
	}

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
