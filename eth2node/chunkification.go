package eth2node

import verkle "github.com/protolambda/go-verkle"

func Chunkify(data ShardBlockData) []ShardBlockDataChunk {
	// TODO: convert data to big numbers
	var d []verkle.Big

	// TODO pick scale based on block size
	fs := verkle.NewFFTSettings(10)
	coeffs, err := fs.FFT(d, true)
	// TODO: sanity check evaluations for [0, N) evaluate to the expected data
	// TODO: evaluate [N, 2N) to get more error correction data
	// TOOD: chunkify the result

	return nil
}
