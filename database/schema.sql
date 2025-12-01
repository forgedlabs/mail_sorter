-- Email Deliverability Validation System - PostgreSQL Schema
-- Version: 1.0
-- Database: PostgreSQL 15+

-- ============================================================================
-- VALIDATION RESULTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS validation_results (
    id BIGSERIAL,
    email_hash VARCHAR(64) NOT NULL,  -- SHA256 hash for privacy
    email_domain VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('valid', 'invalid', 'catch-all', 'unknown', 'risky')),
    reason VARCHAR(100),
    confidence DECIMAL(3,2) CHECK (confidence >= 0 AND confidence <= 1),
    
    -- SMTP Details
    smtp_code INT,
    smtp_response TEXT,
    mx_host VARCHAR(255),
    mx_records JSONB,
    
    -- Domain Metadata
    is_catch_all BOOLEAN DEFAULT FALSE,
    is_disposable BOOLEAN DEFAULT FALSE,
    
    -- Timing
    validation_duration_ms INT,
    checked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Metadata
    customer_id VARCHAR(50),
    job_id BIGINT,
    
    -- Partitioning key
    created_date DATE NOT NULL DEFAULT CURRENT_DATE,
    
    PRIMARY KEY (id, created_date)
) PARTITION BY RANGE (created_date);

-- Create indexes
CREATE INDEX idx_validation_results_email_hash ON validation_results(email_hash, created_date);
CREATE INDEX idx_validation_results_domain ON validation_results(email_domain, created_date);
CREATE INDEX idx_validation_results_customer ON validation_results(customer_id, created_date);
CREATE INDEX idx_validation_results_job_id ON validation_results(job_id, created_date);
CREATE INDEX idx_validation_results_checked_at ON validation_results(checked_at);

-- Create partitions for current and next 12 months
CREATE TABLE validation_results_2025_11 PARTITION OF validation_results
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE validation_results_2025_12 PARTITION OF validation_results
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

CREATE TABLE validation_results_2026_01 PARTITION OF validation_results
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE validation_results_2026_02 PARTITION OF validation_results
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE validation_results_2026_03 PARTITION OF validation_results
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- Default partition for future dates
CREATE TABLE validation_results_default PARTITION OF validation_results DEFAULT;

-- ============================================================================
-- VALIDATION JOBS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS validation_jobs (
    id BIGSERIAL PRIMARY KEY,
    job_uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    customer_id VARCHAR(50) NOT NULL,
    
    -- Job Configuration
    priority VARCHAR(20) DEFAULT 'standard' CHECK (priority IN ('express', 'standard', 'bulk')),
    total_emails INT NOT NULL,
    
    -- Progress Tracking
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),
    emails_processed INT DEFAULT 0,
    emails_valid INT DEFAULT 0,
    emails_invalid INT DEFAULT 0,
    emails_catch_all INT DEFAULT 0,
    emails_unknown INT DEFAULT 0,
    emails_risky INT DEFAULT 0,
    
    -- Timing
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    -- Callback
    webhook_url TEXT,
    webhook_sent BOOLEAN DEFAULT FALSE,
    
    -- Metadata
    metadata JSONB,
    
    CONSTRAINT valid_progress CHECK (emails_processed <= total_emails)
);

CREATE INDEX idx_validation_jobs_customer ON validation_jobs(customer_id, created_at DESC);
CREATE INDEX idx_validation_jobs_status ON validation_jobs(status, created_at DESC);
CREATE INDEX idx_validation_jobs_uuid ON validation_jobs(job_uuid);

-- ============================================================================
-- DOMAIN METADATA TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS domain_metadata (
    domain VARCHAR(255) PRIMARY KEY,
    
    -- Reputation
    is_catch_all BOOLEAN,
    catch_all_checked_at TIMESTAMP WITH TIME ZONE,
    is_disposable BOOLEAN DEFAULT FALSE,
    is_free_provider BOOLEAN DEFAULT FALSE,
    
    -- MX Information
    mx_records JSONB,
    mx_last_checked TIMESTAMP WITH TIME ZONE,
    
    -- Statistics
    total_validations INT DEFAULT 0,
    valid_count INT DEFAULT 0,
    invalid_count INT DEFAULT 0,
    last_validation_at TIMESTAMP WITH TIME ZONE,
    
    -- Rate Limiting
    last_smtp_check TIMESTAMP WITH TIME ZONE,
    rate_limit_hits INT DEFAULT 0,
    
    -- Blacklist Status
    is_blacklisted BOOLEAN DEFAULT FALSE,
    blacklist_reason TEXT,
    blacklisted_at TIMESTAMP WITH TIME ZONE,
    
    -- Metadata
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_domain_metadata_catch_all ON domain_metadata(is_catch_all) WHERE is_catch_all = TRUE;
CREATE INDEX idx_domain_metadata_disposable ON domain_metadata(is_disposable) WHERE is_disposable = TRUE;
CREATE INDEX idx_domain_metadata_blacklisted ON domain_metadata(is_blacklisted) WHERE is_blacklisted = TRUE;

-- ============================================================================
-- DISPOSABLE DOMAINS TABLE (Pre-populated)
-- ============================================================================

CREATE TABLE IF NOT EXISTS disposable_domains (
    domain VARCHAR(255) PRIMARY KEY,
    added_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    source VARCHAR(50)
);

CREATE INDEX idx_disposable_domains_added ON disposable_domains(added_at DESC);

-- Seed with common disposable domains
INSERT INTO disposable_domains (domain, source) VALUES
    ('tempmail.com', 'manual'),
    ('guerrillamail.com', 'manual'),
    ('10minutemail.com', 'manual'),
    ('throwaway.email', 'manual'),
    ('mailinator.com', 'manual'),
    ('trashmail.com', 'manual'),
    ('getnada.com', 'manual'),
    ('temp-mail.org', 'manual')
ON CONFLICT (domain) DO NOTHING;

-- ============================================================================
-- AUDIT LOG TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    customer_id VARCHAR(50),
    action VARCHAR(50) NOT NULL,
    resource_type VARCHAR(50),
    resource_id VARCHAR(100),
    ip_address INET,
    user_agent TEXT,
    metadata JSONB
);

