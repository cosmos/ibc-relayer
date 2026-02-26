package metrics

import (
	"context"
	"fmt"
	math2 "math"
	"math/big"
	"time"

	"github.com/go-kit/kit/metrics"
	prom "github.com/go-kit/kit/metrics/prometheus"
	stdprom "github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type (
	RelayType  string
	BridgeType string
)

const (
	IBCV2SendToRecvRelayType    RelayType = "ibcv2_send_to_recv"
	IBCV2SendToTimeoutRelayType RelayType = "ibcv2_send_to_timeout"
	IBCV2RecvToAckRelayType     RelayType = "ibcv2_recv_to_ack"

	IBCV2BridgeType BridgeType = "ibcv2"

	SuccessStatus = "success"
	FailureStatus = "failure"

	requestStatusCodeLabel    = "status_code"
	methodLabel               = "method"
	chainIDLabel              = "chain_id"
	clientIDLabel             = "client_id"
	counterpartyChainIDLabel  = "counterparty_chain_id"
	gasBalanceLevelLabel      = "gas_balance_level"
	sourceChainIDLabel        = "source_chain_id"
	destinationChainIDLabel   = "destination_chain_id"
	statusLabel               = "status"
	codeLabel                 = "code"
	successLabel              = "success"
	providerLabel             = "provider"
	chainNameLabel            = "chain_name"
	sourceChainNameLabel      = "source_chain_name"
	destinationChainNameLabel = "destination_chain_name"
	chainEnvironmentLabel     = "chain_environment"
	gasTokenSymbolLabel       = "gas_token_symbol"
	sourceChannelLabel        = "source_channel"
	relayTypelLabel           = "relay_type"
	bridgeTypeLabel           = "bridge_type"
	sourceClientIDLabel       = "source_client_id"
	destinationClientIDLabel  = "destination_client_id"
)

type Metrics interface {
	AddRequest(string, uint32)
	AddRelayRequest(BridgeType, uint32)
	AddRelayerAPIRequestLatency(string, time.Duration)
	SetGasBalance(string, string, string, string, big.Int, big.Int, big.Int, uint8, BridgeType)

	NodeDependencyLatency(string, string, string, string, time.Duration)

	AddRelayerLoop()
	RelayerLoopLatency(time.Duration)
	AttestationAPILatency(latency time.Duration)
	AddAttestationAPIRequest(uint32)
	AttestationConfirmationLatency(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, latency time.Duration)
	AddTransactionSubmitted(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string)
	AddTransactionRetryAttempt(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType)
	AddTransactionConfirmed(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string)
	SetReceiveTransactionGasCost(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, gasCost uint64)
	AddExcessiveRelayLatencyObservation(sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType)
	AddUnprofitableRelay(sourceChainName, destinationChainName, chainEnvironment string)
	AddUntrackedTransfer(string, string, string)
	RelayLatency(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType, latency time.Duration)
	AddRelayCompleted(sourceChainID, sourceClientID, destinationChainID, destinationClientID string, relayType RelayType)
	AddUnrelayedTransferWithAttestationObservation(sourceChainName, destinationChainName, chainEnvironment string)

	AddTransferMonitorChainHeightIncrease(chainID, chainName, chainEnvironment string)

	AddSuccessfulExternalRequest(ctx context.Context, method, code, provider string)
	AddFailedExternalRequest(ctx context.Context, method, code, provider string)
	ExternalRequestLatency(ctx context.Context, method string, provider string, latency time.Duration)

	AddClientNeedsUpated(chainID string, clientID string, counterpartyChainID string)
	AddClientUpdated(chainID string, clientID string, counterpartyChainID string)
}

type (
	metricsContextKey             struct{}
	sourceChainIDContextKey       struct{}
	destinationChainIDContextKey  struct{}
	sourceClientIDContextKey      struct{}
	destinationCleintIDContextKey struct{}
	bridgeTypeContextKey          struct{}
	relayTypeContextKey           struct{}
)

func ContextWithMetrics(ctx context.Context, metrics Metrics) context.Context {
	return context.WithValue(ctx, metricsContextKey{}, metrics)
}

