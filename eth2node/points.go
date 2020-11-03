package eth2node

import (
	"fmt"
	verkle "github.com/protolambda/go-verkle"
	"github.com/protolambda/ztyp/tree"
)

func (c *ExpandedConfig) MakeSamples(data ShardBlockData) ([]ShardBlockDataChunk, error) {
	dataPoints, err := c.shardDataToPoints(data)
	if err != nil {
		return nil, err
	}
	sampleCount := uint64(len(dataPoints)) / c.POINTS_PER_SAMPLE
	if sampleCount*c.POINTS_PER_SAMPLE != uint64(len(dataPoints)) {
		return nil, fmt.Errorf("bad data-points count %d, expected it to be divisible by sample chunk size: %d", len(dataPoints), c.POINTS_PER_SAMPLE)
	}
	out := make([]ShardBlockDataChunk, sampleCount, sampleCount)
	for i := uint64(0); i < sampleCount; i++ {
		start := i * c.POINTS_PER_SAMPLE
		sample := out[i]
		for j := uint64(0); j < c.POINTS_PER_SAMPLE; j++ {
			p := &dataPoints[start+j]
			raw := verkle.BigNumTo31(p)
			copy(sample[j*POINT_SIZE:(j+1)*POINT_SIZE], raw[:])
		}
	}
	return out, nil
}

func (c *ExpandedConfig) shardDataToPoints(input []byte) ([]Point, error) {
	l := uint64(len(input))
	if l > c.MAX_DATA_SIZE {
		return nil, fmt.Errorf("data is too large: %d bytes, expected no more than %d", len(input), c.MAX_DATA_SIZE)
	}
	// round up
	inputPoints := (l + POINT_SIZE - 1) / POINT_SIZE

	// Get depth of next power of 2 (if not already)
	// Example (in, out):
	// (0 0), (1 0), (2 1), (3 2), (4 2), (5 3), (6 3), (7 3), (8 3), (9 4)
	inputDepth := tree.CoverDepth(inputPoints)
	inputPointsPaddedLen := uint64(1) << inputDepth

	changedOrder := reverseBitOrder(inputPointsPaddedLen)
	points := make([]Point, inputPointsPaddedLen, inputPointsPaddedLen)
	for i := uint64(0); i < inputPoints; i++ {
		var tmp [31]byte
		copy(tmp[:], input[i*POINT_SIZE:])                 // copy the next 31 bytes (or less if clipped end)
		verkle.BigNumFrom31(&points[changedOrder[i]], tmp) // directly put it into its reverse-bitorder place
	}
	// pad the input with zeros
	for i := inputPoints; i < inputPointsPaddedLen; i++ {
		verkle.CopyBigNum(&points[changedOrder[i]], &verkle.ZERO)
	}

	// Now make a copy
	extension := make([]Point, inputPointsPaddedLen, inputPointsPaddedLen)
	for i := uint64(0); i < inputPointsPaddedLen; i++ {
		verkle.CopyBigNum(&extension[i], &points[i])
	}
	extendedDepth := inputDepth + 1
	// And run the extension process (modifies points in-place, hence above copy)
	fs := verkle.NewFFTSettings(extendedDepth)
	fs.DASFFTExtension(points[inputPointsPaddedLen:])

	// And then put the inputs in even positions, and extension in odd positions
	extendedLength := uint64(1) << extendedDepth
	extended := make([]Point, extendedLength, extendedLength)
	for i := uint64(0); i < extendedLength; i += 2 {
		verkle.CopyBigNum(&extended[i], &points[i>>1])
		verkle.CopyBigNum(&extended[i+1], &extension[i>>1])
	}

	return extended, nil
}

func reverseBitOrder(width uint64) []uint64 {
	order := make([]uint64, width, width)
	for i := uint64(0); i < width; i++ {
		order[i] = i
	}
	fillReverseBitOrder(order)
	return order
}

func fillReverseBitOrder(out []uint64) {
	if len(out) == 0 {
		return
	}
	if len(out) == 1 {
		out[0] = 0
	}
	half := len(out) >> 1
	fillReverseBitOrder(out[:half])
	// double the numbers in the first half
	for i := 0; i < half; i++ {
		out[i] = out[i] << 1
	}
	// then the other half is the same, but plus 1
	for i := 0; i < half; i++ {
		out[half+i] = out[i] + 1
	}
}
