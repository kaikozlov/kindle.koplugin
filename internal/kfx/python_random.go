package kfx

import (
	cryptorand "crypto/rand"
	"encoding/binary"
)

// pythonRandom is the MT19937 generator used by CPython's _random.Random.
// KFX Input uses random.Random for Scribe density-stroke feathering
// (yj_to_epub_notebook.py:484-511). Go's math/rand uses a different generator,
// so identical nmdl.random_seed values otherwise produce different pixels.
type pythonRandom struct {
	state [624]uint32
	index int
}

// newPythonRandomSeed reproduces CPython's integer-seed path: split the
// non-negative integer into little-endian 32-bit words and feed init_by_array.
// Scribe nmdl.random_seed values are Ion integers.
func newPythonRandomSeed(seed uint64) *pythonRandom {
	key := []uint32{uint32(seed)}
	if seed>>32 != 0 {
		key = append(key, uint32(seed>>32))
	}
	r := &pythonRandom{}
	r.initByArray(key)
	return r
}

func pythonRandomFromIonSeed(seed interface{}) *pythonRandom {
	if seed == nil {
		return newPythonRandomEntropy()
	}
	var n uint64
	switch v := seed.(type) {
	case int:
		if v < 0 {
			n = uint64(-(v + 1)) + 1
		} else {
			n = uint64(v)
		}
	case int32:
		vv := int64(v)
		if vv < 0 {
			n = uint64(-(vv + 1)) + 1
		} else {
			n = uint64(vv)
		}
	case int64:
		if v < 0 {
			n = uint64(-(v + 1)) + 1
		} else {
			n = uint64(v)
		}
	case uint32:
		n = uint64(v)
	case uint64:
		n = v
	case float64:
		// KFX Scribe random seeds are Ion integers. Preserve the old Go
		// fallback for a malformed/nonnative numeric seed; non-negative
		// integral floats happen to share Python's integer seed sequence.
		if v < 0 {
			n = uint64(-v)
		} else {
			n = uint64(v)
		}
	default:
		return newPythonRandomEntropy()
	}
	return newPythonRandomSeed(n)
}

// newPythonRandomEntropy mirrors Python random.Random()'s nondeterministic
// construction closely enough for the seed=None case. Exact byte parity is
// impossible when upstream intentionally obtains fresh OS entropy, but both
// sides use MT19937 initialized from OS-random 32-bit words.
func newPythonRandomEntropy() *pythonRandom {
	key := make([]uint32, 624)
	buf := make([]byte, len(key)*4)
	if _, err := cryptorand.Read(buf); err == nil {
		for i := range key {
			key[i] = binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
		}
	} else {
		// crypto/rand failure is exceptionally unlikely. Keep a valid generator
		// rather than failing conversion; CPython also has a fallback seeder.
		key[0] = 5489
	}
	r := &pythonRandom{}
	r.initByArray(key)
	return r
}

func (r *pythonRandom) initGenRand(seed uint32) {
	r.state[0] = seed
	for i := 1; i < len(r.state); i++ {
		prev := r.state[i-1]
		r.state[i] = 1812433253*(prev^(prev>>30)) + uint32(i)
	}
	r.index = len(r.state)
}

// initByArray is the reference MT19937 init_by_array routine used by CPython.
func (r *pythonRandom) initByArray(key []uint32) {
	if len(key) == 0 {
		key = []uint32{0}
	}
	r.initGenRand(19650218)
	i, j := 1, 0
	k := len(r.state)
	if len(key) > k {
		k = len(key)
	}
	for ; k > 0; k-- {
		prev := r.state[i-1]
		r.state[i] = (r.state[i] ^ ((prev ^ (prev >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= len(r.state) {
			r.state[0] = r.state[len(r.state)-1]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k = len(r.state) - 1; k > 0; k-- {
		prev := r.state[i-1]
		r.state[i] = (r.state[i] ^ ((prev ^ (prev >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= len(r.state) {
			r.state[0] = r.state[len(r.state)-1]
			i = 1
		}
	}
	r.state[0] = 0x80000000
	r.index = len(r.state)
}

func (r *pythonRandom) uint32() uint32 {
	const (
		matrixA   = uint32(0x9908b0df)
		upperMask = uint32(0x80000000)
		lowerMask = uint32(0x7fffffff)
	)
	if r.index >= len(r.state) {
		for kk := 0; kk < 624-397; kk++ {
			y := (r.state[kk] & upperMask) | (r.state[kk+1] & lowerMask)
			r.state[kk] = r.state[kk+397] ^ (y >> 1)
			if y&1 != 0 {
				r.state[kk] ^= matrixA
			}
		}
		for kk := 624 - 397; kk < 623; kk++ {
			y := (r.state[kk] & upperMask) | (r.state[kk+1] & lowerMask)
			r.state[kk] = r.state[kk+(397-624)] ^ (y >> 1)
			if y&1 != 0 {
				r.state[kk] ^= matrixA
			}
		}
		y := (r.state[623] & upperMask) | (r.state[0] & lowerMask)
		r.state[623] = r.state[396] ^ (y >> 1)
		if y&1 != 0 {
			r.state[623] ^= matrixA
		}
		r.index = 0
	}

	y := r.state[r.index]
	r.index++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

// Float64 reproduces CPython random_random(): 53 random bits assembled from
// two MT outputs (27 high bits + 26 high bits).
func (r *pythonRandom) Float64() float64 {
	a := r.uint32() >> 5
	b := r.uint32() >> 6
	return (float64(a)*67108864.0 + float64(b)) * (1.0 / 9007199254740992.0)
}
