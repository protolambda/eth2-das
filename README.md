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
  - `CHUNK_INDEX_SUBNETS` DAS subnets
  - `SHARD_COUNT` Shard subnets
  - Shard headers global topic
- Subscribe to `P` DAS subnets randomly based on publicly known seed, unique to peer.
  - For stability
  - Find other peers via discovery (TODO add discovery)
- Subscribe to `K` DAS subnets randomly 
  - The random sampling necessary to make DAS work secure enough
  - Verify if all the seen chunks are correct
    - TODO: keep track of what has been seen, *and what has not*
      - Adjust forkchoice to steer clear of unavailable shard data
- Subscribe randomly to a shard block subnet for every Validator that runs
  - Rotate out on random `[EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION, 2 * EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION]` interval (per shard)
  - For any seen block, split it up in chunks, and for each DAS subnet we are on, propagate the relevant chunks to it.
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
| `DASSubnetIndex` | `uint64` | An index identifying a DAS subnet |
| `PointIndex` | `uint64` | An index of a point, w.r.t. the extended points of a shard data blob |
| `RawPoint` | `Vector[byte, POINT_SIZE]` | A raw point, serialized form |
| `Point` | `uint256` | A point, `F_r` element |
| `ChunkIndex` | `uint64` | An index of a chunk, w.r.t. the chunkified extended points of a shard data blob | 
| `Chunk` | `Vector[Point, POINTS_PER_CHUNK]` | A sequence of points, smallest sample unit |

### Constants / configurables

Don't take values as canonical. The point is to find good values.

#### Configurables

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `K` | `16` | indices | Subset of indices that nodes sample, privately  |
| `P` | `4` | indices | Subset of indices that nodes sample, publicly |
| `MAX_CHUNKS_PER_BLOCK` | `16` | chunks | Maximum number of chunks in which the extended points are split into |
| `POINTS_PER_CHUNK` | `16` | points | Number of points per chunk |
| `POINT_SIZE` | `31` | bytes | Number of bytes per point |
| `MAX_BLOCK_SIZE` | `524288` | bytes | Number of bytes in a block |
| `SHARD_HEADER_SIZE` | `256` | bytes | Number of bytes in a shard header | 
| `SHARD_COUNT` | `64` | shards | Number of shards |
| `SECONDS_PER_SLOT` | `12` | seconds | Number of seconds in each slot |

Note that the total points per block is a power of 2 for various proof and sampling purposes.
However the points themselves cannot efficiently be a power of 2 in byte-length.

If a block is not full, the padding to the next power of 2 of points may be serialized in a more efficient way to avoid unnecessary network overhead. 

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `SLOTS_PER_K_ROTATION_MAX` | `32` | slots | Maximum of how frequently one of the K dasSubnets are randomly swapped. Rotations of a subnet in K can happen any time between 1 and SLOTS_PER_K_ROTATION_MAX (incl) slots. |
| `SLOTS_PER_P_ROTATION` | `320` | slots | How frequently each of the P dasSubnets are randomly swapped. (deterministic on peer ID, so public and predictable) |
| `SLOT_OFFSET_PER_P_INDEX` | `16` | slots | SLOT_OFFSET_PER_P_INDEX is the time for an index of P to wait for the previous index to rotate. |
| `EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION` | `256` | epochs | how frequently shard subnet subscriptions are kept |
| `TARGET_PEERS_PER_DAS_SUB` | `6` | peers | How many peers should be acquired per topic. If insufficient, search for more with discovery. |
| `PEER_COUNT_LO` | `120` | peers | How many peers for low-water |
| `PEER_COUNT_HI` | `200` | peers | How many peers to maintain for hi-water |


#### Estimates for emulation purposes

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `VALIDATOR_COUNT` | `150000` | validators | Number of active validators |
| `NODE_COUNT` | `10000` | nodes | Physical validator nodes in the p2p network |


#### Derived configurable value

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `CHUNK_INDEX_SUBNETS` | `MAX_BLOCK_SIZE / CHUNK_SIZE = 1024` | subnets | Number of chunks that make up a block | 
| `AVERAGE_VALIDATORS_PER_NODE` | `VALIDATOR_COUNT / NODE_COUNT` | validators | validators per node | 
| `MAX_DATA_SIZE` | `POINT_SIZE * POINTS_PER_CHUNK * MAX_CHUNKS_PER_BLOCK / 2` | bytes | max bytes per block data blob |

### Public DAS subnet sampling, a.k.a. `P`

Public indices are sampled as:
```python
def das_public_peer_seed(peer_id: PeerID) -> Bytes32:
  return H(PUBLIC_DAS_SUBNETS_DOMAIN ++ peer_id)

def das_public_peer_slot_offset(peer_seed) -> Slot:
  return Slot(peer_seed[:8] % SLOTS_PER_P_ROTATION)

def das_public_subnet_slot_offset(i: DASSubnetIndex) -> Slot:
  return Slot(i * SLOT_OFFSET_PER_P_INDEX) % SLOTS_PER_P_ROTATION

def das_public_subnet_index(peer_seed: Bytes32, slot: Slot, i: uint64) -> DASSubnetIndex:
  window_index = slot ~/ SLOTS_PER_P_ROTATION
  return H(peer_seed ++ i ++ window_index)[:8] % CHUNK_INDEX_SUBNETS

def das_public_subnet_indices(peer_id: PeerID, slot: Slot, das_p: uint64) -> Set[DASSubnetIndex]:
  peer_seed = das_public_peer_seed(peer_id)
  peer_offset = das_public_peer_slot_offset(peer_seed)
  return set(
 		das_public_subnet_index(
				peer_seed,
				slot + peer_offset + das_public_subnet_slot_offset(i),
				i,
			) for i in range(das_p))

p_indices = das_public_subnet_indices(peer_id, slot, P)
```