func ContextWithSourceChainID(ctx context.Context, sourceChainID string) context.Context {
	return context.WithValue(ctx, sourceChainIDContextKey{}, sourceChainID)
}

func ContextWithSourceClientID(ctx context.Context, sourceClientID string) context.Context {
	return context.WithValue(ctx, sourceClientIDContextKey{}, sourceClientID)
}

func ContextWithDestChainID(ctx context.Context, destChainID string) context.Context {
	return context.WithValue(ctx, destinationChainIDContextKey{}, destChainID)
}

func ContextWithDestClientID(ctx context.Context, destClientID string) context.Context {
	return context.WithValue(ctx, destinationCleintIDContextKey{}, destClientID)
}

func ContextWithBridgeType(ctx context.Context, bridgeType BridgeType) context.Context {
	return context.WithValue(ctx, bridgeTypeContextKey{}, bridgeType)
}

func ContextWithRelayType(ctx context.Context, relayType RelayType) context.Context {
	return context.WithValue(ctx, relayTypeContextKey{}, relayType)
}

func FromContext(ctx context.Context) Metrics {
	metricsFromContext := ctx.Value(metricsContextKey{})
	if metricsFromContext == nil {
		return NewNoOpMetrics()
	} else {
		return metricsFromContext.(Metrics)
	}
}

func SourceChainIDFromContext(ctx context.Context) string {
	sourceChainID := ctx.Value(sourceChainIDContextKey{})
	if sourceChainID == nil {
		return ""
	}
	sourceChainIDStr, ok := sourceChainID.(string)
	if !ok {
		return ""
	}
	return sourceChainIDStr
}

func SourceClientIDFromContext(ctx context.Context) string {
	sourceClientID := ctx.Value(sourceClientIDContextKey{})
	if sourceClientID == nil {
		return ""
	}
	sourceClientIDStr, ok := sourceClientID.(string)
	if !ok {
		return ""
	}
	return sourceClientIDStr
}

func DestinationChainIDFromContext(ctx context.Context) string {
	destChainID := ctx.Value(destinationChainIDContextKey{})
	if destChainID == nil {
		return ""
	}
	destChainIDStr, ok := destChainID.(string)
	if !ok {
		return ""
	}
	return destChainIDStr
}

func DestinationClientIDFromContext(ctx context.Context) string {
	destClientID := ctx.Value(destinationCleintIDContextKey{})
	if destClientID == nil {
		return ""
	}
	destClientIDStr, ok := destClientID.(string)
	if !ok {
		return ""
	}
	return destClientIDStr
}

func RelayTypeFromContext(ctx context.Context) RelayType {
	relayType := ctx.Value(relayTypeContextKey{})
	if relayType == nil {
		return ""
	}
	relayTypeType, ok := relayType.(RelayType)
	if !ok {
		return ""
	}
	return relayTypeType
}

func BridgeTypeFromContext(ctx context.Context) BridgeType {
	bridgeType := ctx.Value(bridgeTypeContextKey{})
	if bridgeType == nil {
		return ""
	}
	bridgeTypeType, ok := bridgeType.(BridgeType)
	if !ok {
		return ""
	}
	return bridgeTypeType
}

func UnaryServerInterceptor(outerCtx context.Context) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		metricsFromServerContext := FromContext(outerCtx)
		ctx = ContextWithMetrics(ctx, metricsFromServerContext)
		start := time.Now()
		defer func() {
			metricsFromServerContext.AddRequest(info.FullMethod, uint32(status.Code(err)))
			metricsFromServerContext.AddRelayerAPIRequestLatency(info.FullMethod, time.Since(start))
		}()
		resp, err = handler(ctx, req)
		return resp, err
	}
}

func UnaryClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	start := time.Now()
	defer func() {
		FromContext(ctx).ExternalRequestLatency(ctx, method, cc.Target(), time.Since(start))
		if err != nil {
			FromContext(ctx).AddFailedExternalRequest(ctx, method, status.Code(err).String(), cc.Target())
			return
		}
		FromContext(ctx).AddSuccessfulExternalRequest(ctx, method, status.Code(err).String(), cc.Target())
	}()

	return invoker(ctx, method, req, reply, cc, opts...)
}

