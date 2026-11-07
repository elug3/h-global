-- Order numbers are HG-YYMMDD-XXXXX with XXXXX random (so they do not show
-- how many orders a day brings). orders.number stays UNIQUE; the daily
-- counter from 0009 is no longer used.
DROP TABLE IF EXISTS order_number_days;
