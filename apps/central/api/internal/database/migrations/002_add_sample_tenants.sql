-- ============================================================
-- Migration: 002_add_sample_tenants
-- Description: Intentionally empty.
-- ============================================================
--
-- This migration used to insert two sample tenants, «Fruits Company» and
-- «Sweets Company», for local development. Every new installation got them,
-- an on-premises one included, where they are only clutter in the console.
-- Databases that already applied it keep the rows (nothing reads them by id);
-- a new database starts with no sample data. The version stays so the
-- sequence of applied migrations is unchanged.

SELECT 1;
