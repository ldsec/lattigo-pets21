package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/ldsec/lattigo/v2/bfv"
	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/ldsec/lattigo/v2/utils"
)

type Node struct {
	Parent   PartyID
	Children []PartyID
}

type Tree map[PartyID]*Node

func NewTree(peers map[PartyID]string, branching uint64) (tree Tree) {

	var node *Node

	nPeers := uint64(len(peers))

	tree = make(map[PartyID]*Node)

	for i := uint64(0); i < nPeers; i++ {
		node = new(Node)

		if i == 0 {
			node.Parent = PartyID(0)
		} else {
			node.Parent = PartyID((i - 1) / branching)
		}

		for j := uint64(0); j < branching; j++ {

			ID := PartyID(i*branching + 1 + j)

			if _, ok := peers[ID]; !ok {
				break
			}

			node.Children = append(node.Children, ID)
		}

		tree[PartyID(i)] = node
	}

	return
}

type MHETripleGenMessage struct {
	PartyID
	Data  []byte
	Round int
}

type MHETripleGenRound struct {
	seed                  []byte
	a, b, c               []uint64
	encA, encB, encC, tmp *bfv.Ciphertext
	decryptionShare       *ring.Poly
}

type MHETripleGenProtocol struct {
	*LocalParty
	*rlwe.SecretKey
	*rlwe.RelinearizationKey
	bfv.Evaluator
	bfv.Encoder
	bfv.Encryptor
	bfv.Decryptor

	gaussianSampler *ring.GaussianSampler

	Triples chan Triple

	Chan     chan MHETripleGenMessage
	Parent   *MHETripleGenRemote
	Children map[PartyID]*MHETripleGenRemote

	rq     *ring.Ring
	n      uint64 // number of beaver triples per ciphertext
	q      uint64 // ring of the beaver triples
	params bfv.Parameters
}

type MHETripleGenRemote struct {
	ID   PartyID
	Chan chan MHETripleGenMessage
}

func (lp *LocalParty) NewMHETripleGenProtocol(params bfv.Parameters, sk *rlwe.SecretKey, rlk *rlwe.RelinearizationKey, tree Tree) *MHETripleGenProtocol {
	tgp := new(MHETripleGenProtocol)
	tgp.LocalParty = lp
	tgp.SecretKey = sk
	tgp.rq = params.RingQ()
	tgp.Evaluator = bfv.NewEvaluator(params, rlwe.EvaluationKey{Rlk: rlk})
	tgp.Encoder = bfv.NewEncoder(params)
	tgp.Encryptor = bfv.NewEncryptorFromSk(params, sk)

	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}
	tgp.gaussianSampler = ring.NewGaussianSampler(prng, tgp.rq, 3.19, 19)

	tgp.params = params
	// Number of Beaver triplets elements (has to comply with the BFV parameters)
	tgp.n = params.N()

	//tgp.context, _ = ring.NewContextWithParams(tgp.n, tgp.params.Qi)

	// Beaver triplets moduli (has to comply with the BFV parameters)
	tgp.q = params.T()

	tgp.Chan = make(chan MHETripleGenMessage, 32)

	if lp.ID != tree[lp.ID].Parent {
		tgp.Parent = &MHETripleGenRemote{
			ID:   tree[lp.ID].Parent,
			Chan: make(chan MHETripleGenMessage, 32),
		}
	}

	tgp.Children = make(map[PartyID]*MHETripleGenRemote)
	for _, child := range tree[lp.ID].Children {
		tgp.Children[PartyID(child)] = &MHETripleGenRemote{
			ID:   child,
			Chan: make(chan MHETripleGenMessage, 32),
		}
	}

	tgp.Triples = make(chan Triple, tgp.n)

	return tgp
}