var _ Metrics = (*PromMetrics)(nil)

type PromMetrics struct {
	totalRequests                             metrics.Counter
	totalRelayRequests                        metrics.Counter
	latencyPerRelayerAPIRequest               metrics.Histogram
	gasBalance                                metrics.Gauge
	gasBalanceState                           metrics.Gauge
	latencyPerNodeDependencyRequest           metrics.Histogram
	totalRelayerLoops                         metrics.Counter
	latencyPerRelayerLoop                     metrics.Histogram
	latencyPerAttestationAPIRequest           metrics.Histogram
	totalAttestationAPIRequests               metrics.Counter
	latencyPerAttestationConfirmation         metrics.Histogram
	totalTransactionSubmitted                 metrics.Counter
	totalTransactionRetryAttempts             metrics.Counter
	totalTransactionsConfirmed                metrics.Counter
	totalUnprofitableRelays                   metrics.Counter
	receiveTransactionGasCost                 metrics.Gauge
	totalUntrackedTransactions                metrics.Counter
	latencyPerRelay                           metrics.Histogram
	transferMonitorChainHeightIncreaseCounter metrics.Counter
	totalNobleForwardingRetryAttempts         metrics.Counter
	totalNobleForwardingRequestsAbandoned     metrics.Counter
	totalNobleForwardingRequests              metrics.Counter
	totalExcessiveRelayLatencyObservations    metrics.Counter
	totalRelaysCompleted                      metrics.Counter
	// totalExternalRequests is a counter that tracks the number of external requests
	totalExternalRequests metrics.Counter
	// latencyPerExternalRequest is a histogram that tracks the time per external request
	latencyPerExternalRequest metrics.Histogram

	totalClientsNeedsUpdated                          metrics.Counter
	totalClientsUpdated                               metrics.Counter
	totalUnrelayedTransferWithAttestationObservations metrics.Counter
}

