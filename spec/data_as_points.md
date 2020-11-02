# Data as points

This document describes the core lower-level details of how to represent shard data as points.

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
| `PointIndex` | `uint64` | An index of a point, w.r.t. the extended points of a shard data blob |
| `RawPoint` | `Vector[byte, POINT_SIZE]` | A raw point, serialized form |
| `Point` | `uint256` | A point, `F_r` element |

### Constants / configurables

#### Constants

These values are not intended to change.

| `POINT_SIZE` | `31` | bytes | Number of bytes per point |


#### Configurables

Don't take values as canonical. The point is to find good values.

No configurables here.


## Shard data and DAS

The shard block data is partitioned into a sequence of points for sampling part of the DAS process.
The length in points is padded to a power of two, for extension and proof purposes.
 
These points are then partitioned into samples of `POINTS_PER_SAMPLE` points.

There are several interpretation requirements to these raw bytes:
- Bytes as `F_r` points
- Data extension:
  - Requirements for FFT recovery 
  - Points organized in even indices (more structured w.r.t. domain)
- Does not exceed the number of points


## Bytes as `F_r` points

The Kate commitments, proofs and data recovery all rely on the data be interpreted as `F_r` points.
I.e. the raw data is partitioned in values that fit in a modulo `r` (BLS curve order) field.
The modulo is just shy of spanning 256 bits, thus partitioning the bytes in 32 byte samples will not work.
There is an option to partition data in 254 bits to use as many bits as possible.
For practical purposes the closest byte boundary is chosen instead, thus `POINT_SIZE = 31 bytes`.

```python
def deserialize_point(p: RawPoint) -> Point:
    return Point(uint256.from_bytes(p, byteorder='little'))

def raw_point(input_bytes: bytes, i: PointIndex) -> RawPoint:
    return RawPoint(input_bytes[i*31:(i+1)*31] + "\x00")
```

## Data extension

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
inv2 = modular_inverse(2, modulus)

# "a" are the even-indexed expected outputs
# The returned list is "b", the derived odd-index outputs.
def das_extension(a: list, modulus: int, domain: list, inverse_domain: list) -> list:
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

    L1 = das_extension(L0, modulus, domain[::2], inverse_domain[::2])
    R1 = das_extension(R0, modulus, domain[::2], inverse_domain[::2])

    b = [0] * half
    for i, (x, y) in enumerate(zip(L1, R1)):    # Half the work of a regular FFT: only deal with uneven-index outputs
        y_times_root = y * domain[1 + i * 2]
        b[i] = (x + y_times_root) % modulus
        b[halfhalf + i] = (x - y_times_root) % modulus

    return b
```

And then the extension process can be simplified to:
```python
inverse_domain = [modular_inverse(d, MODULUS) for d in domain]  # Or simply reverse the domain (except first 1)
odd_values = das_extension(input_values, MODULUS, domain, inverse_domain)
extended_values = [input_values[i // 2] if i % 2 == 0 else odd_values[i // 2] for i in range(width)]
```

## Data recovery

TODO.

See:
- Original write up: https://ethresear.ch/t/reed-solomon-erasure-code-recovery-in-n-log-2-n-time-with-ffts/3039
- Original code: https://github.com/ethereum/research/blob/master/mimc_stark/recovery.py
- Proof of concept of recovering extended data: https://github.com/protolambda/partial_fft/blob/master/recovery_poc.py
- Work in progress Go implementation: https://github.com/protolambda/go-verkle/
