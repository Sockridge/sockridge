CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS skill_embeddings (
    agent_id   TEXT        NOT NULL,
    skill_id   TEXT        NOT NULL,
    embedding  vector(384),
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (agent_id, skill_id)
);

-- use exact search for now (ivfflat needs 1000s of rows to be effective)
-- switch to ivfflat index when you have > 1000 agents:
-- CREATE INDEX ON skill_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);