func NewPromMetrics() Metrics {
	return &PromMetrics{
		totalRequests: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_request_counter",
			Help:      "number of requests, paginated by method and status",
		}, []string{methodLabel, requestStatusCodeLabel}),
		totalRelayRequests: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_relay_requests_per_bridge",
			Help:      "number of requests to relay a transfer for a bridge",
		}, []string{bridgeTypeLabel, requestStatusCodeLabel}),
		latencyPerRelayerAPIRequest: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_relayer_api_request",
			Help:      "latency per relayer_api_request in milliseconds, paginated by method",
			Buckets:   []float64{5, 10, 25, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 1500, 3000, 5000, 10000, 20000},
		}, []string{methodLabel}),
		gasBalance: prom.NewGaugeFrom(stdprom.GaugeOpts{
			Namespace: "relayerapi",
			Name:      "gas_balance_gauge",
			Help:      "gas balances, paginated by chain chain and gas token",
		}, []string{chainIDLabel, chainNameLabel, gasTokenSymbolLabel, chainEnvironmentLabel, bridgeTypeLabel}),
		gasBalanceState: prom.NewGaugeFrom(stdprom.GaugeOpts{
			Namespace: "relayerapi",
			Name:      "gas_balance_state_gauge",
			Help:      "gas balance states (0=ok 1=warning 2=critical), paginated by chain",
		}, []string{chainIDLabel, chainNameLabel, chainEnvironmentLabel, bridgeTypeLabel}),
		latencyPerNodeDependencyRequest: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_node_dependency_request",
			Help:      "latency per node dependency request in milliseconds, paginated by chain id and method",
			Buckets:   []float64{5, 10, 25, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 1500, 3000, 5000, 10000, 20000},
		}, []string{chainIDLabel, chainNameLabel, chainEnvironmentLabel, methodLabel}),
		totalRelayerLoops: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_relayer_loops_counter",
			Help:      "number of relayer loops",
		}, []string{}),
		latencyPerRelayerLoop: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_relayer_loop",
			Help:      "latency per relayer loop in milliseconds",
			Buckets:   []float64{5, 10, 25, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 1500, 3000, 5000, 10000, 20000},
		}, []string{}),
		latencyPerAttestationAPIRequest: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_attestation_api_request",
			Help:      "latency per attestation api request in milliseconds",
			Buckets:   []float64{5, 10, 25, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 1500, 3000, 5000, 10000, 20000},
		}, []string{}),
		totalAttestationAPIRequests: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_attestation_api_requests_counter",
			Help:      "number of attestation api requests, paginated by status code",
		}, []string{requestStatusCodeLabel}),
		latencyPerAttestationConfirmation: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_attestation_confirmation",
			Help:      "latency per attestation confirmation in seconds, paginated by source and destination chain id",
			Buckets:   []float64{30, 60, 300, 600, 900, 1200, 1500, 1800, 2400, 3000, 3600},
		}, []string{sourceChainIDLabel, destinationChainIDLabel, sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel}),
		totalTransactionSubmitted: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_transactions_submitted_counter",
			Help:      "number of transactions submitted, paginated by success status and source and destination chain id",
		}, []string{successLabel, sourceChainIDLabel, destinationChainIDLabel, sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel}),
		totalTransactionRetryAttempts: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_transaction_retry_attempts_counter",
			Help:      "number of transactions retried, paginated by source and destination chain id",
		}, []string{sourceChainIDLabel, destinationChainIDLabel, sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel, relayTypelLabel}),
		totalTransactionsConfirmed: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_transactions_confirmed_counter",
			Help:      "number of transactions confirmed, paginated by success status and source and destination chain id",
		}, []string{successLabel, sourceChainIDLabel, destinationChainIDLabel, sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel}),
		totalUnprofitableRelays: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_unprofitable_relays_counter",
			Help:      "number of unprofitable relays, paginated by route",
		}, []string{sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel}),
		receiveTransactionGasCost: prom.NewGaugeFrom(stdprom.GaugeOpts{
			Namespace: "relayerapi",
			Name:      "receive_transaction_gas_cost_gauge",
			Help:      "receive transaction gas costs, source and destination chain id",
		}, []string{sourceChainIDLabel, destinationChainIDLabel, sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel}),
		totalUntrackedTransactions: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_untracked_transfer_counter",
			Help:      "number of untracked transfers, paginated by chainID",
		}, []string{chainIDLabel, chainNameLabel, chainEnvironmentLabel}),
		latencyPerRelay: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_relay",
			Help:      "latency from relay source transaction to relay completion in milliseconds, paginated by source and destination chain id. for ibcv2 transfers, source and destination chain are from the perspective of the packet being relayed",
			Buckets:   []float64{1000, 2000, 3000, 4000, 5000, 10000, 15000, 20000, 25000, 30000, 45000, 60000, 90000, 120000, 150000, 180000, 210000, 240000, 270000, 300000, 600000, 900000, 1_200_000, 1_500_000, 1_800_000, 1_920_000, 2_100_000, 2_250_000, 2_400_000, 2_700_000, 3_000_000, 3_300_000, 3_600_000},
		}, []string{sourceChainIDLabel, destinationChainIDLabel, sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel, relayTypelLabel}),
		transferMonitorChainHeightIncreaseCounter: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "transfer_monitor_chain_height_increase_counter",
			Help:      "counter tracking increases in chain height in the transfer monitor, paginated by chainID, chain name and chain environment",
		}, []string{chainIDLabel, chainNameLabel, chainEnvironmentLabel}),
		totalNobleForwardingRetryAttempts: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_noble_forwarding_retry_attempts_counter",
			Help:      "number of noble forwarding send packets retried, paginated by source channel and chain environment",
		}, []string{sourceChannelLabel, chainEnvironmentLabel}),
		totalNobleForwardingRequestsAbandoned: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_noble_forwarding_requests_abandoned",
			Help:      "number of noble forwarding requests abandoned, paginated by source channel and chain environment",
		}, []string{sourceChannelLabel, chainEnvironmentLabel}),
		totalNobleForwardingRequests: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_noble_forwarding_requests",
			Help:      "number of noble forwarding requests , paginated by source channel and chain environment",
		}, []string{sourceChannelLabel, chainEnvironmentLabel}),
		totalExcessiveRelayLatencyObservations: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_excessive_relay_latency_observations",
			Help:      "number of times excessive relay latency is observed, paginated by source channel and chain environment. for ibcv2 transfers, source and destination chain are from the perspective of the packet being relayed",
		}, []string{sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel, relayTypelLabel}),
		totalRelaysCompleted: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_relays_completed",
			Help:      "total number of relays that have been completed of each type. source and destination chain are from the perspective of the packet being relayed",
		}, []string{sourceChainIDLabel, sourceClientIDLabel, destinationChainIDLabel, destinationClientIDLabel, relayTypelLabel}),
		totalExternalRequests: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_external_requests",
			Help:      "number of external requests, by name and status",
		}, []string{sourceChainIDLabel, sourceClientIDLabel, destinationChainIDLabel, destinationClientIDLabel, methodLabel, statusLabel, codeLabel, providerLabel, bridgeTypeLabel}),
		latencyPerExternalRequest: prom.NewHistogramFrom(stdprom.HistogramOpts{
			Namespace: "relayerapi",
			Name:      "latency_per_external_request",
			Help:      "latency per each external request (paginated by method name) in milliseconds",
			Buckets:   []float64{50, 100, 200, 300, 500, 750, 1000, 1500, 3000, 5000, 10000, 15000, 20000, 25000, 30000, 35000, 40000, 45000, 50000, 55000, 60000, 65000, 70000, 75000, 80000, 85000, 90000, 95000, 100000},
		}, []string{sourceChainIDLabel, sourceClientIDLabel, destinationChainIDLabel, destinationClientIDLabel, methodLabel, providerLabel, bridgeTypeLabel}),
		totalClientsNeedsUpdated: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_clients_needs_updated",
			Help:      "total number of times it has been observed that a client needs to be updated.",
		}, []string{chainIDLabel, clientIDLabel, counterpartyChainIDLabel}),
		totalClientsUpdated: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_clients_updated",
			Help:      "total number of times a client has been updated.",
		}, []string{chainIDLabel, clientIDLabel, counterpartyChainIDLabel}),
		totalUnrelayedTransferWithAttestationObservations: prom.NewCounterFrom(stdprom.CounterOpts{
			Namespace: "relayerapi",
			Name:      "total_unrelayed_transfer_with_attestation_observations",
			Help:      "total number of times an unrelayed transfer with attestation is observed.",
		}, []string{sourceChainNameLabel, destinationChainNameLabel, chainEnvironmentLabel}),
	}
}

