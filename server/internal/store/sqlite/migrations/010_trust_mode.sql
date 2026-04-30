-- Add trust_mode column to environments table with default value 'strict'
ALTER TABLE environments ADD COLUMN trust_mode TEXT NOT NULL DEFAULT 'strict';
