package config

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	"github.com/cosmos/ibc-relayer/db/gen/db"
)

// Config Enum Types
type ChainType string

const (
	ChainType_COSMOS ChainType = "cosmos"
	ChainType_EVM    ChainType = "evm"
	ChainType_SVM    ChainType = "svm"
)

type BridgeType string

const (
	BridgeType_IBCV2 BridgeType = "ibcv2"
)

func (t BridgeType) ToDBBridgeType() (db.BridgeType, error) {
	switch t {
	case BridgeType_IBCV2:
		return db.BridgeTypeIbcv2, nil
	default:
		return "", fmt.Errorf("invalid bridge type %s", t)
	}
}

type ChainEnvironment string

const (
	ChainEnvironment_MAINNET ChainEnvironment = "mainnet"
	ChainEnvironment_TESTNET ChainEnvironment = "testnet"
)

const (
	EnvServiceAccountToken = "SERVICE_ACCOUNT_TOKEN"
)

// Config Schema
type Config struct {
	Postgres       PostgresConfig         `yaml:"postgres"`
	Chains         map[string]ChainConfig `yaml:"chains"`
	Metrics        MetricsConfig          `yaml:"metrics"`
	RelayerAPI     RelayerAPIConfig       `yaml:"relayer_api"`
	Coingecko      CoingeckoConfig        `yaml:"coingecko,omitempty"`
	IBCV2ProofAPI IBCV2ProofAPIConfig   `yaml:"ibcv2_proof_api"`
	Signing        SigningConfig          `yaml:"signing"`
}

type SigningConfig struct {
	KeysPath        string `yaml:"keys_path"`
	GRPCAddress     string `yaml:"grpc_address"`
	CosmosWalletKey string `yaml:"cosmos_wallet_key"`
	EVMWalletKey    string `yaml:"evm_wallet_key"`
	SVMWalletKey    string `yaml:"svm_wallet_key"`
}

type MetricsConfig struct {
	PrometheusAddress string `yaml:"prometheus_address"`
}

type RelayerAPIConfig struct {
	Address string `yaml:"address"`
}

type PostgresConfig struct {
	Hostname       string `yaml:"hostname"`
	Port           string `yaml:"port"`
	Database       string `yaml:"database"`
	IAMAuthEnabled bool   `yaml:"iam_auth_enabled"`
}

type IBCV2ProofAPIConfig struct {
	GRPCAddress    string `yaml:"grpc_address"`
	GRPCTLSEnabled bool   `yaml:"grpc_tls_enabled"`
}