func (m *PromMetrics) AddRequest(method string, statusCode uint32) {
	m.totalRequests.With(methodLabel, method, requestStatusCodeLabel, fmt.Sprint(statusCode)).Add(1)
}

func (m *PromMetrics) AddRelayRequest(bridgeType BridgeType, statusCode uint32) {
	m.totalRelayRequests.With(bridgeTypeLabel, string(bridgeType), requestStatusCodeLabel, fmt.Sprint(statusCode)).Add(1)
}

func (m *PromMetrics) AddRelayerAPIRequestLatency(method string, duration time.Duration) {
	m.latencyPerRelayerAPIRequest.With(methodLabel, method).Observe(float64(duration.Milliseconds()))
}

func (m *PromMetrics) SetGasBalance(chainID, chainName, gasTokenSymbol, chainEnvironment string, gasBalance, warningThreshold, criticalThreshold big.Int, gasTokenDecimals uint8, bridgeType BridgeType) {
	// We compare the gas balance against thresholds locally rather than in the grafana alert definition since
	// the prometheus metric is exported as a float64 and the thresholds reach Wei amounts where precision is lost.
	gasBalanceFloat, _ := gasBalance.Float64()
	gasTokenAmount := gasBalanceFloat / (math2.Pow10(int(gasTokenDecimals)))
	gasBalanceState := 0
	if gasBalance.Cmp(&criticalThreshold) < 0 {
		gasBalanceState = 2
	} else if gasBalance.Cmp(&warningThreshold) < 0 {
		gasBalanceState = 1
	}
	m.gasBalanceState.With(chainIDLabel, chainID, chainNameLabel, chainName, chainEnvironmentLabel, chainEnvironment, bridgeTypeLabel, string(bridgeType)).Set(float64(gasBalanceState))
	m.gasBalance.With(chainIDLabel, chainID, chainNameLabel, chainName, gasTokenSymbolLabel, gasTokenSymbol, chainEnvironmentLabel, chainEnvironment, bridgeTypeLabel, string(bridgeType)).Set(gasTokenAmount)
}

