// Package fabric wraps the Hyperledger Fabric Gateway Go SDK for the FINIX
// event ledger.
//
// Responsibilities:
//   - Connect to peer0 over mTLS with the FinixMSP application identity.
//   - Submit RecordEvent transactions (endorse -> order -> commit).
//   - Evaluate read-only queries (QueryByUser / QueryByCategory / GetProof).
//   - Stream committed chaincode events so the caller can mirror them into
//     Postgres (the off-chain world-state used for fast API reads).
//
// Fabric remains the source of truth; Postgres is a derived read model.
package fabric

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config is the subset of settings the client needs (mirrors config.FabricConfig).
type Config struct {
	PeerEndpoint string
	GatewayPeer  string
	Channel      string
	Chaincode    string
	MSPID        string
	CertPath     string
	KeyDir       string
	TLSCertPath  string
}

// Event mirrors the chaincode's Event struct (and backend blockchain.Event).
type Event struct {
	ID            string              `json:"id"`
	UserID        string              `json:"userId"`
	Action        string              `json:"action"`
	Resource      string              `json:"resource"`
	Category      string              `json:"category"`
	PayloadHash   string              `json:"payloadHash"`
	PrevHash      string              `json:"prevHash"`
	Hash          string              `json:"hash"`
	Timestamp     time.Time           `json:"timestamp"`
	Seq           uint64              `json:"seq"`
	TxID          string              `json:"txId"`
	Payload       string              `json:"payload"`
	SmartContract SmartContractResult `json:"smartContract"`

	// BlockNum is the committed Fabric block. It is not part of the chaincode
	// payload; the event listener fills it from the delivered block so the
	// Postgres mirror row is traceable on-chain. Zero for query results.
	BlockNum uint64 `json:"-"`
}

// SmartContractResult mirrors the chaincode rule outcome.
type SmartContractResult struct {
	RuleName   string    `json:"ruleName"`
	Passed     bool      `json:"passed"`
	Violation  string    `json:"violation"`
	EnforcedAt time.Time `json:"enforcedAt"`
}

// Proof mirrors the chaincode integrity summary.
type Proof struct {
	UserID     string `json:"userId"`
	MerkleRoot string `json:"merkleRoot"`
	EventCount int    `json:"eventCount"`
	HeadHash   string `json:"headHash"`
	Intact     bool   `json:"intact"`
	Detail     string `json:"detail"`
}

// Client is a connected Fabric gateway.
type Client struct {
	cfg      Config
	conn     *grpc.ClientConn
	gw       *client.Gateway
	contract *client.Contract
	network  *client.Network
}

