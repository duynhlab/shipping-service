-- One shipment per order, for the order-fulfillment saga (Temporal).
--
-- CreateShipment must be idempotent by order_id (a retried activity must not
-- create a second shipment). A UNIQUE constraint lets the insert use
-- ON CONFLICT (order_id) and makes "one shipment per order" a DB invariant.
-- Existing rows already have distinct order_ids, so this is safe to add.
ALTER TABLE shipments ADD CONSTRAINT uq_shipments_order_id UNIQUE (order_id);
