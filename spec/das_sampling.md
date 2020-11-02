# Phase 1 DAS sampling

This document describes how the sampling of the data-availaibility-sampling (DAS) works.
It abstracts away any lower-level details of individual points in a sample see [DAS points](./das_points.md) for that.
And the validator and network specification build on this sampling specification. 

## Table of contents

<!-- TOC -->
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
TODO TOC
<!-- END doctoc generated TOC please keep comment here to allow auto update -->
<!-- /TOC -->

## Custom types

| Name | SSZ equivalent | Description |
| - | - | - |
| `VerticalIndex` | `uint64` | An index identifying a DAS subnet |
| `HorizontalIndex` | `uint64` | An index identifying a horizontal subnet, this could be a shard subnet. |
| `SampleIndex` | `uint64` | An index of a sample, w.r.t. the sample-ified extended points of a shard data blob | 
| `Sample` | `Vector[RawPoint, POINTS_PER_SAMPLE]` | A sequence of points, sample unit |

## Constants / configurables

Don't take values as canonical. The point is to find good values during DAS prototyping.

### Configurables

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `FAST_INDICES` | `16` | indices | Subset of indices that nodes sample, privately chosen, and replaced quickly |
| `SLOW_INDICES` | `4` | indices | Subset of indices that nodes sample, publicly determined, and replaced slowly |
| `MAX_SAMPLES_PER_SHARD_BLOCK` | `16` | samples | Maximum number of samples in which the extended points are split into |
| `POINTS_PER_SAMPLE` | `16` | points | Number of points per sample |
| `MAX_SHARD_BLOCK_SIZE` | `524288` | bytes | Number of bytes in a shard block |
| `SLOTS_PER_FAST_ROTATION_MAX` | `32` | slots | Maximum of how frequently a fast vertical subnet subscriptions is randomly swapped. Rotations of a subnet can happen any time between 1 and `SLOTS_PER_FAST_ROTATION_MAX` (incl) slots. |
| `SLOTS_PER_SLOW_ROTATION` | `2048` | slots | How frequently a slow vertical subnet subscriptions is randomly swapped. (deterministic on peer ID, so public and predictable) |
| `SLOT_OFFSET_PER_SLOW_INDEX` | `512` | slots | The time for a slow vertical subscription to wait for the previous index to rotate, for stagger effect. |
| `SAMPLE_SUBNETS` | `MAX_SAMPLES_PER_SHARD_BLOCK * SHARD_COUNT = 16 * 64 = 1024` | subnets | Number of subnets to propagate total samples to | 


## Subnet sampling

For sampling, all peers need to query for `k` random samples each slot. This is a lot of work, and ideally happens at a low latency.
 
To achieve quick querying, the query model is changed to *push* the samples to listeners instead.
The listeners then randomly rotate their subscriptions to keep queries unpredictable.
Except for a small subset of subscriptions, which will function as a backbone to keep topics more stable.

This shifts routing workload to the publishers, of which are there are far less at a time.
Additionally, publishing can utilize the fan-out functionality in GossipSub, and is easier to split between nodes:
nodes on the horizontal networks can help by producing the same samples and fan-out publishing to their own peers.  

And best of all, a push model obfuscates the original source of a message: the listerners will not have to make individual queries to some identified source.

The push model does not aim to serve "historical" queries (anything older than the most recent).

When a sample is missing, there are a two different options: Wait, Pull

## Waiting on missing samples

Wait for the sample to re-broadcast. Someone may be slow with publishing, or someone else is able to do the work.

Any node can do extra work to keep the network healthy:
- Listen on a horizontal subnet, chunkify the block data in samples, and propagate the samples to vertical subnets.
- Listen on enough vertical subnets, reconstruct the missing samples by recovery, and propagate the recovered samples.

This is not a requirement, but can improve the network stability with little resources, and without any central party.

## Pulling missing samples

The more realistic option, to execute when a sample is missing, is to query any node that is known to hold it.
Since *consensus identity is disconnected from network identity*, there is no direct way to contact custody holders 
without explicitly asking for the data.

However, *network identities* are still used to build a backbone for each vertical subnet.
These nodes should have received the samples, and can serve a buffer of them on demand.
Although serving these is not directly incentivised, it is little work:
1. Buffer any message you see on the `SLOW_INDICES` vertical subnets, for a buffer of approximately two weeks.
2. Serve the samples on request. An individual sample is just `~ 0.5 KB`, and does not require any pre-processing to serve.

Pulling samples directly from nodes with a custody responsibility, without revealing their identity to the network, is an open problem. 


## Public/slow DAS subnet sampling

