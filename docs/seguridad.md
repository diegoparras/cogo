# COGO — Seguridad y despliegue

COGO guarda tu memoria de proyecto y expone un servidor MCP. Quien llegue a ese
puerto puede **leer/escribir el vault** y **manejar el MCP** (incluido el Guard,
que gasta tokens del modelo). Este documento es cómo desplegarlo sin exponerlo.

## Los tres modos

| Modo | Cómo se prende | Para qué |
|---|---|---|
| **standalone** | por defecto (sin nada) | local, **solo loopback** |
| **token** | `COGO_MCP_TOKEN=<secreto>` | un VPS / acceso programático (Claude Code) |
| **federado** | `AUTH_MODE=federado` + `LOCKATUS_*` | equipo con SSO (OIDC/Lockatus) |

En **token** y **federado**, `/api/*` y `/mcp` exigen credencial:
- **Bearer token** (`Authorization: Bearer <secreto>`) — para el cliente MCP.
- **cookie de sesión OIDC** — para el navegador (modo federado).

El visor (la web) en modo token te pide el token una vez y lo guarda en el
navegador; después lo manda solo en cada request.

## Fail-safe

COGO **se niega a arrancar** en una interfaz pública (`0.0.0.0` / `:puerto`) si
no hay auth, para que un vault sin protección no termine en internet por
descuido. Te da las tres salidas. Si el puerto ya está detrás de un firewall o
túnel, lo forzás con `COGO_ALLOW_INSECURE=1`.

## Receta para un VPS (nivel banco, capas)

Ninguna capa sola alcanza; la fuerza es apilarlas:

1. **No expongas el puerto.** Bindealo a `127.0.0.1` y llegá por **túnel SSH** o
   **WireGuard/Tailscale**. El `/mcp` nunca ve internet. (La capa más fuerte.)
   ```
   cogo serve -http 127.0.0.1:8080 -vault /srv/cogo/vault
   # y en tu máquina:  ssh -N -L 8080:127.0.0.1:8080 usuario@vps
   ```
2. **O** exponelo detrás de un reverse proxy con **TLS** (Caddy/nginx + Let's
   Encrypt) y un **token**:
   ```
   COGO_MCP_TOKEN="$(openssl rand -hex 32)" COOKIE_SECURE=1 \
     cogo serve -http 127.0.0.1:8080 -vault /srv/cogo/vault
   ```
   En Claude Code (`.mcp.json`), el cliente manda el header:
   ```json
   { "mcpServers": { "cogo": {
       "type": "http", "url": "https://cogo.tu-dominio/mcp",
       "headers": { "Authorization": "Bearer <el-mismo-secreto>" } } } }
   ```
3. **`SECRET_KEY` fija** en federado (si no, es aleatoria y las sesiones se caen
   al reiniciar). **Disco cifrado** en el VPS. **`ANONIMAL` (scrub) prendido**
   para que no queden secretos/PII en las notas.

Ya incluido en COGO: rate limiting por IP, security headers (nosniff, frame
deny, referrer-policy, HSTS bajo TLS), comparación de token en tiempo constante.

## Lo que COGO NO hace (limitaciones honestas)

- **Sin ACL por nota**: en federado, todo usuario autenticado ve **todo** el
  vault. No hay aislamiento por usuario.
- **Vault en texto plano** en disco (markdown). Cifrá el disco del VPS.
- **La API key del modelo** vive en `.cogo/llm.json` en texto plano (gitignoreado).
- El token es un **secreto compartido** (no rota solo). Rotalo cambiando la env.

## Eficiencia de tokens

Vía MCP, **solo la tool `guard` gasta tokens del modelo**. Todo lo demás
(`pack`, `search`, `open`, `capture`, `verify`, `archive`, …) es **determinista
→ 0 tokens**. El `pack` va presupuestado y deduplicado, así tu agente consume el
juicio ya computado en vez de re-derivarlo. El gasto (Guard/lint, opcional y
acotado) lo ves en el menú (**≈ N tokens IA**), persistido en `.cogo/usage.json`.
Para gasto casi nulo: modelo **local (Ollama)** para el Guard; Tier2/steelman
apagado salvo que lo necesites.
