# Eth2 Data Availability Sampling - Testground plan

This repository is a test plan to import into [Testground](https://github.com/testground/testground).
It tests the viability of various networking ideas for Phase 1 of Eth2.

## DAS experiment

Background:
- [Original write-up of Vitalik with different approaches](https://notes.ethereum.org/@vbuterin/r1v8VCULP)
- [Expansion on approach 2 by protolambda, using gossip subnets](https://notes.ethereum.org/McLGvrWgSX6Cpg60ewMYeQ)

The idea is to implement a proof of concept in Go, and run it on Testground with different parameters.
**Work in progress, highly experimental**

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

### Custom types

| Name | SSZ equivalent | Description |
| - | - | - |
| `VerticalIndex` | `uint64` | An index identifying a DAS subnet |
| `HorizontalIndex` | `uint64` | An index identifying a horizontal subnet, this could be a shard subnet. |
| `PointIndex` | `uint64` | An index of a point, w.r.t. the extended points of a shard data blob |
| `RawPoint` | `Vector[byte, POINT_SIZE]` | A raw point, serialized form |
| `Point` | `uint256` | A point, `F_r` element |
| `SampleIndex` | `uint64` | An index of a sample, w.r.t. the sample-ified extended points of a shard data blob | 
| `Sample` | `Vector[Point, POINTS_PER_SAMPLE]` | A sequence of points, sample unit |

### Constants / configurables

Don't take values as canonical. The point is to find good values.

#### Configurables

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `FAST_INDICES` | `16` | indices | Subset of indices that nodes sample, privately chosen, and replaced quickly |
| `SLOW_INDICES` | `4` | indices | Subset of indices that nodes sample, publicly determined, and replaced slowly |
| `MAX_SAMPLES_PER_SHARD_BLOCK` | `16` | samples | Maximum number of samples in which the extended points are split into |
| `POINTS_PER_SAMPLE` | `16` | points | Number of points per sample |
| `POINT_SIZE` | `31` | bytes | Number of bytes per point |
| `MAX_SHARD_BLOCK_SIZE` | `524288` | bytes | Number of bytes in a shard block |
| `SHARD_HEADER_SIZE` | `256` | bytes | Number of bytes in a shard header |
| `SLOTS_PER_FAST_ROTATION_MAX` | `32` | slots | Maximum of how frequently a fast vertical subnet subscriptions is randomly swapped. Rotations of a subnet can happen any time between 1 and `SLOTS_PER_FAST_ROTATION_MAX` (incl) slots. |
| `SLOTS_PER_SLOW_ROTATION` | `2048` | slots | How frequently a slow vertical subnet subscriptions is randomly swapped. (deterministic on peer ID, so public and predictable) |
| `SLOT_OFFSET_PER_SLOW_INDEX` | `512` | slots | The time for a slow vertical subscription to wait for the previous index to rotate, for stagger effect. |
| `EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION` | `256` | epochs | how frequently shard subnet subscriptions are kept |
| `TARGET_PEERS_PER_DAS_SUB` | `6` | peers | How many peers should be acquired per topic. If insufficient, search for more with discovery. |

Note that the total points per block is a power of 2 for various proof and sampling purposes.
However the points themselves cannot efficiently be a power of 2 in byte-length.

If a block is not full, the padding to the next power of 2 of points may be serialized in a more efficient way to avoid unnecessary network overhead. 

#### Generic / testing configurables

Not part of the DAS spec, but for testing purposes:

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `PEER_COUNT_LO` | `120` | peers | How many peers for low-water |
| `PEER_COUNT_HI` | `200` | peers | How many peers to maintain for hi-water |
| `SHARD_COUNT` | `64` | shards | Number of shards |
| `SECONDS_PER_SLOT` | `12` | seconds | Number of seconds in each slot |
| `VALIDATOR_COUNT` | `150000` | validators | Number of active validators |
| `NODE_COUNT` | `10000` | nodes | Physical validator nodes in the p2p network |

#### Derived configurable value

Useful aliases, derived from the other config values.

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `SAMPLE_SUBNETS` | `MAX_SAMPLES_PER_SHARD_BLOCK * SHARD_COUNT` | subnets | Number of subnets to propagate total samples to | 
| `AVERAGE_VALIDATORS_PER_NODE` | `VALIDATOR_COUNT / NODE_COUNT` | validators | validators per node | 
| `MAX_DATA_SIZE` | `POINT_SIZE * POINTS_PER_SAMPLE * MAX_SAMPLES_PER_SHARD_BLOCK / 2` | bytes | max bytes per (unextended) block data blob |

### Subnet sampling

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

#### Waiting on missing samples

Wait for the sample to re-broadcast. Someone may be slow with publishing, or someone else is able to do the work.

Any node can do extra work to keep the network healthy:
- Listen on a horizontal subnet, chunkify the block data in samples, and propagate the samples to vertical subnets.
- Listen on enough vertical subnets, reconstruct the missing samples by recovery, and propagate the recovered samples.

This is not a requirement, but can improve the network stability with little resources, and without any central party.

#### Pulling missing samples

The more realistic option, to execute when a sample is missing, is to query any node that is known to hold it.
Since *consensus identity is disconnected from network identity*, there is no direct way to contact custody holders 
without explicitly asking for the data.

However, *network identities* are still used to build a backbone for each vertical subnet.
These nodes should have received the samples, and can serve a buffer of them on demand.
Although serving these is not directly incentivised, it is little work:
1. Buffer any message you see on the `SLOW_INDICES` vertical subnets, for a buffer of approximately two weeks.
2. Serve the samples on request. An individual sample is just `~ 0.5 KB`, and does not require any pre-processing to serve.

Pulling samples directly from nodes with a custody responsibility, without revealing their identity to the network, is an open problem. 


#### Public/slow DAS subnet sampling

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

#### Private/quick DAS subnet sampling

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

### Shard data and DAS samples

The shard block data is partitioned into a sequence of points for the DAS sampling process.
The length in points is padded to a power of two, for extension and proof purporses.
 
These points are then partitioned into samples of `POINTS_PER_SAMPLE` points.

There are several interpretation requirements to these raw bytes:
- Bytes as `F_r` points
- Data extension:
  - Requirements for FFT recovery 
  - Points organized in even indices (more structured w.r.t. domain)
- Does not exceed the number of points


#### Bytes as `F_r` points

The Kate commitments, proofs and data recovery all rely on the data be interpreted as `F_r` points.
I.e. the raw data is partitioned in values that fit in a modulo `r` (BLS curve order) field.
The modulo is just shy of spanning 256 bits, thus partitioning the bytes in 32 byte samples will not work.
There is an option to partition data in 254 bits to use as many bits as possible.
For practical purposes the closest byte boundary is chosen instead, thus `POINTS_PER_SAMPLE = 31 bytes`.

```python
padded_bytes = input_bytes + "\x00" * (POINT_SIZE*POINTS_PER_SAMPLE - (len(input_bytes) % POINT_SIZE))

def deserialize_point(p: RawPoint) -> Point:
    return Point(uint256.from_bytes(p))

def point(i: PointIndex) -> Point:
    return deserialize_point(RawPoint(padded_bytes[i*31:(i+1)*31] + "\x00"))

def sample(sample_index: SampleIndex) -> Sequence[Point]:
    # TODO: replace this with bit-reverse order, so that points are grouped more efficiently for sample proofs
    start = sample_index * POINTS_PER_SAMPLE
    return [point(i) for i in range(start, start+POINTS_PER_SAMPLE)]

input_point_count = next_power_of_2(len(padded_bytes) // POINT_SIZE)
extended_point_count = input_point_count * 2
sample_count = extended_point_count // POINTS_PER_SAMPLE
```

#### Data extension

The redundant extension doubles the count of the values from `N` to `2N` in such a way that any subset of `N` points will be enough to recover all of it.
By sampling the data in an extended form, instead of just the original data, not publishing a piece of data without detection becomes intractable.
Honest validators can trust that data has been published by making `k` random sampled queries,
 and quickly adjust their forkchoice in case of unavailable data. 

The data that is extended in such a way that it enables a
[FFT-based recovery method](https://ethresear.ch/t/reed-solomon-erasure-code-recovery-in-n-log-2-n-time-with-ffts/3039).

This method works as follows:

```python
width = 2 * N  # number of points in extended values
root_of_unity = ... # chosen based on width
domain = expand_root_of_unity(root_of_unity)  # generate remaining roots of unity (width total, r^0, r^1, ... r^2n), spanning the given width. Precomputed.
modulus = 52435875175126190479447740508185965837690552500527637822603658699938581184513   # BLS curve order
input_values = ...  #  width//2 points
coeffs = inverse_fft(input_values, modulus, domain[::2])  # use the contained domain spanning the even values
# The second half of the coefficients is required to be zero,
# such that the original values are forced into the even indices, and odd indices are used for error correction.
padded_coeffs = coeffs + [0] * N
# Now with the full domain, derive the original inputs
extended_values = fft(padded_coeffs, modulus, domain)
```

Any random subset of `N` points of the `extended_values` can then be recovered into the full `extended_values`.
And then the even indices matches the original, i.e. `input_values == extended_values[::2]`

Extension using two FFTs is relatively inefficient, and can be reduced to a modified FFT.
The expected even-index values are set to the original values, and the expected right half of the input values,
 the coefficients, is implicitly set to zeroes.

This can be implemented as:
```python
# "a" are the even-indexed expected outputs
# The returned list is "b", the derived odd-index outputs.
def das_fft(a: list, modulus: int, domain: list, inverse_domain: list, inv2: int) -> list:
    if len(a) == 2:
        a_half0 = a[0]
        a_half1 = a[1]
        x = (((a_half0 + a_half1) % modulus) * inv2) % modulus
        # y = (((a_half0 - x) % modulus) * inverse_domain[0]) % modulus     # inverse_domain[0] will always be 1
        y = (a_half0 - x) % modulus

        y_times_root = y * domain[1]
        return [
            (x + y_times_root) % modulus,
            (x - y_times_root) % modulus
        ]

    if len(a) == 1:  # for illustration purposes, neat simplification. Inputs are always a power of two, dead code.
        return a

    half = len(a)  # a is already half, compared with a regular FFT
    halfhalf = half // 2

    L0 = [0] * halfhalf        # P
    R0 = [0] * halfhalf
    for i, (a_half0, a_half1) in enumerate(zip(a[:halfhalf], a[halfhalf:])):
        L0[i] = (((a_half0 + a_half1) % modulus) * inv2) % modulus
        R0[i] = (((a_half0 - L0[i]) % modulus) * inverse_domain[i * 2]) % modulus

    L1 = das_fft(L0, modulus, domain[::2], inverse_domain[::2], inv2)
    R1 = das_fft(R0, modulus, domain[::2], inverse_domain[::2], inv2)

    b = [0] * half
    for i, (x, y) in enumerate(zip(L1, R1)):    # Half the work of a regular FFT: only deal with uneven-index outputs
        y_times_root = y * domain[1 + i * 2]
        b[i] = (x + y_times_root) % modulus
        b[halfhalf + i] = (x - y_times_root) % modulus

    return b
```

And then the extension process can be simplified to:
```python
inverse_domain = [modular_inverse(d, MODULUS) for d in domain]  # Precomputed.
odd_values = das_fft(input_values, MODULUS, domain, inverse_domain, inverse_of_2)
extended_values = [input_values[i // 2] if i % 2 == 0 else odd_values[i // 2] for i in range(width)]
```

#### Data recovery

TODO.

See:
- Original write up: https://ethresear.ch/t/reed-solomon-erasure-code-recovery-in-n-log-2-n-time-with-ffts/3039
- Original code: https://github.com/ethereum/research/blob/master/mimc_stark/recovery.py
- Proof of concept of recovering extended data: https://github.com/protolambda/partial_fft/blob/master/recovery_poc.py
- Work in progress Go implementation: https://github.com/protolambda/go-verkle/

### Mapping samples to DAS subnets

TODO. `hash(sample_index, shard, slot) % subnet_count = subnet index`

Randomized to spread load better. Skewed load would be limited in case of attack (manipulating sample count), but inconvenient.
Sample counts are also only powers of 2, so there is little room for manipulation there.

Shared subnets + high sample count = all subnets used, empty subnets should not be a problem.


### DAS Validator duties

TODO


## License

MIT, see [`LICENSE`](./LICENSE) file.
