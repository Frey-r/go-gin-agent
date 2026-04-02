-- ============================================================
-- 002_prompts_and_agents.sql — Prompt management + Agent definitions
-- ============================================================

-- Prompts gestionados externamente
-- Cada prompt tiene un prompt_id (slug semántico), pertenece a una organization,
-- y soporta versionamiento (solo la versión activa se usa en runtime).
CREATE TABLE IF NOT EXISTS prompts (
    id TEXT PRIMARY KEY,                                          -- UUID v4
    prompt_id TEXT NOT NULL,                                      -- slug semántico: "sales-assistant", "code-reviewer"
    organization TEXT NOT NULL,                                   -- tenant/org que posee el prompt
    content TEXT NOT NULL,                                        -- el system prompt completo
    version INTEGER NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT 1,                        -- solo 1 activo por prompt_id+org
    metadata TEXT,                                                -- JSON libre: modelo preferido, temperatura, etc.
    created_by TEXT REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_prompts_active
    ON prompts(prompt_id, organization) WHERE is_active = 1;
CREATE INDEX IF NOT EXISTS idx_prompts_org ON prompts(organization);
CREATE INDEX IF NOT EXISTS idx_prompts_slug ON prompts(prompt_id);

-- Definiciones de agentes
-- Un agente es una configuración que combina: prompt, modelo, herramientas, y sub-agentes.
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,                                          -- UUID v4
    agent_id TEXT NOT NULL,                                       -- slug: "orchestrator", "researcher", "coder"
    organization TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    prompt_id TEXT NOT NULL,                                      -- referencia al prompt que usa
    model TEXT NOT NULL DEFAULT 'gemini-2.5-pro',                -- LLM model a usar
    max_iterations INTEGER NOT NULL DEFAULT 10,                  -- máx loops de tool-calling
    tools TEXT,                                                   -- JSON array de tool names habilitados
    sub_agents TEXT,                                              -- JSON array de agent_ids que puede invocar
    is_active BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_active
    ON agents(agent_id, organization) WHERE is_active = 1;
CREATE INDEX IF NOT EXISTS idx_agents_org ON agents(organization);
