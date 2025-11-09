-- ============================================================
-- Migration: 002_add_sample_tenants
-- Description: Add sample tenants for local development
-- Created: 2025-10-18
-- ============================================================

-- Insert Fruits tenant (for Apple and Banana services)
INSERT INTO tenants (id, name, slug, description, active, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111111'::uuid,
    'Fruits Company',
    'fruits',
    'Sample tenant for Apple and Banana services - demonstrates multi-tenant SSO',
    true,
    NOW(),
    NOW()
)
ON CONFLICT (slug) DO NOTHING;

-- Insert Sweets tenant (for Chocolate service)
INSERT INTO tenants (id, name, slug, description, active, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222222'::uuid,
    'Sweets Company',
    'sweets',
    'Sample tenant for Chocolate service - demonstrates tenant isolation',
    true,
    NOW(),
    NOW()
)
ON CONFLICT (slug) DO NOTHING;

COMMENT ON TABLE tenants IS 'Multi-tenant organizations. Each tenant has isolated users and clients.';
