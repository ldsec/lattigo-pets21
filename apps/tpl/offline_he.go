package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/ldsec/lattigo/v2/bfv"
	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/ldsec/lattigo/v2/utils"
)

type TripleGenMessage struct {
	PartyID
	bfv.Ciphertext
	Query bool
}

func (m *TripleGenMessage) String() string {
	ctBytes, _ := m.Ciphertext.MarshalBinary()
	mType := "response"
	if m.Query {
		mType = "query"
	}
	return fmt.Sprintf("{%s | %d, %v}", mType, m.PartyID, md5.Sum(ctBytes))
}

type TripleGenRound struct {
	a, b, c        []uint64
	plainA, plainB *bfv.Plaintext
	plainM         map[PartyID]*bfv.Plaintext
	encA, encAggr  *bfv.Ciphertext

	hasQueried   map[PartyID]struct{}
	hasResponded map[PartyID]struct{}
}

type TripleGenProtocol struct {
	*LocalParty
	bfv.Evaluator
	bfv.Encoder
	bfv.Encryptor
	bfv.Decryptor

	gaussianSampler *ring.GaussianSampler

	Triples chan Triple

	Chan  chan TripleGenMessage
	Peers map[PartyID]*TripleGenRemote

	rq     *ring.Ring
	n      uint64 // number of beaver triples per ciphertext
	q      uint64 // ring of the beaver triples
	params bfv.Parameters
}

type TripleGenRemote struct {
	*RemoteParty
	Chan chan TripleGenMessage
}

func (lp *LocalParty) NewTripleGenProtocol(params bfv.Parameters, sk *rlwe.SecretKey) *TripleGenProtocol {
	tgp := new(TripleGenProtocol)
	tgp.LocalParty = lp
	tgp.rq = params.RingQ()
	tgp.Evaluator = bfv.NewEvaluator(params, rlwe.EvaluationKey{})
	tgp.Encoder = bfv.NewEncoder(params)
	tgp.Encryptor = bfv.NewEncryptorFromSk(params, sk)
	tgp.Decryptor = bfv.NewDecryptor(params, sk)

	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}
	tgp.gaussianSampler = ring.NewGaussianSampler(prng, tgp.rq, 3.19, 19)

	tgp.params = params
	// Number of Beaver triplets elements (has to comply with the BFV parameters)
	tgp.n = params.N()
	// Beaver triplets moduli (has to comply with the BFV parameters)
	tgp.q = params.T()

	tgp.Chan = make(chan TripleGenMessage, 32)

	tgp.Peers = make(map[PartyID]*TripleGenRemote)
	for _, rp := range lp.Peers {
		tgp.Peers[rp.ID] = &TripleGenRemote{
			RemoteParty: rp,
			Chan:        make(chan TripleGenMessage, 32),
		}
	}

	tgp.Triples = make(chan Triple, tgp.n)

	return tgp
}

func (tgp *TripleGenProtocol) Run(nTriple uint64) {

	round := tgp.genInput()

	// Send input

	for _, rp := range tgp.Peers {
		if rp.ID != tgp.ID {
			m := TripleGenMessage{PartyID: tgp.ID, Ciphertext: *round.encA, Query: true}
			rp.Chan <- m
			//fmt.Println(tgp, "sent to", rp, &m,)
		}
	}

	// Listen for Messages
	for m := range tgp.Chan {
		//fmt.Println(tgp, "got from", m.PartyID , &m)
		if m.Query {
			response := tgp.processQuery(m.PartyID, &m.Ciphertext, round)
			tgp.Peers[m.PartyID].Chan <- TripleGenMessage{PartyID: tgp.ID, Ciphertext: *response, Query: false}
			continue
		}

		tgp.processResponse(m.PartyID, &m.Ciphertext, round)
		//fmt.Println(tgp, "got response from", m.PartyID)

		if tgp.IsComplete(round) {
			break
		}
	}

	fmt.Println("\tround 1 ok")

	needed := nTriple
	for _, t := range tgp.decryptTriples(round) {
		tgp.Triples <- t
		needed--
		if needed == 0 {
			break
		}
	}

	close(tgp.Triples)
}

func (tgp *TripleGenProtocol) IsComplete(round *TripleGenRound) bool {
	complete := true
	for peer := range tgp.Peers {
		_, hasQueried := round.hasQueried[peer]
		_, hasResponded := round.hasResponded[peer]
		complete = (complete && peer == tgp.ID) || (complete && hasQueried && hasResponded)
	}
	return complete
}