func (tgp *MHETripleGenProtocol) Run(nTriple uint64) {

	round := tgp.genInput()

	var state uint64

	// We are at round zero -> If we are a leaf, we end enc(a), enc(b) to our Parent
	if len(tgp.Children) == 0 {

		data0, _ := round.encA.MarshalBinary()
		data1, _ := round.encB.MarshalBinary()

		m := MHETripleGenMessage{PartyID: tgp.ID, Data: append(data0, data1...), Round: 0}
		tgp.Parent.Chan <- m
	}
	//fmt.Println(tgp, "sent to", rp, &m,)

	// Then we listen to our Parent and Children
	for m := range tgp.Chan {
		//fmt.Println(tgp, "got from", m.PartyID , &m)

		// If the message is enc(a), enc(b), we aggregate it with our own
		// Once we get enc(a), enc(b) from all our Children, we relay it to our Parent
		if m.Round == 0 {

			// We aggregate enc(a), enc(b) from our Children with our own enc(a), enc(b)
			tgp.aggregateEncAEncB(m.Data, round)

			state++

			// If we have recieved enc(a), enc(b) from both our Children, we relay the aggregation to our Parent
			// unless we are the root, in which case we compute sum(enc(a)) * sum(enc(b))
			if state == uint64(len(tgp.Children)) {

				if tgp.Parent != nil {
					data0, _ := round.encA.MarshalBinary()
					data1, _ := round.encB.MarshalBinary()

					tgp.Parent.Chan <- MHETripleGenMessage{PartyID: tgp.ID, Data: append(data0, data1...), Round: 0}

				} else {

					//tgp.Evaluator.Add(round.encC, round.encA, round.encC)
					//tgp.Evaluator.Add(round.encC, round.encB, round.encC)
					tgp.Evaluator.Mul(round.encA, round.encB, round.tmp)
					tgp.Evaluator.Relinearize(round.tmp, round.encC)

					// And we relay it to our children
					NTTA := round.encC.Value[1].CopyNew()
					tgp.rq.NTT(NTTA, NTTA)
					data, _ := NTTA.MarshalBinary()

					for i := range tgp.Children {
						tgp.Children[i].Chan <- MHETripleGenMessage{PartyID: tgp.ID, Data: data, Round: 1}
					}

					tgp.genDecryptionShare(data, round)
				}

				state = 0
			}

		}

		// Now we wait for a message of the encryption of enc(a)*enc(b)
		if m.Round == 1 {

			// First we try to relay the enc(a)*enc(b) to our Children
			for i := range tgp.Children {
				tgp.Children[i].Chan <- MHETripleGenMessage{PartyID: tgp.ID, Data: m.Data, Round: 1}
			}

			// Then we compute our decryption share
			tgp.genDecryptionShare(m.Data, round)

			// If we are a leaf (no children), we directly relay it to our Parent and close the connection.
			if len(tgp.Children) == 0 {
				data, _ := round.decryptionShare.MarshalBinary()
				tgp.Parent.Chan <- MHETripleGenMessage{PartyID: tgp.ID, Data: data, Round: 2}
				break
			}
		}

		// If we are not a leaf we wait for the decryption share of our Children
		if m.Round == 2 {

			tgp.aggregateDecryptionShare(m.Data, round)

			state++

			// Once we received all the decryption share of all our Children
			// we relay it to our Parent and close the connection
			if state == uint64(len(tgp.Children)) {

				if tgp.Parent != nil {
					data, _ := round.decryptionShare.MarshalBinary()
					tgp.Parent.Chan <- MHETripleGenMessage{PartyID: tgp.ID, Data: data, Round: 2}
					state = 0
				} else {
					tgp.rootFinalize(round)
				}
				break
			}

			fmt.Println("\t\tround 1 ok")
		}
	}

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

func (tgp *MHETripleGenProtocol) genInput() (round *MHETripleGenRound) {
	round = new(MHETripleGenRound)

	round.seed = []byte{0x49, 0x0a, 0x42, 0x3d, 0x97, 0x9d, 0xc1, 0x07, 0xa1, 0xd7, 0xe9, 0x7b, 0x3b, 0xce, 0xa1, 0xdb}

	prng, err := utils.NewKeyedPRNG(round.seed)
	if err != nil {
		panic(err)
	}
	crpGen := ring.NewUniformSampler(prng, tgp.rq)

	// Each party samples its [a] and [b] and computes c' = [a_self] * [b_self]
	round.a = sampleUniformVector(tgp.n, tgp.q)
	round.b = sampleUniformVector(tgp.n, tgp.q)
	round.c = sampleUniformVector(tgp.n, tgp.q)

	// Those [a_self] and [b_self] are encode to a BFV plaintext
	plainA := bfv.NewPlaintext(tgp.params)
	plainB := bfv.NewPlaintext(tgp.params)

	tgp.EncodeUint(round.a, plainA)
	tgp.EncodeUint(round.b, plainB)

	// Each party encrypts [a_self] to a BFV ciphertext : enc([a_self]).
	round.encB = tgp.EncryptFromCRPNew(plainA, crpGen.ReadNew())
	round.encA = tgp.EncryptFromCRPNew(plainB, crpGen.ReadNew())

	round.tmp = bfv.NewCiphertext(tgp.params, 2)
	round.encC = bfv.NewCiphertext(tgp.params, 1)

	round.decryptionShare = ring.NewPoly(tgp.n, uint64(len(tgp.params.Q())))

	return
}

func (tgp *MHETripleGenProtocol) aggregateEncAEncB(data []byte, round *MHETripleGenRound) {

	encA := new(bfv.Ciphertext)
	encB := new(bfv.Ciphertext)

	encA.UnmarshalBinary(data[:len(data)>>1])
	encB.UnmarshalBinary(data[len(data)>>1:])

	tgp.rq.Add(round.encA.Value[0], encA.Value[0], round.encA.Value[0])
	tgp.rq.Add(round.encB.Value[0], encB.Value[0], round.encB.Value[0])

	//tgp.Evaluator.Add(round.encA, encA, round.encA)
	//tgp.Evaluator.Add(round.encB, encB, round.encB)
}

func (tgp *MHETripleGenProtocol) aggregateDecryptionShare(data []byte, round *MHETripleGenRound) {

	share := new(ring.Poly)
	share.UnmarshalBinary(data)

	tgp.rq.Add(round.decryptionShare, share, round.decryptionShare)

}

func (tgp *MHETripleGenProtocol) genDecryptionShare(data []byte, round *MHETripleGenRound) {

	a := new(ring.Poly)
	a.UnmarshalBinary(data)

	share := tgp.rq.NewPoly()

	// a*s
	tgp.rq.MulCoeffsMontgomeryAndAdd(a, tgp.SecretKey.Value, share)
	tgp.rq.InvNTT(share, share)

	if tgp.Parent != nil {
		// a*s + e
		tgp.rq.Add(share, tgp.gaussianSampler.ReadNew(), share)

		c_plain := bfv.NewPlaintext(tgp.params)
		tgp.Encoder.EncodeUint(round.c, c_plain)

		// a*s - c + e
		tgp.rq.Sub(share, c_plain.Value, share)
	}

	round.decryptionShare = share

}

func (tgp *MHETripleGenProtocol) rootFinalize(round *MHETripleGenRound) {

	tgp.rq.Add(round.encC.Value[0], round.decryptionShare, round.encC.Value[0])
	pt := &bfv.Plaintext{&rlwe.Plaintext{Value: round.encC.Value[0]}}
	round.c = tgp.Encoder.DecodeUintNew(pt)
}

func (tgp *MHETripleGenProtocol) decryptTriples(round *MHETripleGenRound) (triples []Triple) {

	triples = make([]Triple, tgp.n, tgp.n)
	for i := range triples {
		triples[i].A = round.a[i]
		triples[i].B = round.b[i]
		triples[i].C = round.c[i]
	}

	return triples
}

func (tgp *MHETripleGenProtocol) BindNetwork(nw *TCPNetworkStruct) {

	var binds []*MHETripleGenRemote

	if tgp.Parent != nil {
		binds = append(binds, tgp.Parent)
	}

	for _, i := range tgp.Children {
		binds = append(binds, i)
	}

	for _, rp := range binds {

		conn := nw.Conns[rp.ID]

		// Receiving loop from remote
		go func(conn net.Conn, rp *MHETripleGenRemote) {

			for {
				var id uint64
				var err error
				var datalen uint64
				var round uint64

				err = binary.Read(conn, binary.BigEndian, &id)
				if err == io.EOF {
					return
				}
				check(err)
				check(binary.Read(conn, binary.BigEndian, &datalen))

				ctBuff := make([]byte, datalen, datalen)
				_, err = io.ReadFull(conn, ctBuff)
				check(err)
				//check(ct.UnmarshalBinary(ctBuff))
				check(binary.Read(conn, binary.BigEndian, &round))
				msg := MHETripleGenMessage{
					PartyID: PartyID(id),
					Data:    ctBuff,
					Round:   int(round),
				}
				//fmt.Println(tgp, "receiving", &msg, "from", rp)
				tgp.Chan <- msg
			}
		}(conn, rp)

		// Sending loop of remote
		go func(conn net.Conn, rp *MHETripleGenRemote) {
			var m MHETripleGenMessage
			var open = true
			for open {
				m, open = <-rp.Chan
				//fmt.Println(tgp, "sending", &m, "to", rp)
				check(binary.Write(conn, binary.BigEndian, m.PartyID))
				check(binary.Write(conn, binary.BigEndian, uint64(len(m.Data))))
				_, err := conn.Write(m.Data)
				check(err)
				check(binary.Write(conn, binary.BigEndian, uint64(m.Round)))
			}
		}(conn, rp)
	}
}
