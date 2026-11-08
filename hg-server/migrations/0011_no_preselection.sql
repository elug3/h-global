-- Products are never prepared before a stream: every item is listed on air,
-- numbered from No.1 (plan 5.1). Items picked in advance under the old flow
-- count as existing products added on air.
UPDATE show_items SET source = 'matched' WHERE source = 'preselected';
ALTER TABLE show_items DROP CONSTRAINT show_items_source_check;
ALTER TABLE show_items ADD CONSTRAINT show_items_source_check CHECK (source IN ('captured', 'matched'));