CREATE INDEX idx_audit_logs_customer ON audit_logs(customer_id, timestamp DESC);
CREATE INDEX idx_audit_logs_action ON audit_logs(action, timestamp DESC);

-- ============================================================================
-- CUSTOMER API KEYS TABLE (for authentication)
-- ============================================================================

CREATE TABLE IF NOT EXISTS api_keys (
    id BIGSERIAL PRIMARY KEY,
    customer_id VARCHAR(50) NOT NULL,
    api_key_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA256 hash
    name VARCHAR(100),
    tier VARCHAR(20) DEFAULT 'free' CHECK (tier IN ('free', 'standard', 'enterprise')),
    
    -- Rate Limits
    rate_limit_per_hour INT DEFAULT 100,
    rate_limit_per_day INT DEFAULT 1000,
    
    -- Status
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_api_keys_customer ON api_keys(customer_id);
CREATE INDEX idx_api_keys_active ON api_keys(is_active) WHERE is_active = TRUE;

-- ============================================================================
-- VIEWS
-- ============================================================================

-- View for recent validations by customer
CREATE OR REPLACE VIEW v_customer_validations AS
SELECT 
    customer_id,
    COUNT(*) as total_validations,
    COUNT(*) FILTER (WHERE status = 'valid') as valid_count,
    COUNT(*) FILTER (WHERE status = 'invalid') as invalid_count,
    COUNT(*) FILTER (WHERE status = 'catch-all') as catch_all_count,
    COUNT(*) FILTER (WHERE status = 'unknown') as unknown_count,
    COUNT(*) FILTER (WHERE status = 'risky') as risky_count,
    AVG(validation_duration_ms) as avg_duration_ms,
    MAX(checked_at) as last_validation_at
FROM validation_results
WHERE checked_at > NOW() - INTERVAL '30 days'
GROUP BY customer_id;

-- View for domain statistics
CREATE OR REPLACE VIEW v_domain_stats AS
SELECT 
    email_domain,
    COUNT(*) as total_checks,
    COUNT(*) FILTER (WHERE status = 'valid') as valid_count,
    COUNT(*) FILTER (WHERE status = 'invalid') as invalid_count,
    ROUND(COUNT(*) FILTER (WHERE status = 'valid')::NUMERIC / NULLIF(COUNT(*), 0) * 100, 2) as valid_percentage,
    MAX(checked_at) as last_checked_at
FROM validation_results
WHERE checked_at > NOW() - INTERVAL '7 days'
GROUP BY email_domain
HAVING COUNT(*) > 5
ORDER BY total_checks DESC;

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

-- Function to update domain metadata after validation
CREATE OR REPLACE FUNCTION update_domain_metadata()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO domain_metadata (domain, total_validations, valid_count, invalid_count, last_validation_at, updated_at)
    VALUES (
        NEW.email_domain,
        1,
        CASE WHEN NEW.status = 'valid' THEN 1 ELSE 0 END,
        CASE WHEN NEW.status = 'invalid' THEN 1 ELSE 0 END,
        NEW.checked_at,
        NOW()
    )
    ON CONFLICT (domain) DO UPDATE SET
        total_validations = domain_metadata.total_validations + 1,
        valid_count = domain_metadata.valid_count + CASE WHEN NEW.status = 'valid' THEN 1 ELSE 0 END,
        invalid_count = domain_metadata.invalid_count + CASE WHEN NEW.status = 'invalid' THEN 1 ELSE 0 END,
        last_validation_at = NEW.checked_at,
        is_catch_all = COALESCE(NEW.is_catch_all, domain_metadata.is_catch_all),
        updated_at = NOW();
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update domain metadata
CREATE TRIGGER trigger_update_domain_metadata
AFTER INSERT ON validation_results
FOR EACH ROW
EXECUTE FUNCTION update_domain_metadata();

-- Function to update job progress
CREATE OR REPLACE FUNCTION update_job_progress()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE validation_jobs
    SET 
        emails_processed = emails_processed + 1,
        emails_valid = emails_valid + CASE WHEN NEW.status = 'valid' THEN 1 ELSE 0 END,
        emails_invalid = emails_invalid + CASE WHEN NEW.status = 'invalid' THEN 1 ELSE 0 END,
        emails_catch_all = emails_catch_all + CASE WHEN NEW.status = 'catch-all' THEN 1 ELSE 0 END,
        emails_unknown = emails_unknown + CASE WHEN NEW.status = 'unknown' THEN 1 ELSE 0 END,
        emails_risky = emails_risky + CASE WHEN NEW.status = 'risky' THEN 1 ELSE 0 END,
        status = CASE 
            WHEN emails_processed + 1 >= total_emails THEN 'completed'
            WHEN started_at IS NULL THEN 'processing'
            ELSE status
        END,
        started_at = COALESCE(started_at, NOW()),
        completed_at = CASE 
            WHEN emails_processed + 1 >= total_emails THEN NOW()
            ELSE completed_at
        END
    WHERE id = NEW.job_id;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update job progress
CREATE TRIGGER trigger_update_job_progress
AFTER INSERT ON validation_results
FOR EACH ROW
WHEN (NEW.job_id IS NOT NULL)
EXECUTE FUNCTION update_job_progress();

-- ============================================================================
-- UTILITY FUNCTIONS
-- ============================================================================

-- Function to clean up old data (for data retention policy)
CREATE OR REPLACE FUNCTION cleanup_old_validations(days_to_keep INT DEFAULT 90)
RETURNS TABLE(deleted_count BIGINT) AS $$
DECLARE
    cutoff_date DATE;
    rows_deleted BIGINT;
BEGIN
    cutoff_date := CURRENT_DATE - days_to_keep;
    
    DELETE FROM validation_results WHERE created_date < cutoff_date;
    GET DIAGNOSTICS rows_deleted = ROW_COUNT;
    
    RETURN QUERY SELECT rows_deleted;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- MAINTENANCE PROCEDURES
-- ============================================================================

-- Create partition for next month (call this monthly via cron)
CREATE OR REPLACE FUNCTION create_next_partition()
RETURNS void AS $$
DECLARE
    next_month DATE;
    following_month DATE;
    partition_name TEXT;
BEGIN
    next_month := DATE_TRUNC('month', CURRENT_DATE + INTERVAL '2 months');
    following_month := next_month + INTERVAL '1 month';
    partition_name := 'validation_results_' || TO_CHAR(next_month, 'YYYY_MM');
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF validation_results FOR VALUES FROM (%L) TO (%L)',
        partition_name,
        next_month,
        following_month
    );
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE validation_results IS 'Stores all email validation results with partitioning by month';
COMMENT ON TABLE validation_jobs IS 'Tracks batch validation jobs and their progress';
COMMENT ON TABLE domain_metadata IS 'Stores domain-level metadata including catch-all status, MX records, and reputation';
COMMENT ON TABLE disposable_domains IS 'List of known disposable/temporary email domains';
COMMENT ON TABLE audit_logs IS 'Audit trail for all API operations';
COMMENT ON TABLE api_keys IS 'Customer API keys for authentication';