// AddSuccessfulExternalRequest increments the count of successful external
// requests. If you get any response at all from the external server, that is
// considered a successful external request. i.e. a 500 response from the
// server is a successful request since we did receive a response. The fact
// that the response is not what we expected should be recorded in the code.
// The code can record information about the status of the response. This may
// look different per provider, for rpc calls it may be the rpc error response
// code (or nothing if no error), for http requests this may be the http status
// code of the response, and for grpc requests it may be the status code of the
// error.
func (m *PromMetrics) AddSuccessfulExternalRequest(ctx context.Context, method, code, provider string) {
	m.addExternalRequest(ctx, method, SuccessStatus, code, provider, BridgeTypeFromContext(ctx))
}

// AddFailedExternalRequest increments the count of failed external requests.
// Only if you get no response from the server is the request considered a
// failed request. i.e. a 500 response from the server is a successful request
// since we did receive a response. If there is a code possible to retrieve, it
// should recorded, however if it is not possible to record a code, use "-1".
func (m *PromMetrics) AddFailedExternalRequest(ctx context.Context, method, code, provider string) {
	m.addExternalRequest(ctx, method, FailureStatus, code, provider, BridgeTypeFromContext(ctx))
}

func (m *PromMetrics) addExternalRequest(ctx context.Context, method, status, code, provider string, bridgeType BridgeType) {
	sourceChainID := SourceChainIDFromContext(ctx)
	destChainID := DestinationChainIDFromContext(ctx)
	sourceClientID := SourceClientIDFromContext(ctx)
	destinationClientID := DestinationClientIDFromContext(ctx)
	m.totalExternalRequests.With(
		methodLabel, method,
		statusLabel, status,
		codeLabel, code,
		providerLabel, provider,
		bridgeTypeLabel, string(bridgeType),
		sourceChainIDLabel, sourceChainID,
		destinationChainIDLabel, destChainID,
		sourceClientIDLabel, sourceClientID,
		destinationClientIDLabel, destinationClientID,
	).Add(1)
}

func (m *PromMetrics) ExternalRequestLatency(ctx context.Context, method, provider string, latency time.Duration) {
	sourceChainID := SourceChainIDFromContext(ctx)
	destChainID := DestinationChainIDFromContext(ctx)
	sourceClientID := SourceClientIDFromContext(ctx)
	destinationClientID := DestinationClientIDFromContext(ctx)
	m.latencyPerExternalRequest.With(
		methodLabel, method,
		providerLabel, provider,
		bridgeTypeLabel, string(BridgeTypeFromContext(ctx)),
		sourceChainIDLabel, sourceChainID,
		destinationChainIDLabel, destChainID,
		sourceClientIDLabel, sourceClientID,
		destinationClientIDLabel, destinationClientID,
	).Observe(float64(latency.Milliseconds()))
}

func (m *PromMetrics) NodeDependencyLatency(chainID, chainName, chainEnvironment, method string, latency time.Duration) {
	m.latencyPerNodeDependencyRequest.With(chainIDLabel, chainID, chainNameLabel, chainName, chainEnvironmentLabel, chainEnvironment, methodLabel, method).Observe(float64(latency.Milliseconds()))
}

func (m *PromMetrics) AddRelayerLoop() {
	m.totalRelayerLoops.Add(1)
}

func (m *PromMetrics) RelayerLoopLatency(latency time.Duration) {
	m.latencyPerRelayerLoop.Observe(float64(latency.Milliseconds()))
}

func (m *PromMetrics) AttestationAPILatency(latency time.Duration) {
	m.latencyPerAttestationAPIRequest.Observe(float64(latency.Milliseconds()))
}

