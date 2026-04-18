# Prospección Agéntica 🦀

## Estructura
- **raw/**: JSON crudos de MCPs (ej: jobs.json).
- **leads/**: Leads calificados por Grok (JSON).
- **scripts/**: Subagentes y utils.

## Flujo
1. MCP → raw/empresa-YYYYMMDD.json
2. Cron → spawn Grok subagent → leads/
3. Telegram notif con ficha lead.