CREATE TABLE api_key_statuses (
    name TEXT PRIMARY KEY,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO api_key_statuses (name, description) VALUES
('unassigned', 'Key generated but not yet assigned to user'),
('assigned', 'Key assigned to user and active'),
('exhausted', 'All quotas for this key are depleted'),
('revoked', 'Key manually revoked/suspended');
