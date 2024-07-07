package main

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	blobstreamxwrapper "github.com/succinctlabs/blobstreamx/bindings"
	"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/rpc/client/http"
	"encoding/hex"
	"math/big"
	"os"
)

func main() {
	err := verify()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func verify() error {
	ctx := context.Background()

	// start the tendermint RPC client
	trpc, err := http.New("https://node.arabica.celenium.io:443", "/websocket")
	if err != nil {
		return err
	}
	err = trpc.Start()
	if err != nil {
		return err
	}
	txHash, err := hex.DecodeString("DD4E2253312CD8D251F2673CFF5E283B144F601D2493A862C8A9EDE855700F80")
    if err != nil {
		return err
	}

	// get the PayForBlob transaction that contains the published blob
	tx, err := trpc.Tx(ctx, txHash, true)
	if err != nil {
		return err
	}

	// get the block containing the PayForBlob transaction
	blockRes, err := trpc.Block(ctx, &tx.Height)
	if err != nil {
		return err
	}

	// get the nonce corresponding to the block height that contains
	// the PayForBlob transaction
	// since BlobstreamX emits events when new batches are submitted,
	// we will query the events
	// and look for the range committing to the blob
	// first, connect to an EVM RPC endpoint
	ethClient, err := ethclient.Dial("https://ethereum-holesky-rpc.publicnode.com")
	// ethClient, err := ethclient.Dial("https://ethereum-holesky.blockpi.network/v1/rpc/41e60fb717494222bb14179a551895f4786d654b")
	if err != nil {
		return err
	}
	defer ethClient.Close()

	// use the BlobstreamX contract binding
	wrapper, err := blobstreamxwrapper.NewBlobstreamX(ethcmn.HexToAddress("0x8354693274eAe91Bc11B4b8981a8aB26d85F4A66"), ethClient)
	if err != nil {
		return err
	}


	// get the block data root inclusion proof to the data root tuple root
	dcProof, err := trpc.DataRootInclusionProof(ctx, uint64(tx.Height), 1421160, 1421175)
	if err != nil {
		return err
	}

	// verify that the data root was committed to by the BlobstreamX contract
	committed, err := VerifyDataRootInclusion(ctx, wrapper, 0, uint64(tx.Height), blockRes.Block.DataHash, dcProof.Proof)
	if err != nil {
		return err
	}
	if committed {
		fmt.Println("data root was committed to by the BlobstreamX contract")
	} else {
		fmt.Println("data root was not committed to by the BlobstreamX contract")
		return nil
	}
    return nil
}

func VerifyDataRootInclusion(
	_ context.Context,
	blobstreamXwrapper *blobstreamxwrapper.BlobstreamX,
	nonce uint64,
	height uint64,
	dataRoot []byte,
	proof merkle.Proof,
) (bool, error) {
	tuple := blobstreamxwrapper.DataRootTuple{
		Height:   big.NewInt(int64(height)),
		DataRoot: *(*[32]byte)(dataRoot),
	}

	sideNodes := make([][32]byte, len(proof.Aunts))
	for i, aunt := range proof.Aunts {
		sideNodes[i] = *(*[32]byte)(aunt)
	}
	wrappedProof := blobstreamxwrapper.BinaryMerkleProof{
		SideNodes: sideNodes,
		Key:       big.NewInt(proof.Index),
		NumLeaves: big.NewInt(proof.Total),
	}

	valid, err := blobstreamXwrapper.VerifyAttestation(
		&bind.CallOpts{},
		big.NewInt(int64(nonce)),
		tuple,
		wrappedProof,
	)
	if err != nil {
		return false, err
	}
	return valid, nil
}