Any overlap in `p_indices` is rare, and acceptable. The set will be a little smaller sometimes.

The idea of making `P` slow, public and predictable is:
- As backbone, for subnet stability
- Easy to discover peers through
- By using a seed based on peer ID, no data has to be tracked on discovery. No blow up in ENR size.
- Predictable enough to prepare subnet peering in advance
- Small value, other sampling can be unpredictable and rotate quicker.

### Private DAS subnet sampling, a.k.a. `K`

For the random sampling, `K` subnets are joined randomly,
selected out of the total `CHUNK_INDEX_SUBNETS`, except for what is already joined publicly (`P`).

Then on random intervals, each of the `K` subnet subscriptions is swapped for another subnet.

This effectively creates a PUSH model where validators 
register themselves on short notice to be sampling a random subset.
The shard blocks are split up in chunks, and should reach the validators with relatively low latency.

**Note: depending on test results, a larger K may be set, but for any topics that are subscribed to in e.g. 
the last few slots won't count as heavy towards bad data availability,
as it may just be an issue to subscribe to the topic fast enough.**


```python
class KInfo:
    sub: Subscription
    expiry: Slot

def random_subnet() -> DASSubnetIndex:
    return DASSubnetIndex(random_uint64() % CHUNK_INDEX_SUBNETS)

def random_expiry(now: Slot) -> Slot:
    return now + 1 + Slot(random_uint64() % SLOTS_PER_K_ROTATION_MAX)

def das_private_subnets_update(
    current_k: Dict[DASSubnetIndex, KInfo],
    current_p: Dict[DASSubnetIndex, Subscription],
    slot: Slot):

    old: Dict[DASSubnetIndex, Subscription] = {}
    for subnet, info in current_k.items():
        if info.expiry <= slot:
            old[subnet] = info.sub
            current_k.remove(subnet)
    # By trial and error, backfill k
    while len(current_k) < K:
        while True:
            subnet = random_subnet()
            if subnet in current_k:
                continue
            if subnet in current_p:
                continue
            if subnet in old:
                old.remove(subnet)
                current_k[subnet] = KInfo(sub=old[subnet], expiry=random_expiry(slot))
            else:
                sub = subcribe(subnet)
                current_k[subnet] = KInfo(sub=sub, expiry=random_expiry(slot))
            break
    # unsubscribe from remaining old topics
    for sub in old:
        sub.unsubscribe()
```

### DAS subnet discovery

The node uses a simple discovery interface:
```python
class Discovery(Protocol):
	def find_public(conf: Config, slot: Slot, subnets: Set[DASSubnetIndex]) -> Dict[DASSubnetIndex, Set[PeerID]]: ...
	def addrs(id: PeerID) -> List[Multiaddr]: ...

```

It lists possible candidates for each subnet, based on public subnet info known of the peers: their `P` subset.

In preparation of rotation of subnets, the Eth2 node can then try to peer with peers that complement their current peers,
 to allow gossipsub to build a mesh for the topic.

### Shard data and DAS chunks

The shard block data is partitioned into a sequence of points for the DAS sampling process.
The length in points is padded to a power of two, for extension and proof purporses.
 
These points are then partitioned into chunks of `POINTS_PER_CHUNK` points.

There are several interpretation requirements to these raw bytes:
- Bytes as `F_r` points
- Data extension:
  - Requirements for FFT recovery 
  - Points organized in even indices (more structured w.r.t. domain)
- Does not exceed the number of points


#### Bytes as `F_r` points

The Kate commitments, proofs and data recovery all rely on the data be interpreted as `F_r` points.
I.e. the raw data is partitioned in values that fit in a modulo `r` (BLS curve order) field.
The modulo is just shy of spanning 256 bits, thus partitioning the bytes in 32 byte chunks will not work.
There is an option to partition data in 254 bits to use as many bits as possible.
For practical purposes the closest byte boundary is chosen instead, thus `POINTS_PER_CHUNK = 31 bytes`.

```python
padded_bytes = input_bytes + "\x00" * (POINT_SIZE*POINTS_PER_CHUNK - (len(input_bytes) % POINT_SIZE))

def deserialize_point(p: RawPoint) -> Point:
    return Point(uint256.from_bytes(p))

def point(i: PointIndex) -> Point:
    return deserialize_point(RawPoint(padded_bytes[i*31:(i+1)*31] + "\x00"))

def chunk(chunk_index: ChunkIndex) -> Sequence[Point]:
    # TODO: replace this with bit-reverse order, so that points are grouped more efficiently for chunk proofs
    start = chunk_index * POINTS_PER_CHUNK
    return [point(i) for i in range(start, start+POINTS_PER_CHUNK)]

input_point_count = next_power_of_2(len(padded_bytes) // POINT_SIZE)
extended_point_count = input_point_count * 2
chunk_count = extended_point_count // POINTS_PER_CHUNK
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

### Mapping chunks to DAS subnets

TODO. `hash(chunk_index, shard, slot) % subnet_count = subnet index`

Randomized to spread load better. Skewed load would be limited in case of attack (manipulating chunk count), but inconvenient.
Chunk counts are also only powers of 2, so there is little room for manipulation there.

Shared subnets + high chunk count = all subnets used, empty subnets should not be a problem.


### DAS Validator duties

TODO


## License

MIT, see [`LICENSE`](./LICENSE) file.