func (tgp *TripleGenProtocol) genInput() (round *TripleGenRound) {
	round = new(TripleGenRound)

	// Each party samples its [a] and [b] and computes c' = [a_self] * [b_self]
	round.a = sampleUniformVector(tgp.n, tgp.q)
	round.b = sampleUniformVector(tgp.n, tgp.q)
	round.c = mulVec(round.a, round.b, tgp.q)

	// Those [a_self] and [b_self] are encode to a BFV plaintext
	round.plainA = bfv.NewPlaintext(tgp.params)
	round.plainB = bfv.NewPlaintext(tgp.params)
	tgp.EncodeUint(round.a, round.plainA)
	tgp.EncodeUint(round.b, round.plainB)

	// Each party samples a uniform mask to be assigned to each other party to the protocol.
	m := make(map[PartyID][]uint64, len(tgp.Peers))
	round.plainM = make(map[PartyID]*bfv.Plaintext, len(tgp.Peers))

	for peerID := range tgp.Peers {

		// For each party that isn't itself
		if peerID != tgp.ID {

			// Sample the mask and assigns it to the ID of the other party
			m[peerID] = sampleUniformVector(tgp.n, tgp.q)

			// Compute c' = [a_self]*[b_self] - sum(m_i_self)
			round.c = subVec(round.c, m[peerID], tgp.q)

			// Encodes the m_i_self to a BFV plaintext
			round.plainM[peerID] = bfv.NewPlaintext(tgp.params)
			tgp.EncodeUint(m[peerID], round.plainM[peerID])
		}
	}

	// Each party encrypts [a_self] to a BFV ciphertext : enc([a_self]).
	round.encA = tgp.EncryptNew(round.plainA)

	round.encAggr = bfv.NewCiphertext(tgp.params, 1)

	round.hasQueried = make(map[PartyID]struct{}, len(tgp.Peers))
	round.hasResponded = make(map[PartyID]struct{}, len(tgp.Peers))
	return
}

func (tgp *TripleGenProtocol) processQuery(fromPeer PartyID, encA *bfv.Ciphertext, round *TripleGenRound) (encResponse *bfv.Ciphertext) {

	round.hasQueried[fromPeer] = struct{}{}

	// Computes enc([a_i]) * [b_self] + m_i_self
	tgp.Mul(encA, round.plainB, encA)
	tgp.Add(encA, round.plainM[fromPeer], encA)

	// Adds smudgning error to the ciphertext
	tgp.rq.Add(encA.Value[0], tgp.gaussianSampler.ReadNew(), encA.Value[0])
	tgp.rq.Add(encA.Value[1], tgp.gaussianSampler.ReadNew(), encA.Value[1])

	return encA
}

func (tgp *TripleGenProtocol) processResponse(fromPeer PartyID, encResponse *bfv.Ciphertext, round *TripleGenRound) {

	round.hasResponded[fromPeer] = struct{}{}
	tgp.Add(round.encAggr, encResponse, round.encAggr)
}

func (tgp *TripleGenProtocol) decryptTriples(round *TripleGenRound) (triples []Triple) {

	// [c] = c' + sum([a_self] * [b_j] + m_j)
	round.c = addVec(round.c, tgp.DecodeUintNew(tgp.DecryptNew(round.encAggr)), tgp.q)
	triples = make([]Triple, tgp.n, tgp.n)
	for i := range triples {
		triples[i].A = round.a[i]
		triples[i].B = round.b[i]
		triples[i].C = round.c[i]
	}
	return triples
}

func (tgp *TripleGenProtocol) BindNetwork(nw *TCPNetworkStruct) {
	for partyID, conn := range nw.Conns {

		if partyID == tgp.ID {
			continue
		}

		rp := tgp.Peers[partyID]

		// Receiving loop from remote
		go func(conn net.Conn, rp *TripleGenRemote) {
			wireLen := bfv.NewCiphertext(tgp.params, 1).GetDataLen(true)

			for {
				var id uint64
				var ct bfv.Ciphertext
				var query bool
				var err error
				ctBuff := make([]byte, wireLen, wireLen)
				err = binary.Read(conn, binary.BigEndian, &id)
				if err == io.EOF {
					return
				}
				check(err)
				_, err = io.ReadFull(conn, ctBuff)
				check(err)
				check(ct.UnmarshalBinary(ctBuff))
				check(binary.Read(conn, binary.BigEndian, &query))
				msg := TripleGenMessage{
					PartyID:    PartyID(id),
					Ciphertext: ct,
					Query:      query,
				}
				//fmt.Println(tgp, "receiving", &msg, "from", rp)
				tgp.Chan <- msg
			}
		}(conn, rp)

		// Sending loop of remote
		go func(conn net.Conn, rp *TripleGenRemote) {
			var m TripleGenMessage
			var open = true
			for open {
				m, open = <-rp.Chan
				//fmt.Println(tgp, "sending", &m, "to", rp)
				check(binary.Write(conn, binary.BigEndian, m.PartyID))
				data, err := m.Ciphertext.MarshalBinary()
				check(err)
				_, err = conn.Write(data)
				check(err)
				check(binary.Write(conn, binary.BigEndian, m.Query))
			}
		}(conn, rp)
	}
}
