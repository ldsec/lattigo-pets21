package main

import (
	"fmt"
	"sync"
)

type PartyID uint64

type Party struct {
	ID   PartyID
	Addr string
}

type LocalParty struct {
	Party
	*sync.WaitGroup
	Peers map[PartyID]*RemoteParty
}

func check(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func NewLocalParty(id PartyID, peers map[PartyID]string) (*LocalParty, error) {
	p := &LocalParty{}
	p.ID = id

	p.Peers = make(map[PartyID]*RemoteParty, len(peers))
	p.Addr = peers[id]

	var err error
	for pId, pAddr := range peers {
		p.Peers[pId], err = NewRemoteParty(pId, pAddr)
		if err != nil {
			return nil, err
		}
	}

	return p, nil
}

func (lp *LocalParty) String() string {
	return fmt.Sprintf("party-%d", lp.ID)
}

type RemoteParty struct {
	Party
}

func (rp *RemoteParty) String() string {
	return fmt.Sprintf("party-%d", rp.ID)
}

func NewRemoteParty(id PartyID, addr string) (*RemoteParty, error) {
	p := &RemoteParty{}
	p.ID = id
	p.Addr = addr
	return p, nil
}
