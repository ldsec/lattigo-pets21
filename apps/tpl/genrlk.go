package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/ldsec/lattigo/v2/bfv"
	"github.com/ldsec/lattigo/v2/dbfv"
	"github.com/ldsec/lattigo/v2/drlwe"
	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/ldsec/lattigo/v2/utils"
)

type RkgProtocol struct {
	*LocalParty
	params bfv.Parameters
	*rlwe.SecretKey
	*dbfv.RKGProtocol

	u      *rlwe.SecretKey
	share1 *drlwe.RKGShare
	share2 *drlwe.RKGShare
	crp    []*ring.Poly

	rlk *rlwe.RelinearizationKey

	Chan     chan RkgGenMessage
	Parent   *RkgGenRemote
	Children map[PartyID]*RkgGenRemote
}

type RkgGenMessage struct {
	PartyID
	Data  []byte
	Round int
}

type RkgGenRemote struct {
	ID   PartyID
	Chan chan RkgGenMessage
}

func (lp *LocalParty) NewRkgProtocol(params bfv.Parameters, sk *rlwe.SecretKey, tree Tree) (rkg *RkgProtocol) {

	rkg = new(RkgProtocol)
	rkg.params = params
	rkg.LocalParty = lp
	rkg.SecretKey = sk

	rkg.Chan = make(chan RkgGenMessage, 32)

	if lp.ID != tree[lp.ID].Parent {
		rkg.Parent = &RkgGenRemote{
			ID:   tree[lp.ID].Parent,
			Chan: make(chan RkgGenMessage, 32),
		}
	}

	rkg.Children = make(map[PartyID]*RkgGenRemote)
	for _, child := range tree[lp.ID].Children {
		rkg.Children[PartyID(child)] = &RkgGenRemote{
			ID:   child,
			Chan: make(chan RkgGenMessage, 32),
		}
	}

	return
}

func (rkg *RkgProtocol) Run() (evakey *rlwe.RelinearizationKey) {

	rkg.RKGProtocol = dbfv.NewRKGProtocol(rkg.params)
	rkg.u, rkg.share1, rkg.share2 = rkg.RKGProtocol.AllocateShares()

	var state int

	// Root
	if rkg.Parent == nil {

		// First generates a seed, computes the CRP and sends it to its Children
		seed := []byte{'b', 'e', 'a', 'v', 'e', 'r', 's'}

		prng, err := utils.NewKeyedPRNG(seed)
		if err != nil {
			panic(err)
		}
		crpGen := ring.NewUniformSampler(prng, rkg.params.RingQP())
		crp := make([]*ring.Poly, rkg.params.Beta())

		for i := uint64(0); i < rkg.params.Beta(); i++ {
			crp[i] = crpGen.ReadNew()
		}

		rkg.crp = crp

		// Sends the seed to the Children
		for i := range rkg.Children {
			rkg.Children[i].Chan <- RkgGenMessage{PartyID: rkg.ID, Data: seed, Round: 0}

		}

		// And generates its Round1 share
		rkg.GenShareRoundOne(rkg.SecretKey, crp, rkg.u, rkg.share1)

		// Then listen
		for m := range rkg.Chan {

			// Recieves a Round1 share
			if m.Round == 1 {
				// We receive the Round1 share from the Children, and aggregate it with our own
				//fmt.Println("Aggregate Share Round 1 from :", rkg.ID)
				rkg.aggregateShareRound1(m.Data)
				state++

				// If we recieved from all the Children, then we compute our Round2 share and
				// send the aggregate of Round1 share to our children
				if state == len(rkg.Children) {

					data, _ := rkg.share1.MarshalBinary()

					for i := range rkg.Children {
						rkg.Children[i].Chan <- RkgGenMessage{PartyID: rkg.ID, Data: data, Round: 2}
					}

					//fmt.Println("Gen Share Round 2 from :", rkg.ID)
					rkg.GenShareRoundTwo(rkg.u, rkg.SecretKey, rkg.share1, rkg.crp, rkg.share2)

					fmt.Println("\t\tround 1 ok")
					state = 0
				}
			}

			if m.Round == 3 {

				rkg.aggregateShareRound2(m.Data)
				state++

				if state == len(rkg.Children) {

					rkg.rlk = bfv.NewRelinearizationKey(rkg.params, 1)
					rkg.GenRelinearizationKey(rkg.share1, rkg.share2, rkg.rlk)
					fmt.Println("\t\tround 2 ok")
					break
				}
			}
		}

		// Everyone else
	} else {

		// Then listen on the Channel
		for m := range rkg.Chan {
			// Awaits the CRP seed
			if m.Round == 0 {

				// Forwards the seed to the Children
				for i := range rkg.Children {
					rkg.Children[i].Chan <- m
				}

				// Generates the CRP from the seed
				prng, err := utils.NewKeyedPRNG(m.Data)
				if err != nil {
					panic(err)
				}
				crpGen := ring.NewUniformSampler(prng, rkg.params.RingQP())

				crp := make([]*ring.Poly, rkg.params.Beta())

				for i := uint64(0); i < rkg.params.Beta(); i++ {
					crp[i] = crpGen.ReadNew()
				}

				rkg.crp = crp

				// Generates the Round1 share
				//fmt.Println("Gen Share Round 1 from :", rkg.ID)
				rkg.GenShareRoundOne(rkg.SecretKey, crp, rkg.u, rkg.share1)

				// If leaf, then directly broadcast Round1 share to the Parent
				if len(rkg.Children) == 0 {
					data, _ := rkg.share1.MarshalBinary()
					rkg.Parent.Chan <- RkgGenMessage{PartyID: rkg.ID, Data: data, Round: 1}
				}
			}

			// Recieves a Round1 share
			if m.Round == 1 {

				// We receive the Round1 share from the Children, and aggregate it with our own
				//fmt.Println("Gen Share Round 1 from :", rkg.ID)
				rkg.aggregateShareRound1(m.Data)
				state++

				// If we received from all the Children, then we send it to our Parent
				if state == len(rkg.Children) {
					data, _ := rkg.share1.MarshalBinary()
					rkg.Parent.Chan <- RkgGenMessage{PartyID: rkg.ID, Data: data, Round: 1}
					state = 0
					fmt.Println("\t\tround 1 ok")
				}
			}

			// Recieves the aggregation of the Round1 shares
			if m.Round == 2 {

				// Forwards the aggretated Round1 shares to the Children
				for i := range rkg.Children {
					rkg.Children[i].Chan <- m
				}

				// We update the share1 with the aggregated one
				if err := rkg.share1.UnmarshalBinary(m.Data); err != nil {
					panic(err)
				}

				// Computes Round2 share
				//fmt.Println("Gen Share Round 1 from :", rkg.ID)
				rkg.GenShareRoundTwo(rkg.u, rkg.SecretKey, rkg.share1, rkg.crp, rkg.share2)

				// If leaf, then send directly Round2 share to the parent
				if len(rkg.Children) == 0 {
					data, _ := rkg.share2.MarshalBinary()
					rkg.Parent.Chan <- RkgGenMessage{PartyID: rkg.ID, Data: data, Round: 3}
					fmt.Println("\t\tround 2 ok")
					break
				}
			}

			// Recieves a Round2 share from a Children
			if m.Round == 3 {

				//fmt.Println("Aggregate Share Round 2 from :", rkg.ID)
				rkg.aggregateShareRound2(m.Data)
				state++

				// If we received from all the Children, then we send it to our Parent
				if state == len(rkg.Children) {
					data, _ := rkg.share2.MarshalBinary()
					rkg.Parent.Chan <- RkgGenMessage{PartyID: rkg.ID, Data: data, Round: 3}
					state = 0
					fmt.Println("\t\tround 2 ok")
					break
				}
			}
		}
	}
	return nil
}

