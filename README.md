# **DEPRECATED**

My view on Data Availability Sampling has changed, exploring UDP (discv5 based) instead of TCP (gossipsub based) sampling looks more promising.

# Eth2 Data Availability Sampling

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

A mock Eth2 node with just the Phase 1 DAS functionality (sampling, networking, propagation, validation, processing), and instrumentation for gossip tests.

### Misc. configurables

Not part of the DAS spec, but for testing purposes:

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `PEER_COUNT_LO` | `120` | peers | How many peers for low-water |
| `PEER_COUNT_HI` | `200` | peers | How many peers to maintain for hi-water |
| `SHARD_COUNT` | `64` | shards | Number of shards |
| `SECONDS_PER_SLOT` | `12` | seconds | Number of seconds in each slot |
| `VALIDATOR_COUNT` | `150000` | validators | Number of active validators |

## License

MIT, see [`LICENSE`](./LICENSE) file.
