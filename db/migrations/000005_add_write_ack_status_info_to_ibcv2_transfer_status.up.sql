ALTER TYPE ibcv2_relay_status ADD VALUE 'COMPLETE_WITH_WRITE_ACK_SUCCESS';

CREATE TYPE ibcv2_write_ack_status AS ENUM (
    'SUCCESS',
    'ERROR',
    'UNKNOWN'
);

ALTER TABLE ibcv2_transfers ADD COLUMN write_ack_status ibcv2_write_ack_status;