func (rkg *RkgProtocol) aggregateShareRound1(data []byte) {
	share := new(drlwe.RKGShare)
	if err := share.UnmarshalBinary(data); err != nil {
		panic(err)
	}
	rkg.RKGProtocol.AggregateShares(rkg.share1, share, rkg.share1)
}

func (rkg *RkgProtocol) aggregateShareRound2(data []byte) {
	share := new(drlwe.RKGShare)
	if err := share.UnmarshalBinary(data); err != nil {
		panic(err)
	}
	rkg.RKGProtocol.AggregateShares(rkg.share2, share, rkg.share2)
}

func (rkg *RkgProtocol) BindNetwork(nw *TCPNetworkStruct) {

	var binds []*RkgGenRemote

	if rkg.Parent != nil {
		binds = append(binds, rkg.Parent)
	}

	for _, i := range rkg.Children {
		binds = append(binds, i)
	}

	for _, rp := range binds {
		conn := nw.Conns[rp.ID]

		// Receiving loop from remote
		go func(conn net.Conn, rp *RkgGenRemote) {

			if conn == nil {
				panic(fmt.Errorf("conn is nil for %s", rp.ID))
			}

			for {
				var id uint64
				var round uint64
				var err error
				var datalen uint64

				err = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
				if err != nil {
					panic(fmt.Errorf("SetReadDeadline failed:", err))
				}

				err = binary.Read(conn, binary.BigEndian, &id)
				if err != nil {
					if err == io.EOF || err.Error() == syscall.ECONNRESET.Error() {
						return
					}
					panic(err)
				}
				check(binary.Read(conn, binary.BigEndian, &datalen))

				ctBuff := make([]byte, datalen, datalen)
				_, err = io.ReadFull(conn, ctBuff)
				check(err)
				//check(ct.UnmarshalBinary(ctBuff))
				check(binary.Read(conn, binary.BigEndian, &round))
				msg := RkgGenMessage{
					PartyID: PartyID(id),
					Data:    ctBuff,
					Round:   int(round),
				}

				//fmt.Println(rkg, "receiving", msg.Round, "from", rp.ID)

				rkg.Chan <- msg
			}
		}(conn, rp)

		// Sending loop of remote
		go func(conn net.Conn, rp *RkgGenRemote) {
			var m RkgGenMessage
			var open = true
			for open {
				m, open = <-rp.Chan

				//fmt.Println(rkg, "sending m round", m.Round, "to", rp.ID)
				check(binary.Write(conn, binary.BigEndian, m.PartyID))
				check(binary.Write(conn, binary.BigEndian, uint64(len(m.Data))))
				_, err := conn.Write(m.Data)
				check(err)
				check(binary.Write(conn, binary.BigEndian, uint64(m.Round)))
			}
		}(conn, rp)
	}
}
