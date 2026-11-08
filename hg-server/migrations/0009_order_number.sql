-- Order numbers become HG-YYMMDD-XXXXX: the order date in Korea time and the
-- day's count, from 00001. The count is taken in the order's transaction (one
-- row per day, updated under its row lock), so numbers have no gaps and are
-- never shared. Orders made before this keep their HG0000001-style numbers.
CREATE TABLE order_number_days (
    day  date PRIMARY KEY,
    last int  NOT NULL CHECK (last > 0)
);

DROP SEQUENCE order_number_seq;
