CREATE TYPE ibcv2_relay_status AS ENUM (
    'PENDING',
    'CHECK_RECV_PACKET_DELIVERY',
    'GET_RECV_PACKET',
    'DELIVER_RECV_PACKET',
    'WAIT_FOR_WRITE_ACK',
    'CHECK_ACK_PACKET_DELIVERY',
    'GET_ACK_PACKET',
    'DELIVER_ACK_PACKET',
    'CHECK_TIMEOUT_PACKET_DELIVERY',
    'GET_TIMEOUT_PACKET',
    'DELIVER_TIMEOUT_PACKET',
    'COMPLETE_WITH_ACK',
    'COMPLETE_WITH_TIMEOUT',
    'FAILED'
);

CREATE TABLE IF NOT EXISTS ibcv2_transfers (
    id serial PRIMARY KEY,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- updated on entry into every step
    status ibcv2_relay_status NOT NULL DEFAULT 'PENDING',

    -- may or may not be populated to contain extra information about the
    -- transfer is in a certain state
    status_text TEXT,

    -- written by during Relay API request
    source_chain_id text NOT NULL,
    destination_chain_id text NOT NULL,
    source_tx_hash text NOT NULL,
    source_tx_time timestamp NOT NULL,
    packet_sequence_number int NOT NULL,
    packet_source_client_id text NOT NULL,
    packet_destination_client_id text NOT NULL,
    packet_timeout_timestamp timestamp NOT NULL,

    -- written by DELIVER_RECV_PACKET step
    recv_tx_hash text,
    recv_tx_time timestamp,

    -- written by the WAIT_FOR_WRITE_ACK_PACKET step
    write_ack_tx_hash text,
    write_ack_tx_time timestamp,

    -- written by the DELIVER_ACK_PACKET step
    ack_tx_hash text,
    ack_tx_time timestamp,

    -- written by TIMEOUT_PACKET step
    timeout_tx_hash text,
    timeout_tx_time timestamp
);

CREATE UNIQUE INDEX index_ibcv2_transfer_packet
ON ibcv2_transfers (source_chain_id, packet_sequence_number, packet_source_client_id);
