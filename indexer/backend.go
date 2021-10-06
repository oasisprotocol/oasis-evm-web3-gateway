package indexer

import (
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

// Result is a query result.
type Result struct {
	// TxHash is the hash of the matched transaction.
	TxHash hash.Hash
	// TxIndex is the index of the matched transaction within the block.
	TxIndex uint32
}

// Results are query results.
//
// Map key is the round number and value is a list of transaction hashes
// that match the query.
type Results map[uint64][]Result

// BackendFactory is the indexer backend factory interface.
type BackendFactory func(
	dataDir string,
	runtimeID common.Namespace,
	storage storage.Storage) (Backend, error)

// QueryableBackend is the read-only indexer backend interface.
type QueryableBackend interface {
	// querie block round by block hash.
	QueryBlockRound(blockHash hash.Hash) (uint64, error)

	QueryBlockHash(round uint64) (hash.Hash, error)

	// querie oasis tx result by ethereum tx hash.
	QueryTxResult(ethTransactionHash hash.Hash) (*model.TxResult, error)
}

// Backend is the indexer backend interface.
type Backend interface {
	QueryableBackend

	Index(
		round uint64,
		blockHash hash.Hash,
		txs []*types.UnverifiedTransaction,
	) error

	Close()
}

type psqlBackend struct {
	logger  *logging.Logger
	storage storage.Storage
}

func (p *psqlBackend) Index(
	round uint64,
	blockHash hash.Hash,
	txs []*types.UnverifiedTransaction,
) error {
	//block round <-> block hash
	blockRef := &model.Block{
		Round: round,
		Hash:  blockHash.String(),
	}

	p.storage.Store(blockRef)

	for idx, utx := range txs {
		if len(utx.AuthProofs) != 1 || utx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}

		// Extract raw Ethereum transaction for further processing.
		// Use standard libraries to decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		if err := rlp.DecodeBytes(utx.Body, ethTx); err != nil {
			p.logger.Error("decode ethereum transaction", err)
			continue
		}

		txRef := &model.Transaction{
			EthTxHash: ethTx.Hash().String(),
			Result: &model.TxResult{
				//Hash:  ,
				Index: uint32(idx),
				Round: round,
			},
		}

		p.storage.Store(txRef)
	}

	return nil
}

func (p *psqlBackend) QueryBlockRound(blockHash hash.Hash) (uint64, error) {
	round, err := p.storage.GetBlockRound(blockHash.String())

	if err != nil {
		p.logger.Error("Can't find matched block")
		return 0, err
	}

	return round, nil
}

func (p *psqlBackend) QueryBlockHash(round uint64) (hash.Hash, error) {
	hash, err := p.storage.GetBlockHash(round)

	if err != nil {
		panic("Indexer error!")
		return nil, err
	}

	return hash, nil
}

func (p *psqlBackend) QueryTxResult(ethTransactionHash hash.Hash) (*model.TxResult, error) {
	result, err := p.storage.GetTxResult(ethTransactionHash.String())

	if err != nil {
		p.logger.Error("Can't find matched transaction result")
		return nil, err
	}

	return result, nil
}

func (p *psqlBackend) Close() {
	p.logger.Info("Psql backend closed!")
}

func newPsqlBackend(storage storage.Storage) (Backend, error) {
	b := &psqlBackend{
		logger:  logging.GetLogger("gateway/indexer/backend").With("runtime_id", runtimeID),
		storage: storage,
	}

	b.logger.Info("New psql backend")

	return b, nil
}

func NewPsqlBackend(storage storage.Storage) BackendFactory {
	return newPsqlBackend
}
