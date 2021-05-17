package main

import (
	"crypto/rand"
	"math/big"

	"github.com/ldsec/lattigo/v2/ring"
)

func sampleUniformVector(n, q uint64) (v []uint64) {
	v = make([]uint64, n)
	//mask := uint64((1 << uint64(bits.Len64(q))) - 1)
	for i := uint64(0); i < n; i++ {
		x, err := rand.Int(rand.Reader, big.NewInt(int64(q)))
		v[i] = x.Uint64()
		if err != nil {
			panic(err)
		}
	}
	return
}

func negVec(a []uint64, q uint64) (v []uint64) {
	v = make([]uint64, len(a))
	for i := range a {
		v[i] = q - a[i]
	}
	return
}

func addVec(a, b []uint64, q uint64) (v []uint64) {
	v = make([]uint64, len(a))
	for i := range a {
		v[i] = ring.CRed(a[i]+b[i], q)
	}
	return
}

func subVec(a, b []uint64, q uint64) (v []uint64) {
	v = make([]uint64, len(a))
	for i := range a {
		v[i] = ring.CRed(a[i]+(q-b[i]), q)
	}
	return
}

func mulVec(a, b []uint64, q uint64) (v []uint64) {
	v = make([]uint64, len(a))
	bredParams := ring.BRedParams(q)
	for i := range a {
		v[i] = ring.BRed(a[i], b[i], q, bredParams)
	}
	return
}
