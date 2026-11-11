CREATE TABLE profile_metadata (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    profile_id BLOB NOT NULL UNIQUE
        CHECK (length(profile_id) = 16 AND profile_id != zeroblob(16))
) STRICT;

CREATE TABLE repositories (
    github_host TEXT NOT NULL
        CHECK (length(trim(github_host)) > 0 AND github_host = lower(github_host)),
    node_id TEXT NOT NULL CHECK (length(trim(node_id)) > 0),
    organization_node_id TEXT,
    owner_login TEXT NOT NULL CHECK (length(trim(owner_login)) > 0),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    authorization_state TEXT NOT NULL DEFAULT 'unknown'
        CHECK (authorization_state IN ('unknown', 'accessible', 'denied')),
    discovered_at TEXT,
    PRIMARY KEY (github_host, node_id)
) STRICT;

CREATE INDEX repositories_by_organization
    ON repositories (github_host, organization_node_id, node_id);

CREATE TABLE repository_refresh_progress (
    github_host TEXT NOT NULL,
    repository_node_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'not_started'
        CHECK (state IN ('not_started', 'running', 'partial', 'complete')),
    next_cursor TEXT,
    last_attempt_at TEXT,
    last_success_at TEXT,
    retry_at TEXT,
    last_error_code TEXT,
    PRIMARY KEY (github_host, repository_node_id),
    FOREIGN KEY (github_host, repository_node_id)
        REFERENCES repositories (github_host, node_id) ON DELETE CASCADE
) STRICT;
