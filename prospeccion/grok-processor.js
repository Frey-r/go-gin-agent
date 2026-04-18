// Subagente Grok: procesa raw → lead JSON
// Uso: sessions_spawn con este como task, attach raw file.

prompt = `
Analiza este data raw de empleos/empresas:
{{raw_data}}

Identifica:
- Empresa/target
- Puntos de dolor (tech gaps, hiring pains)
- Oportunidad B2B (tu servicio/producto)
- Score (1-10)
- Mensaje cold inicial

Output SOLO JSON:
{
  "empresa": "...",
  "pain_points": [...],
  "oportunidad": "...",
  "score": 8,
  "mensaje_cold": "...",
  "contacto": {...}
}
`;

module.exports = prompt;