type IBCV2Config struct {
	// CounterpartyChains is a mapping of source client id to counterparty chain id.
	// This should be populated for any connections the relayer should relay for.
	CounterpartyChains map[string]string `yaml:"counterparty_chains"`

	// FinalityOffset is the number of blocks to wait after a transaction before
	// considering it finalized. If nil (default), the relayer will use the
	// chain's native finality mechanism (e.g., the 'finalized' block tag for EVM).
	// If set to a value (including 0), the relayer will consider a tx finalized once
	// (latest_block - tx_block) >= FinalityOffset.
	FinalityOffset *uint64 `yaml:"finality_offset"`

	// AckBatchSize is the maximum amount of packets that can accumulate in a
	// batch before it is immediately sent (even if the AckBatchTimeout has not
	// elapsed).
	AckBatchSize int `yaml:"ack_batch_size"`

	// AckBatchTimeout controls how long the batch will wait for packets to
	// accumulate before starting to process an incomplete batch
	AckBatchTimeout time.Duration `yaml:"ack_batch_timeout"`

	// Number of concurrent ack batches that can be processing (query RelayByTx
	// and submit on chain) to this chain concurrently
	AckBatchConcurrency int `yaml:"ack_batch_concurrency"`

	// RecvBatchSize is the maximum amount of packets that can accumulate in a
	// batch before it is immediately sent (even if the RecvBatchTimeout has not
	// elapsed).
	RecvBatchSize int `yaml:"recv_batch_size"`

	// RecvBatchTimeout controls how long the batch will wait for packets to
	// accumulate before starting to process an incomplete batch
	RecvBatchTimeout time.Duration `yaml:"recv_batch_timeout"`

	// RecvBatchConcurrency controls the number of concurrent recv batches that
	// can be processing (query RelayByTx and submit on chain) to this chain
	// concurrently
	RecvBatchConcurrency int `yaml:"recv_batch_concurrency"`

	// TimeoutBatchSize is the maximum amount of packets that can accumulate in
	// a batch before it is immediately sent (even if the TimeoutBatchTimeout
	// has not elapsed).
	TimeoutBatchSize int `yaml:"timeout_batch_size"`

	// TimeoutBatchTimeout controls how long the batch will wait for packets to
	// accumulate before starting to process an incomplete batch
	TimeoutBatchTimeout time.Duration `yaml:"timeout_batch_timeout"`

	// TimeoutBatchConcurrency controls the number of concurrent timeout
	// batches that can be processing (query RelayByTx and submit on chain) to
	// this chain concurrently
	TimeoutBatchConcurrency int `yaml:"timeout_batch_concurrency"`

	// ShouldRelaySuccessAcks enables relaying success acks
	ShouldRelaySuccessAcks bool `yaml:"should_relay_success_acks"`

	// ShouldRelayErrorAcks enables relaying error acks
	ShouldRelayErrorAcks bool `yaml:"should_relay_error_acks"`
}

type ChainConfig struct {
	ChainName                string                                        `yaml:"chain_name"`
	ChainID                  string                                        `yaml:"chain_id"`
	Type                     ChainType                                     `yaml:"type"`
	Environment              ChainEnvironment                              `yaml:"environment"`
	Cosmos                   *CosmosConfig                                 `yaml:"cosmos,omitempty"`
	EVM                      *EVMConfig                                    `yaml:"evm,omitempty"`
	SVM                      *SVMConfig                                    `yaml:"svm,omitempty"`
	GasTokenSymbol           string                                        `yaml:"gas_token_symbol"`
	GasTokenCoingeckoID      *string                                       `yaml:"gas_token_coingecko_id"`
	GasTokenDecimals         uint8                                         `yaml:"gas_token_decimals"`
	SignerGasAlertThresholds map[BridgeType]SignerGasAlertThresholdsConfig `yaml:"signer_gas_alert_thresholds"`
	IBCV2                    *IBCV2Config                                 `yaml:"ibcv2"`
	SupportedBridges         []BridgeType                                  `yaml:"supported_bridges"`
}

type SignerGasAlertThresholdsConfig struct {
	// WarningThreshold sets the gas balance threshold at which
	// gas balance metrics indicate the signer gas balance is low
	WarningThreshold string `yaml:"warning_threshold"`
	// CriticalThreshold is a value less than WarningThreshold at which
	// gas balance metrics indicate the signer gas balance is critically low.
	CriticalThreshold string `yaml:"critical_threshold"`
}

type CosmosConfig struct {
	GasPrice float64 `yaml:"gas_price"`

	// IBCV2TxFeeDenom is the denom to use as the fee for ibcv2 txs. This
	// must be set if IBCV2TxFeeAmount is set.
	IBCV2TxFeeDenom string `yaml:"ibcv2_tx_fee_denom"`

	// IBCV2TxFeeAmount is a hardcoded amount to use as the fee when
	// delivering ibcv2 txs to this chain. Only one of GasPrice or
	// IBCV2TxFeeAmount can be set.
	IBCV2TxFeeAmount uint64 `yaml:"ibcv2_tx_fee_amount"`

	RPC             string `yaml:"rpc"`
	RPCBasicAuthVar string `yaml:"rpc_basic_auth_var"`
	GRPC            string `yaml:"grpc"`
	GRPCTLSEnabled  bool   `yaml:"grpc_tls_enabled"`
	AddressPrefix   string `yaml:"address_prefix"`
}

