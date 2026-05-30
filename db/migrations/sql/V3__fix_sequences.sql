-- V3__fix_sequences.sql
-- Fix sequence desynchronization caused by seed data inserting explicit ids.
-- Without this, the first application INSERT (creating a shipment) collides on
-- the primary key because shipments_id_seq still points at 1 while the seeded
-- rows already occupy higher ids.

-- Set the sequence for shipments table to the max id
SELECT setval('shipments_id_seq', (SELECT MAX(id) FROM shipments));
