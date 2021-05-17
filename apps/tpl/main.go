package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ldsec/lattigo/v2/bfv"
)

func main() {
	prog := os.Args[0]
	args := os.Args[1:]

	if len(args) < 4 {
		fmt.Println("Usage:", prog, "[proto] [party ID] [n party] [n beaver]")
		os.Exit(1)
	}

	mhe := args[0] == "mhe"

	partyID, errPartyID := strconv.ParseUint(args[1], 10, 64)
	if errPartyID != nil {
		fmt.Println("Party ID should be an unsigned integer")
		os.Exit(1)
	}

	nParties, errNParty := strconv.ParseUint(args[2], 10, 64)
	if errNParty != nil {
		fmt.Println("n party should be an unsigned integer")
		os.Exit(1)
	}

	nTriple, errNTriple := strconv.ParseUint(args[3], 10, 64)
	if errNTriple != nil || nTriple == 0 {
		fmt.Println("n triples should be a positive integer")
	}

	if mhe {
		ClientMHETripleGen(PartyID(partyID), nParties, nTriple)
		return
	}
	ClientHETripleGen(PartyID(partyID), nParties, nTriple)
	//Client(PartyID(partyID), TestCircuits[circuitNum-1])
}

const BasePort = 50000

func ClientHETripleGen(partyID PartyID, nParties, nTriples uint64) {

	fmt.Println("> Init")

	peers := make(map[PartyID]string)
	for i := uint64(0); i < nParties; i++ {
		peers[PartyID(i)] = fmt.Sprintf("party-%d:50000", i)
	}

	lp, err := NewLocalParty(PartyID(partyID), peers)
	check(err)
	netTripleGen, err := NewTCPNetwork(lp)
	check(err)

	fmt.Print("\testablishing connections...")
	err = netTripleGen.Connect(lp)
	check(err)
	fmt.Println(" done")

	paramsDef := bfv.PN13QP218
	paramsDef.T = uint64(4294475777)
	params, err := bfv.NewParametersFromLiteral(paramsDef)
	if err != nil {
		panic(err)
	}
	sk := bfv.NewKeyGenerator(params).GenSecretKey()
	tripleGenProtocol := lp.NewTripleGenProtocol(params, sk)
	tripleGenProtocol.BindNetwork(netTripleGen)

	fmt.Println("> Triple Generation Phase")
	tripleGenStart := time.Now()
	tripleGenProtocol.Run(nTriples)
	tripleGenTime := time.Since(tripleGenStart)

	triples := make([]Triple, 0, nTriples)
	for t := range tripleGenProtocol.Triples {
		triples = append(triples, t)
	}

	sent, received := netTripleGen.Sum()

	fmt.Printf("\tdone\n")
	fmt.Println("Time:", tripleGenTime.Nanoseconds())
	fmt.Println("Comm:", sent+received)
}

func ClientMHETripleGen(partyID PartyID, nParties, nTriples uint64) {

	fmt.Println("> Init")
	peers := make(map[PartyID]string)
	for i := uint64(0); i < nParties; i++ {
		peers[PartyID(i)] = fmt.Sprintf("party-%d:50000", i)
	}

	tree := NewTree(peers, 2)

	lp, err := NewLocalParty(PartyID(partyID), peers)
	check(err)
	netRLKGen, err := NewTCPNetwork(lp)
	check(err)
	netTripleGen, err := NewTCPNetwork(lp)
	check(err)

	fmt.Print("\testablishing connections...")
	err = netRLKGen.Connect(lp)
	check(err)
	fmt.Println(" done")

	fmt.Println("> MHE Setup")
	paramsDef := bfv.PN13QP218
	paramsDef.T = uint64(4294475777)
	params, err := bfv.NewParametersFromLiteral(paramsDef)
	if err != nil {
		panic(err)
	}

	sk := bfv.NewKeyGenerator(params).GenSecretKey()

	fmt.Println("\tgenerating the relinearization key...")
	rlkGenProtocol := lp.NewRkgProtocol(params, sk, tree)
	rlkGenProtocol.BindNetwork(netRLKGen)
	rlkGenStart := time.Now()
	rlkGenProtocol.Run()
	rlkGenTime := time.Since(rlkGenStart)
	rlk := rlkGenProtocol.rlk
	fmt.Println("\tdone")

	fmt.Println("> Triple Generation Phase")

	fmt.Print("\testablishing connections...")
	err = netTripleGen.Connect(lp)
	check(err)
	fmt.Println(" done")

	fmt.Println("\tgenerating the triples...")
	tripleGenProtocol := lp.NewMHETripleGenProtocol(params, sk, rlk, tree)
	tripleGenProtocol.BindNetwork(netTripleGen)
	triples := make([]Triple, 0, nTriples)
	tripleGenStart := time.Now()
	tripleGenProtocol.Run(nTriples)
	tripleGenTime := time.Since(tripleGenStart)
	for t := range tripleGenProtocol.Triples {
		triples = append(triples, t)
	}
	fmt.Println("\tdone")

	fmt.Println("Setup Time:", rlkGenTime.Nanoseconds())
	sent, received := netRLKGen.Sum()
	fmt.Println("Setup Comm:", sent+received)
	fmt.Println("Time:", tripleGenTime.Nanoseconds())
	sent, received = netTripleGen.Sum()
	fmt.Println("Comm:", sent+received)

	<-time.After(1 * time.Second)
}