// New dials the peer, authenticates as the FinixMSP app identity and returns a
// ready client. Callers must Close it.
func New(cfg Config) (*Client, error) {
	tlsCert, err := loadCertificate(cfg.TLSCertPath)
	if err != nil {
		return nil, fmt.Errorf("load peer TLS cert: %w", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(tlsCert)
	transport := credentials.NewClientTLSFromCert(pool, cfg.GatewayPeer)

	conn, err := grpc.NewClient(cfg.PeerEndpoint, grpc.WithTransportCredentials(transport))
	if err != nil {
		return nil, fmt.Errorf("dial peer %s: %w", cfg.PeerEndpoint, err)
	}

	id, err := newIdentity(cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	sign, err := newSigner(cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}

	gw, err := client.Connect(id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(10*time.Second),
		client.WithEndorseTimeout(20*time.Second),
		client.WithSubmitTimeout(10*time.Second),
		client.WithCommitStatusTimeout(2*time.Minute),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("connect gateway: %w", err)
	}

	network := gw.GetNetwork(cfg.Channel)
	return &Client{
		cfg:      cfg,
		conn:     conn,
		gw:       gw,
		network:  network,
		contract: network.GetContract(cfg.Chaincode),
	}, nil
}

// Close releases the gateway and gRPC connection.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.gw != nil {
		c.gw.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// RecordEvent submits an event to the ledger. A smart-contract rejection (e.g.
// cooling-off) surfaces here as an error — the transaction is not committed.
func (c *Client) RecordEvent(ctx context.Context, category, userID, action string, payload any) (Event, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal payload: %w", err)
	}

	result, err := c.contract.SubmitWithContext(ctx, "RecordEvent",
		client.WithArguments(category, userID, action, string(payloadJSON)),
	)
	if err != nil {
		return Event{}, fmt.Errorf("submit RecordEvent: %w", unwrapGatewayError(err))
	}

	var ev Event
	if err := json.Unmarshal(result, &ev); err != nil {
		return Event{}, fmt.Errorf("unmarshal event: %w", err)
	}
	return ev, nil
}

// QueryByUser returns every ledger event for a user (oldest first).
func (c *Client) QueryByUser(ctx context.Context, userID string) ([]Event, error) {
	return c.evaluateEvents(ctx, "QueryByUser", userID)
}

// QueryByCategory returns a user's events within one category.
func (c *Client) QueryByCategory(ctx context.Context, userID, category string) ([]Event, error) {
	return c.evaluateEvents(ctx, "QueryByCategory", userID, category)
}

// GetProof returns the on-chain recomputed Merkle root / chain integrity.
func (c *Client) GetProof(ctx context.Context, userID string) (Proof, error) {
	result, err := c.contract.EvaluateWithContext(ctx, "GetProof", client.WithArguments(userID))
	if err != nil {
		return Proof{}, fmt.Errorf("evaluate GetProof: %w", unwrapGatewayError(err))
	}
	var p Proof
	if err := json.Unmarshal(result, &p); err != nil {
		return Proof{}, fmt.Errorf("unmarshal proof: %w", err)
	}
	return p, nil
}

// VerifyCoolingOff reports the on-chain cooling-off state for a user.
func (c *Client) VerifyCoolingOff(ctx context.Context, userID string) (bool, time.Duration, error) {
	result, err := c.contract.EvaluateWithContext(ctx, "VerifyCoolingOff", client.WithArguments(userID))
	if err != nil {
		return false, 0, fmt.Errorf("evaluate VerifyCoolingOff: %w", unwrapGatewayError(err))
	}
	var out struct {
		Active           bool  `json:"active"`
		RemainingSeconds int64 `json:"remainingSeconds"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return false, 0, fmt.Errorf("unmarshal cooling-off: %w", err)
	}
	return out.Active, time.Duration(out.RemainingSeconds) * time.Second, nil
}

// BlockHeight returns the committed block height of the channel — the ledger's
// tamper-evidence of record, used by VerifyIntegrity.
func (c *Client) BlockHeight(ctx context.Context) (uint64, error) {
	result, err := c.network.GetContract("qscc").EvaluateWithContext(ctx, "GetChainInfo",
		client.WithArguments(c.cfg.Channel))
	if err != nil {
		return 0, fmt.Errorf("qscc GetChainInfo: %w", unwrapGatewayError(err))
	}
	// The response is a protobuf common.BlockchainInfo; the height is the first
	// varint field, which is all we need here (avoids a protobuf dependency).
	height, err := firstVarintField(result)
	if err != nil {
		return 0, fmt.Errorf("decode chain info: %w", err)
	}
	return height, nil
}

// Events streams committed chaincode events (finix.event) from startBlock.
// The returned channel closes when ctx is cancelled. Used to mirror the ledger
// into Postgres.
func (c *Client) Events(ctx context.Context, startBlock uint64) (<-chan Event, error) {
	opts := []client.ChaincodeEventsOption{}
	if startBlock > 0 {
		opts = append(opts, client.WithStartBlock(startBlock))
	}
	ccEvents, err := c.network.ChaincodeEvents(ctx, c.cfg.Chaincode, opts...)
	if err != nil {
		return nil, fmt.Errorf("subscribe chaincode events: %w", err)
	}

	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ccEv, ok := <-ccEvents:
				if !ok {
					return
				}
				if ccEv.EventName != "finix.event" {
					continue
				}
				var ev Event
				if err := json.Unmarshal(ccEv.Payload, &ev); err != nil {
					continue // skip malformed; the ledger remains authoritative
				}
				if ev.TxID == "" {
					ev.TxID = ccEv.TransactionID
				}
				// Traceability: the block that committed this event.
				ev.BlockNum = ccEv.BlockNumber
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// ── internals ───────────────────────────────────────────────

func (c *Client) evaluateEvents(ctx context.Context, fn string, args ...string) ([]Event, error) {
	result, err := c.contract.EvaluateWithContext(ctx, fn, client.WithArguments(args...))
	if err != nil {
		return nil, fmt.Errorf("evaluate %s: %w", fn, unwrapGatewayError(err))
	}
	if len(result) == 0 {
		return nil, nil
	}
	var events []Event
	if err := json.Unmarshal(result, &events); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", fn, err)
	}
	return events, nil
}

func newIdentity(cfg Config) (*identity.X509Identity, error) {
	certPath, err := firstFileIn(cfg.CertPath)
	if err != nil {
		return nil, fmt.Errorf("locate signcert: %w", err)
	}
	cert, err := loadCertificate(certPath)
	if err != nil {
		return nil, fmt.Errorf("load signcert: %w", err)
	}
	id, err := identity.NewX509Identity(cfg.MSPID, cert)
	if err != nil {
		return nil, fmt.Errorf("new x509 identity: %w", err)
	}
	return id, nil
}

func newSigner(cfg Config) (identity.Sign, error) {
	keyPath, err := firstFileIn(cfg.KeyDir)
	if err != nil {
		return nil, fmt.Errorf("locate private key: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	privateKey, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	sign, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		return nil, fmt.Errorf("new signer: %w", err)
	}
	return sign, nil
}

func loadCertificate(path string) (*x509.Certificate, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return identity.CertificateFromPEM(pemBytes)
}

// firstFileIn returns path itself if it is a file, else the first file inside it.
// cryptogen emits randomly-named keystore files, so the directory is scanned.
func firstFileIn(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return path, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			return filepath.Join(path, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no file found in %s", path)
}

// unwrapGatewayError surfaces the chaincode's own message (e.g. a smart-contract
// rejection) instead of the opaque gRPC wrapper.
func unwrapGatewayError(err error) error {
	var endorseErr *client.EndorseError
	if errors.As(err, &endorseErr) && endorseErr.TransactionError != nil {
		return fmt.Errorf("%s", endorseErr.TransactionError.Error())
	}
	var submitErr *client.SubmitError
	if errors.As(err, &submitErr) {
		return fmt.Errorf("%s", submitErr.Error())
	}
	return err
}

// firstVarintField decodes field 1 (a varint) of a protobuf message.
func firstVarintField(b []byte) (uint64, error) {
	if len(b) == 0 {
		return 0, errors.New("empty message")
	}
	// tag for field 1, varint wire type == 0x08
	if b[0] != 0x08 {
		return 0, fmt.Errorf("unexpected first field tag 0x%02x", b[0])
	}
	var value uint64
	var shift uint
	for i := 1; i < len(b); i++ {
		value |= uint64(b[i]&0x7f) << shift
		if b[i]&0x80 == 0 {
			return value, nil
		}
		shift += 7
		if shift > 63 {
			return 0, errors.New("varint overflow")
		}
	}
	return 0, errors.New("truncated varint")
}