type EVMConfig struct {
	RPC                 string            `yaml:"rpc"`
	RPCBasicAuthVar     string            `yaml:"rpc_basic_auth_var"`
	Contracts           EVMContractConfig `yaml:"contracts"`
	GasFeeCapMultiplier *float64          `yaml:"gas_fee_cap_multiplier"`
	GasTipCapMultiplier *float64          `yaml:"gas_tip_cap_multiplier"`
}

type EVMContractConfig struct {
	ICS26RouterAddress   string `yaml:"ics_26_router_address"`
	ICS20TransferAddress string `yaml:"ics_20_transfer_address"`
}

type SVMConfig struct {
	RPC         string   `yaml:"rpc"`
	WS          string   `yaml:"ws"`
	PriorityFee uint64   `yaml:"priority_fee"`
	SubmitRPCs  []string `yaml:"submit_rpcs"`
}

type CoingeckoConfig struct {
	BaseURL              string        `yaml:"base_url"`
	RequestsPerMinute    int           `yaml:"requests_per_minute"`
	APIKey               string        `yaml:"api_key"`
	CacheRefreshInterval time.Duration `yaml:"cache_refresh_interval"`
}

// Config Helpers

func LoadConfig(path string) (Config, error) {
	cfgBytes, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := yaml.Unmarshal(cfgBytes, &config); err != nil {
		return Config{}, err
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func (c Config) Validate() error {
	if c.RelayerAPI.Address == "" {
		return errors.New("relayer_api.address must be configured")
	}
	return nil
}

// ConfigReader Context Helpers

type configContextKey struct{}

func ConfigReaderContext(ctx context.Context, reader ConfigReader) context.Context {
	return context.WithValue(ctx, configContextKey{}, reader)
}

func GetConfigReader(ctx context.Context) ConfigReader {
	return ctx.Value(configContextKey{}).(ConfigReader)
}

// Complex Config Queries

var ErrNoSignerForBridge = errors.New("no signer for bridge")

type ConfigReader interface {
	Config() Config

	GetPostgresConnString() string
	PostgresIAMAuthEnabled() bool

	GetChainEnvironment(chainID string) (ChainEnvironment, error)
	GetRPCEndpoint(chainID string) (string, error)
	GetGRPCEndpoint(chainID string) (string, bool, error)
	GetBasicAuth(chainID string) (*string, error)
	GetSignerGasAlertThresholds(chainID string, bridgeType BridgeType) (warningThreshold, criticalThreshold *big.Int, err error)

	GetChainConfig(chainID string) (ChainConfig, error)
	GetAllChainConfigsOfType(chainType ChainType) ([]ChainConfig, error)
	GetAllChains() []ChainConfig

	GetCoingeckoConfig() CoingeckoConfig
	GetRelayerAPIConfig() RelayerAPIConfig

	GetIBCV2Config(chainID string) (*IBCV2Config, error)
	GetIBCV2ProofRelayerConfig() IBCV2ProofAPIConfig
	GetIBCV2Chains() []ChainConfig
	GetClientCounterpartyChainID(chainID string, clientID string) (string, error)
	GetAllIBCV2ClientsToCounterparties() (map[string]map[string]string, error)
}

type configReader struct {
	config       Config
	chainIDIndex map[string]ChainConfig
}

func NewConfigReader(config Config) ConfigReader {
	r := &configReader{
		config: config,
	}
	r.createIndexes()
	return r
}

func (r *configReader) createIndexes() {
	r.chainIDIndex = make(map[string]ChainConfig)

	for _, chain := range r.config.Chains {
		r.chainIDIndex[chain.ChainID] = chain
	}
}

func (r configReader) Config() Config {
	return r.config
}

func (r configReader) GetIBCV2ProofRelayerConfig() IBCV2ProofAPIConfig {
	return r.config.IBCV2ProofAPI
}

func (r configReader) GetIBCV2Chains() []ChainConfig {
	var chains []ChainConfig
	for _, chain := range r.config.Chains {
		if chain.IBCV2 != nil {
			chains = append(chains, chain)
		}
	}
	return chains
}

func (r configReader) GetIBCV2Config(chainID string) (*IBCV2Config, error) {
	chain, err := r.GetChainConfig(chainID)
	if err != nil {
		return nil, err
	}

	return chain.IBCV2, nil
}

// GetClientCounterpartyChainID returns the chainID of the chain that is being
// tracked by the clientID on chainID
func (r configReader) GetClientCounterpartyChainID(chainID string, clientID string) (string, error) {
	chain, err := r.GetChainConfig(chainID)
	if err != nil {
		return "", err
	}
	if chain.IBCV2 == nil {
		return "", fmt.Errorf("chain %s is not ibcv2 enabled", chainID)
	}

	trackedChain, ok := chain.IBCV2.CounterpartyChains[clientID]
	if !ok {
		return "", fmt.Errorf("unknown client %s on chain %s", clientID, chainID)
	}
	return trackedChain, nil
}

func (r configReader) PostgresIAMAuthEnabled() bool {
	return r.config.Postgres.IAMAuthEnabled
}

func (r configReader) GetPostgresConnString() string {
	dbUser, ok := os.LookupEnv("POSTGRES_USER")
	if !ok {
		dbUser = "relayer"
	}

	dbPassword, ok := os.LookupEnv("POSTGRES_PASSWORD")
	if !ok {
		dbPassword = "relayer"
	}

	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s",
		r.config.Postgres.Hostname,
		r.config.Postgres.Port,
		dbUser,
		dbPassword,
		r.config.Postgres.Database,
	)
}

func (r configReader) GetChainEnvironment(chainID string) (ChainEnvironment, error) {
	chain, ok := r.chainIDIndex[chainID]
	if !ok {
		return "", fmt.Errorf("chain id %s not found", chainID)
	}

	return chain.Environment, nil
}

func (r configReader) GetRPCEndpoint(chainID string) (string, error) {
	chain, ok := r.chainIDIndex[chainID]
	if !ok {
		return "", fmt.Errorf("chain id %s not found", chainID)
	}

	switch chain.Type {
	case ChainType_COSMOS:
		if chain.Cosmos == nil {
			return "", fmt.Errorf("cosmos config not set for chain %s", chainID)
		}
		return chain.Cosmos.RPC, nil
	case ChainType_EVM:
		if chain.EVM == nil {
			return "", fmt.Errorf("evm config not set for chain %s", chainID)
		}
		return chain.EVM.RPC, nil
	case ChainType_SVM:
		if chain.SVM == nil {
			return "", fmt.Errorf("svm config not set for chain %s", chainID)
		}
		return chain.SVM.RPC, nil
	default:
		return "", fmt.Errorf("unknown chain type %s for chain %s", chain.Type, chainID)
	}
}

func (r configReader) GetGRPCEndpoint(chainID string) (string, bool, error) {
	chain, ok := r.chainIDIndex[chainID]
	if !ok {
		return "", false, fmt.Errorf("chain id %s not found", chainID)
	}

	switch chain.Type {
	case ChainType_COSMOS:
		if chain.Cosmos == nil {
			return "", false, fmt.Errorf("cosmos config not set for chain %s", chainID)
		}
		return chain.Cosmos.GRPC, chain.Cosmos.GRPCTLSEnabled, nil
	case ChainType_EVM:
		return "", false, fmt.Errorf("grpc endpoints not supported for chain type %s", ChainType_EVM)
	case ChainType_SVM:
		return "", false, fmt.Errorf("grpc endpoints not supported for chain type %s", ChainType_SVM)
	default:
		return "", false, fmt.Errorf("unknown chain type %s for chain %s", chain.Type, chainID)
	}
}

func (r configReader) GetBasicAuth(chainID string) (*string, error) {
	chain, ok := r.chainIDIndex[chainID]
	if !ok {
		return nil, fmt.Errorf("chain id %s not found", chainID)
	}

	var basicAuthVar string
	switch chain.Type {
	case ChainType_COSMOS:
		if chain.Cosmos == nil {
			return nil, fmt.Errorf("cosmos config not set for chain %s", chainID)
		}
		basicAuthVar = chain.Cosmos.RPCBasicAuthVar
	case ChainType_EVM:
		if chain.EVM == nil {
			return nil, fmt.Errorf("evm config not set for chain %s", chainID)
		}
		basicAuthVar = chain.EVM.RPCBasicAuthVar
	case ChainType_SVM:
		// SVM chains don't support basic auth
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown chain type %s for chain %s", chain.Type, chainID)
	}

	if basicAuth, ok := os.LookupEnv(basicAuthVar); ok {
		return &basicAuth, nil
	}

	return nil, nil
}

func (r configReader) GetChainConfig(chainID string) (ChainConfig, error) {
	chain, ok := r.chainIDIndex[chainID]
	if !ok {
		return ChainConfig{}, fmt.Errorf("chain id %s not found", chainID)
	}

	return chain, nil
}

func (r configReader) GetAllChainConfigsOfType(chainType ChainType) ([]ChainConfig, error) {
	var chains []ChainConfig
	for _, chain := range r.config.Chains {
		if chain.Type == chainType {
			chains = append(chains, chain)
		}
	}
	return chains, nil
}

func (r configReader) GetAllChains() []ChainConfig {
	var chains []ChainConfig
	for _, chain := range r.config.Chains {
		chains = append(chains, chain)
	}
	return chains
}

func (r configReader) GetSignerGasAlertThresholds(chainID string, bridgeType BridgeType) (warningThreshold, criticalThreshold *big.Int, err error) {
	chain, err := r.GetChainConfig(chainID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting chain config for chain %s: %w", chainID, err)
	}

	gasConfig, ok := chain.SignerGasAlertThresholds[bridgeType]
	if !ok {
		return nil, nil, ErrNoSignerForBridge
	}

	warningThreshold, ok = new(big.Int).SetString(gasConfig.WarningThreshold, 10)
	if !ok {
		return nil, nil, fmt.Errorf("failed to parse gas balance warning threshold amount")
	}
	criticalThreshold, ok = new(big.Int).SetString(gasConfig.CriticalThreshold, 10)
	if !ok {
		return nil, nil, fmt.Errorf("failed to parse gas balance critical threshold amount")
	}

	return warningThreshold, criticalThreshold, nil
}

func (r configReader) GetCoingeckoConfig() CoingeckoConfig {
	return r.config.Coingecko
}

func (r configReader) GetRelayerAPIConfig() RelayerAPIConfig {
	return r.config.RelayerAPI
}

// GetAllIBCV2ClientsToCounterparties, returns a map of chainIDs -> map of client
// ids -> counterparty chain IDs
func (r configReader) GetAllIBCV2ClientsToCounterparties() (map[string]map[string]string, error) {
	clientsToCounterparties := make(map[string]map[string]string)
	chains := r.GetIBCV2Chains()
	for _, chain := range chains {
		ibcv2, err := r.GetIBCV2Config(chain.ChainID)
		if err != nil {
			return nil, fmt.Errorf("getting ibcv2 config for chain %s: %w", chain.ChainID, err)
		}
		clientsToCounterparties[chain.ChainID] = ibcv2.CounterpartyChains
	}
	return clientsToCounterparties, nil
}

func UnaryServerInterceptor(configReader ConfigReader) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ConfigReaderContext(ctx, configReader), req)
	}
}

func GetServiceAccountToken() string {
	return os.Getenv(EnvServiceAccountToken)
}
