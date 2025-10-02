-- Authway Database Initialization Script

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Set timezone
SET timezone = 'UTC';

-- Create initial database objects will be handled by GORM auto-migration
-- This script is mainly for setting up extensions and initial data

-- Insert default OAuth client for development
-- This will be handled by the application initialization