func (m *PromMetrics) AddAttestationAPIRequest(statusCode uint32) {
	m.totalAttestationAPIRequests.With(requestStatusCodeLabel, fmt.Sprint(statusCode)).Add(1)
}

func (m *PromMetrics) AttestationConfirmationLatency(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, latency time.Duration) {
	m.latencyPerAttestationConfirmation.With(sourceChainIDLabel, sourceChainID, destinationChainIDLabel, destinationChainID, sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment).Observe(float64(latency.Seconds()))
}

func (m *PromMetrics) AddTransactionSubmitted(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string) {
	m.totalTransactionSubmitted.With(successLabel, fmt.Sprint(success), sourceChainIDLabel, sourceChainID, destinationChainIDLabel, destinationChainID, sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) AddTransactionRetryAttempt(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType) {
	m.totalTransactionRetryAttempts.With(
		sourceChainIDLabel, sourceChainID,
		destinationChainIDLabel, destinationChainID,
		sourceChainNameLabel, sourceChainName,
		destinationChainNameLabel, destinationChainName,
		chainEnvironmentLabel, chainEnvironment,
		relayTypelLabel, string(relayType),
	).Add(1)
}

func (m *PromMetrics) AddTransactionConfirmed(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string) {
	m.totalTransactionsConfirmed.With(successLabel, fmt.Sprint(success), sourceChainIDLabel, sourceChainID, destinationChainIDLabel, destinationChainID, sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) AddUnprofitableRelay(sourceChainName, destinationChainName, chainEnvironment string) {
	m.totalUnprofitableRelays.With(sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) SetReceiveTransactionGasCost(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, gasCost uint64) {
	m.receiveTransactionGasCost.With(sourceChainIDLabel, sourceChainID, destinationChainIDLabel, destinationChainID, sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment).Set(float64(gasCost))
}

func (m *PromMetrics) AddUntrackedTransfer(chainID, chainName, chainEnvironment string) {
	m.totalUntrackedTransactions.With(chainIDLabel, chainID, chainNameLabel, chainName, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) RelayLatency(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType, latency time.Duration) {
	m.latencyPerRelay.With(
		sourceChainIDLabel, sourceChainID,
		destinationChainIDLabel, destinationChainID,
		sourceChainNameLabel, sourceChainName,
		destinationChainNameLabel, destinationChainName,
		chainEnvironmentLabel, chainEnvironment,
		relayTypelLabel, string(relayType),
	).Observe(float64(latency.Milliseconds()))
}

func (m *PromMetrics) AddNobleForwardingRetryAttempt(sourceChannel, chainEnvironment string) {
	m.totalNobleForwardingRetryAttempts.With(sourceChannelLabel, sourceChannel, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) AddNobleForwardingRequestAbandoned(sourceChannel, chainEnvironment string) {
	m.totalNobleForwardingRequestsAbandoned.With(sourceChannelLabel, sourceChannel, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) AddNobleForwardingRequest(sourceChannel, chainEnvironment string) {
	m.totalNobleForwardingRequests.With(sourceChannelLabel, sourceChannel, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) AddExcessiveRelayLatencyObservation(sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType) {
	m.totalExcessiveRelayLatencyObservations.With(
		sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment, relayTypelLabel, string(relayType),
	).Add(1)
}

func (m *PromMetrics) AddTransferMonitorChainHeightIncrease(chainID, chainName, chainEnvironment string) {
	m.transferMonitorChainHeightIncreaseCounter.With(chainIDLabel, chainID, chainNameLabel, chainName, chainEnvironmentLabel, chainEnvironment).Add(1)
}

func (m *PromMetrics) AddRelayCompleted(sourceChainID, sourceClientID, destinationChainID, destinationClientID string, relayType RelayType) {
	m.totalRelaysCompleted.With(
		sourceChainIDLabel, sourceChainID, sourceClientIDLabel, sourceClientID, destinationChainIDLabel, destinationChainID, destinationClientIDLabel, destinationClientID, relayTypelLabel, string(relayType),
	).Add(1)
}

func (m *PromMetrics) AddClientNeedsUpated(chainID string, clientID string, counterpartyChainID string) {
	m.totalClientsNeedsUpdated.With(chainIDLabel, chainID, clientIDLabel, clientID, counterpartyChainIDLabel, counterpartyChainID).Add(1)
}

func (m *PromMetrics) AddClientUpdated(chainID string, clientID string, counterpartyChainID string) {
	m.totalClientsUpdated.With(chainIDLabel, chainID, clientIDLabel, clientID, counterpartyChainIDLabel, counterpartyChainID).Add(1)
}

func (m *PromMetrics) AddUnrelayedTransferWithAttestationObservation(sourceChainName, destinationChainName, chainEnvironment string) {
	m.totalUnrelayedTransferWithAttestationObservations.With(sourceChainNameLabel, sourceChainName, destinationChainNameLabel, destinationChainName, chainEnvironmentLabel, chainEnvironment).Add(1)
}

type NoOpMetrics struct{}

func NewNoOpMetrics() Metrics {
	return &NoOpMetrics{}
}

func (m *NoOpMetrics) AddRequest(method string, statusCode uint32)                       {}
func (m *NoOpMetrics) AddRelayRequest(bridgeType BridgeType, statusCode uint32)          {}
func (m *NoOpMetrics) AddRelayerAPIRequestLatency(method string, duration time.Duration) {}
func (m *NoOpMetrics) SetGasBalance(chainID, chainName, gasTokenSymbol, chainEnvironment string, gasBalance, warningThreshold, criticalThreshold big.Int, gasTokenDecimals uint8, bridgeType BridgeType) {
}

func (m *NoOpMetrics) NodeDependencyLatency(chainID, chainName, chainEnvironment, method string, latency time.Duration) {
}
func (m *NoOpMetrics) AddRelayerLoop()                             {}
func (m *NoOpMetrics) RelayerLoopLatency(duration time.Duration)   {}
func (m *NoOpMetrics) AttestationAPILatency(latency time.Duration) {}
func (m *NoOpMetrics) AddAttestationAPIRequest(u uint32)           {}
func (m *NoOpMetrics) AttestationConfirmationLatency(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, latency time.Duration) {
}

func (m *NoOpMetrics) AddTransactionSubmitted(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string) {
}

func (m *NoOpMetrics) AddTransactionRetryAttempt(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType) {
}

func (m *NoOpMetrics) AddTransactionConfirmed(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string) {
}

func (m *NoOpMetrics) SetReceiveTransactionGasCost(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, gasCost uint64) {
}
func (m *NoOpMetrics) AddUntrackedTransfer(chainID, chainName, chainEnvironment string) {}
func (m *NoOpMetrics) RelayLatency(sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType, latency time.Duration) {
}

func (m *NoOpMetrics) AddExcessiveRelayLatencyObservation(sourceChainName, destinationChainName, chainEnvironment string, relayType RelayType) {
}

func (m *NoOpMetrics) AddUnprofitableRelay(sourceChainName, destinationChainName, chainEnvironment string) {
}

func (m *NoOpMetrics) AddTransferMonitorChainHeightIncrease(chainID, chainName, chainEnvironment string) {
}

func (m *NoOpMetrics) AddRelayCompleted(sourceChainID, sourceClientID, destinationChainID, destinationClientID string, relayType RelayType) {
}

func (m *NoOpMetrics) AddSuccessfulExternalRequest(ctx context.Context, method, code, provider string) {
}

func (m *NoOpMetrics) AddFailedExternalRequest(ctx context.Context, method, code, provider string) {
}

func (m *NoOpMetrics) ExternalRequestLatency(ctx context.Context, method, provider string, latency time.Duration) {
}

func (m *NoOpMetrics) AddClientNeedsUpated(chainID string, clientID string, counterpartyChainID string) {
}
func (m *NoOpMetrics) AddClientUpdated(chainID string, clientID string, counterpartyChainID string) {}

func (m *NoOpMetrics) AddUnrelayedTransferWithAttestationObservation(sourceChainName, destinationChainName, chainEnvironment string) {
}
