# Relayer Alert Setup

This guide explains how to alert on the relayer metrics that are already exposed by the IBC v2 relayer.

## Prerequisites

Before creating alerts, make sure your relayer metrics are available to Prometheus:

1. Enable the metrics endpoint in your relayer config.
2. Configure Prometheus to scrape that endpoint.
3. Confirm the relevant metric series appear in your Prometheus or Grafana Explore UI.

The relayer exposes Prometheus metrics from the address configured in `metrics.prometheus_address`.

Example:

```yaml
metrics:
  prometheus_address: "0.0.0.0:8888"
```

If the relayer is running in Kubernetes, expose that port with a `Service` or `ServiceMonitor`. If it is running outside Kubernetes, add a standard Prometheus `scrape_config` for the relayer host and port.

## Metrics Used By These Alerts

The alert examples in this document use these relayer metrics:

- `relayerapi_total_excessive_relay_latency_observations`
- `relayerapi_gas_balance_state_gauge`
- `relayerapi_receive_transaction_gas_cost_gauge`

Important labels you will commonly filter on:

- `bridge_type`
- `chain_id`
- `chain_name`
- `chain_environment`
- `gas_token_symbol`
- `source_chain_name`
- `destination_chain_name`
- `relay_type`

For IBC v2 relaying, `bridge_type="ibcv2"`.

## 1. Excessive Relay Latency

### What the relayer emits

The relayer increments `relayerapi_total_excessive_relay_latency_observations` whenever it observes a transfer that has been pending for too long.

The current built-in thresholds are:

- Cosmos source chain, send-to-recv: more than 30 minutes
- EVM source chain, send-to-recv: more than 60 minutes
- Cosmos destination chain, recv-to-ack: more than 30 minutes since write-ack
- EVM destination chain, recv-to-ack: more than 60 minutes since write-ack
- Send-to-timeout: more than 5 minutes after the timeout timestamp, with an extra EVM finality guard before alerting

### Recommended alert rule

This alert fires whenever the counter increases within the last 5 minutes.

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: relayer-latency-alerts
spec:
  groups:
    - name: relayer-latency
      rules:
        - alert: RelayerExcessiveRelayLatency
          expr: |
            sum by (source_chain_name, destination_chain_name, chain_environment, relay_type) (
              increase(relayerapi_total_excessive_relay_latency_observations[5m])
            ) > 0
          for: 0m
          labels:
            severity: warning
            service: relayer
          annotations:
            summary: Excessive relay latency observed
            description: |
              The relayer observed at least one transfer with excessive latency
              in the last 5 minutes for {{ $labels.source_chain_name }} ->
              {{ $labels.destination_chain_name }} ({{ $labels.relay_type }}).
```

### Operator notes

- Use `for: 0m` if you want an alert as soon as the counter increases.
- Increase the lookback window if traffic is bursty and you want to reduce alert churn.
- Split this into separate alerts per `relay_type` if you want different routing for `ibcv2_send_to_recv`, `ibcv2_recv_to_ack`, and `ibcv2_send_to_timeout`.

## 2. Low Gas Balance

### What the relayer emits

The relayer exports two gas-balance metrics:

- `relayerapi_gas_balance_state_gauge`
  - `0` = OK
  - `1` = warning
  - `2` = critical
  - The current signer balance in whole-token units after applying `gas_token_decimals`

The warning and critical thresholds are configured per chain in `chains.<chain_key>.signer_gas_alert_thresholds.ibcv2`.

Example:

```yaml
chains:
  cosmoshub:
    signer_gas_alert_thresholds:
      ibcv2:
        warning_threshold: "5000000"
        critical_threshold: "1000000"
```

### Recommended alert rules

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: relayer-gas-balance-alerts
spec:
  groups:
    - name: relayer-gas-balance
      rules:
        - alert: RelayerGasBalanceLow
          expr: |
            max by (chain_id, chain_name, chain_environment) (
              relayerapi_gas_balance_state_gauge{bridge_type="ibcv2"}
            ) == 1
          for: 10m
          labels:
            severity: warning
            service: relayer
          annotations:
            summary: Relayer signer gas balance is low
            description: |
              The relayer signer balance for {{ $labels.chain_name }} is below
              its configured warning threshold.

        - alert: RelayerGasBalanceCritical
          expr: |
            max by (chain_id, chain_name, chain_environment) (
              relayerapi_gas_balance_state_gauge{bridge_type="ibcv2"}
            ) == 2
          for: 5m
          labels:
            severity: critical
            service: relayer
          annotations:
            summary: Relayer signer gas balance is critical
            description: |
              The relayer signer balance for {{ $labels.chain_name }} is below
              its configured critical threshold.
```

### Operator notes

- The relayer already applies the configured warning and critical thresholds before exporting `relayerapi_gas_balance_state_gauge`.
- Keep the warning threshold high enough to allow time for funding and transaction finality.

## 3. Excessive Gas Usage

### Recommended alert rule

Choose a threshold that is appropriate for the destination chain and your operating budget. The example below uses a placeholder threshold and should be tuned per environment.

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: relayer-gas-usage-alerts
spec:
  groups:
    - name: relayer-gas-usage
      rules:
        - alert: RelayerExcessiveGasUsage
          expr: |
            avg_over_time(
              relayerapi_receive_transaction_gas_cost_gauge{
                chain_environment="mainnet"
              }[10m]
            ) > 2000000
          for: 0m
          labels:
            severity: warning
            service: relayer
          annotations:
            summary: Relayer transaction gas cost is elevated
            description: |
              The relayer's average receive transaction gas cost stayed above
              the configured threshold for the last 10 minutes.
```

### Operator notes

- Tune this threshold separately for each chain pair if gas markets differ materially.
- If you need per-route alerts, group or filter on `source_chain_name` and `destination_chain_name`.
- If you want a stricter signal, use separate rules per destination chain instead of one global threshold.
- This example uses `avg_over_time(...[10m])` so the alert resolves soon after gas costs normalize instead of lingering on a single older spike.
- If your deployment needs alerting on cumulative gas spend over time, add a downstream recording rule or export an additional cumulative metric alongside this gauge.

## Validation

After adding your alert rules:

1. Confirm the alert expressions return data in Prometheus or Grafana Explore.
2. Verify the labels on the resulting series match your routing expectations.
3. Test notifications in a non-production environment before enabling paging.
