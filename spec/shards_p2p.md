# Phase 1 networking specification

This document describes how the p2p network is organized for Shards and Data Availaibility Sampling.

It extends each of the domains of the networking specification introduced in Phase 0.

## Table of contents

<!-- TOC -->
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
TODO TOC
<!-- END doctoc generated TOC please keep comment here to allow auto update -->
<!-- /TOC -->


## Constants / configurables

Don't take values as canonical. The point is to find good values during Phase 1 prototyping.

### Configurables

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `TARGET_PEERS_PER_DAS_SUB` | `6` | peers | How many peers should be acquired per topic. If insufficient, search for more with discovery. |

## GossipSub

### Topic names

The topic names follow the same schema as in phase 0: `/eth2/ForkDigestValue/Name/Encoding`
- `eth2` places it in the Eth2 namespace
- `ForkDigestValue` is set to the hex-encoded fork digest, without `0x` prefix.
- `Name` see table below.
- `Encoding` the `ssz_snappy` encoding is used.

| Name                             | Message Type              |
|----------------------------------|---------------------------|
| `shard_headers`                  | `ShardBlockHeader`        |
| `das_vert_{vertical_index}`      | `DASSample`               |
| `das_horz_{horizontal_index}`    | `ShardBlockData`          |

TODO: depending on a shard or beacon-proposer approach, the `shard_headers` are signed or not.

### Topic validation

TODO

## Req-Resp


## Discovery

### DAS subnet discovery

The node uses a simple discovery interface:
```python
class Discovery(Protocol):
	def find_public(conf: Config, slot: Slot, subnets: Set[VerticalIndex]) -> Dict[VerticalIndex, Set[PeerID]]: ...
	def addrs(id: PeerID) -> List[Multiaddr]: ...

```

It lists possible candidates for each subnet, based on public subnet info known of the peers: their `P` subset.

In preparation of rotation of subnets, the Eth2 node can then try to peer with peers that complement their current peers,
 to allow gossipsub to build a mesh for the topic.

### Shard subnet discovery

TODO: how to find nodes on horizontal subnets, where shard data is gossiped in full form.

