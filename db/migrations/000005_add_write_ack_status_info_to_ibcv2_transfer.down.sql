--- cant actually delete values from an enum so we do nothing
DROP TYPE IF EXISTS ibcv2_write_ack_status;
ALTER TABLE ibcv2_transfers DROP COLUMN ibcv2_write_ack_status;
