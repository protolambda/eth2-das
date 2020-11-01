# Eth2 Data Availability Sampling - Testground plan

This repository is a test plan to import into [Testground](https://github.com/testground/testground).
It tests the viability of various networking ideas for Phase 1 of Eth2.

## DAS experiment

Background:
- [Original write-up of Vitalik with different approaches](https://notes.ethereum.org/@vbuterin/r1v8VCULP)
- [Expansion on approach 2 by protolambda, using gossip subnets](https://notes.ethereum.org/McLGvrWgSX6Cpg60ewMYeQ)

The idea is to implement a proof of concept in Go, and run it on Testground with different parameters.
**Work in progress, highly experimental**

### DAS spec

A draft of the phase 1 specification can be found in the [`./spec`](./spec) folder.
This includes:
 - [How sampling is organized](./spec/das_sampling.md)
 - [How validators run DAS](./spec/das_validator.md)
 - [How to deal with points of data](./spec/data_as_points.md)
 - [The network spec for shards and DAS](./spec/shards_p2p.md)

### The `eth2node` package

A mock Eth2 node with the Phase 1 functionality, and instrumentation for gossip tests.

- register topics:
  - `VERTICAL_SUBNETS` DAS subnets
  - `HORIZONTAL_SUBNETS` Distribution subnets, could be shard subnets.
  - Shard headers global topic
- Subscribe to `P` DAS subnets randomly based on publicly known seed, unique to peer.
  - For stability
  - Find other peers via discovery (TODO add discovery)
- Subscribe to `K` DAS subnets randomly 
  - The random sampling necessary to make DAS work secure enough
  - Verify if all the seen samples are correct
    - TODO: keep track of what has been seen, *and what has not*
      - Adjust forkchoice to steer clear of unavailable shard data
- Subscribe randomly to a shard block subnet for every Validator that runs
  - Rotate out on random `[EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION, 2 * EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION]` interval (per shard)
  - For any seen block, split it up in samples, and for each DAS subnet we are on, propagate the relevant samples to it.
- Subscribe to shard headers topic
- Validate DAS subnets:
  - TODO
- Validate Shard subnets:
  - TODO
- Validate Shard headers topic:
  - TODO

### Misc. configurables

Not part of the DAS spec, but for testing purposes:

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `PEER_COUNT_LO` | `120` | peers | How many peers for low-water |
| `PEER_COUNT_HI` | `200` | peers | How many peers to maintain for hi-water |
| `SHARD_COUNT` | `64` | shards | Number of shards |
| `SECONDS_PER_SLOT` | `12` | seconds | Number of seconds in each slot |
| `VALIDATOR_COUNT` | `150000` | validators | Number of active validators |
| `NODE_COUNT` | `10000` | nodes | Physical validator nodes in the p2p network |
| `AVERAGE_VALIDATORS_PER_NODE` | `VALIDATOR_COUNT / NODE_COUNT` | validators | validators per node | 

## License

MIT, see [`LICENSE`](./LICENSE) file.