COMMENT ON COLUMN validation_results.email_hash IS 'SHA256 hash of email for privacy-preserving lookups';
COMMENT ON COLUMN validation_results.confidence IS 'Confidence score between 0 and 1, where 1 is highest confidence';
COMMENT ON COLUMN validation_results.smtp_code IS 'SMTP response code from RCPT TO command';
COMMENT ON COLUMN validation_results.mx_records IS 'JSON array of MX records at time of validation';

-- ============================================================================
-- GRANTS (adjust based on your security model)
-- ============================================================================

-- Create roles
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'email_validator_app') THEN
        CREATE ROLE email_validator_app WITH LOGIN PASSWORD 'CHANGE_ME_IN_PRODUCTION';
    END IF;
    
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'email_validator_readonly') THEN
        CREATE ROLE email_validator_readonly WITH LOGIN PASSWORD 'CHANGE_ME_IN_PRODUCTION';
    END IF;
END
$$;

-- App role permissions
GRANT CONNECT ON DATABASE postgres TO email_validator_app;
GRANT USAGE ON SCHEMA public TO email_validator_app;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO email_validator_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO email_validator_app;

-- Readonly role permissions  
GRANT CONNECT ON DATABASE postgres TO email_validator_readonly;
GRANT USAGE ON SCHEMA public TO email_validator_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO email_validator_readonly;

-- Set default privileges for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO email_validator_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO email_validator_readonly;