The slow indices are sampled as:
```python
def das_slow_peer_seed(peer_id: PeerID) -> Bytes32:
  return H(SLOW_DAS_SUBNETS_DOMAIN + peer_id)  # concat bytes

def das_slow_peer_slot_offset(peer_seed) -> Slot:
  return Slot(peer_seed[:8] % SLOTS_PER_SLOW_ROTATION)

def das_slow_subnet_slot_offset(i: VerticalIndex) -> Slot:
  return Slot(i * SLOT_OFFSET_PER_SLOW_INDEX) % SLOTS_PER_SLOW_ROTATION

def das_slow_subnet_index(peer_seed: Bytes32, slot: Slot, i: uint64) -> VerticalIndex:
  window_index = slot // SLOTS_PER_SLOW_ROTATION
  return H(peer_seed + serialize(i) + serialize(window_index))[:8] % SAMPLE_INDEX_SUBNETS

def das_slow_subnet_indices(peer_id: PeerID, slot: Slot, slow_count: int) -> Set[VerticalIndex]:
  peer_seed = das_slow_peer_seed(peer_id)
  peer_offset = das_slow_peer_slot_offset(peer_seed)
  return set(das_slow_subnet_index(
				peer_seed,
				slot + peer_offset + das_slow_subnet_slot_offset(i),
				i,
			) for i in range(slow_count))

slow_indices = das_slow_subnet_indices(peer_id, slot, SLOW_INDICES)
```

Any overlap in `slow_indices` is rare, and acceptable. The set will be a little smaller sometimes.

The idea of making it slow, public and predictable is:
- As backbone, for subnet stability
- Easy to discover peers through
- By using a seed based on peer ID, no data has to be tracked on discovery. No blow up in ENR size.
- Predictable enough to prepare subnet peering in advance
- Small value, other sampling can be unpredictable and rotate quicker.

Predictability here also avoids the need for:
- Additional information in ENR (e.g. list of indices). Just run the above functions on any peer ID.
- Complicated discovery. Just iterate the known peer IDs.

And offsets are used per peer and per subnet index, to avoid sudden synchronous changes in subscriptions across the network.

## Private/quick DAS subnet sampling

For the random sampling, `FAST_INDICES` indices should be queried randomly.
selected out of the total `SAMPLE_INDEX_SUBNETS`, except for what is already joined publicly (`SLOW_INDICES`).

Then on random intervals, the `FAST_INDICES` subnet subscriptions are swapped for other subnets.

This effectively creates a PUSH model where validators 
register themselves on short notice to be sampling a random subset.

The shard blocks are split up in samples, and should reach the validators with relatively low latency.

**Note**: depending on test results, a larger `FAST_INDICES` may be set.
The effect of quickly changing subscriptions is being researched.
Any topics that are subscribed to in e.g. the last few slots should not count as heavy towards bad data availability,
as it may just be an issue to subscribe to the topic fast enough.


```python
class FastInfo:
    sub: Subscription
    expiry: Slot

def random_subnet() -> VerticalIndex:
    return VerticalIndex(random_uint64() % SAMPLE_SUBNETS)

def random_expiry(now: Slot) -> Slot:
    return now + 1 + Slot(random_uint64() % SLOTS_PER_FAST_ROTATION_MAX)

def das_private_subnets_update(
    current_fast: Dict[VerticalIndex, FastInfo],
    current_slow: Dict[VerticalIndex, Subscription],
    slot: Slot):

    old: Dict[VerticalIndex, Subscription] = {}
    for subnet, info in current_fast.items():
        if info.expiry <= slot:
            old[subnet] = info.sub
            current_fast.remove(subnet)
    # Backfill FAST_INDICES
    while len(current_fast) < FAST_INDICES:
        while True:
            subnet = random_subnet()
            if subnet in current_fast or subnet in current_slow:  # already have it, try again (small chance)
                continue
            if subnet in old:  # got an old subnet again
                old.remove(subnet)
                current_fast[subnet] = FastInfo(sub=old[subnet], expiry=random_expiry(slot))
            else:              # new subnet
                sub = subcribe(subnet)
                current_fast[subnet] = FastInfo(sub=sub, expiry=random_expiry(slot))
            break
    # unsubscribe from remaining old topics
    for sub in old:
        sub.unsubscribe()
```

### Mapping points to samples

```python

def take_sample(extended_points: Sequence[RawPoint], sample_index: SampleIndex) -> Sample:
    start = sample_index * POINTS_PER_SAMPLE
    return Sample(extended_points[start:start+POINTS_PER_SAMPLE])
```
