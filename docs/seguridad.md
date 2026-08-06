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

**Las dos formas componen.** Poné `AUTH_MODE=federado` + `LOCKATUS_*` **y**
`COGO_MCP_TOKEN` juntos: los humanos entran por **Lockatus** (SSO) y los agentes
(Claude Code) por **Bearer token** — cualquiera de las dos autoriza. En el visor,
la pantalla de Lockatus ofrece además un **"o entrá con un token de acceso"**.

El visor en modo token te pide el token una vez y lo guarda en el navegador;
después lo manda solo en cada request.

### Tokens por app (menú ⋮ → Conexiones MCP)

Además del `COGO_MCP_TOKEN` raíz (bootstrap/break-glass), desde el visor emitís
**tokens con nombre**, uno por app/agente (Claude Code, Cursor, un bot de CI…):

- Cada token se **revoca solo**, sin afectar a los demás.
- Se guardan **hasheados** (sha256) en `<vault>/.cogo/tokens.json` — el texto
  plano se muestra **una sola vez** y nunca se persiste.
- Opcionales por token: **vencimiento** (30/90 días, 1 año) y **solo lectura**
  (un token read-only solo puede `pack`/`search`/`open`; `capture`/`verify`/
  `archive`/`remove` se rechazan con 403, tanto por `/api` como por `/mcp`).

Necesitás estar autenticado (root o OIDC) para administrarlos; un token de solo
lectura no puede gestionar tokens.

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

---

## El runner: el modelo de amenaza más grande de COGO

Ejecutar comandos que salen de notas escritas por un LLM es la superficie de ataque
más grande que COGO puede tener. Y **una lista de comandos permitidos no la acota**.

Si el agente escribe la nota *y* el comando, y `go test` está en la lista, entonces
el agente hace que `go test` corra código que él mismo escribió — porque `go test`
ejecuta `TestMain` e `init()`. Lo mismo con `npm test`, que corre lo que diga el
`package.json`.

Por eso **el comando no sale de la nota**. Los declara una persona en
`.cogo/runner.yaml`, con su directorio y su timeout; la nota solo **elige** cuál de
esos checks le aplica, por nombre.

```yaml
enabled: false          # viene apagado: un COGO recién instalado no ejecuta nada
checks:
  - id: go-test
    command: ["go", "test", "./..."]   # argv, no una línea de shell: sin shell no
    workdir: /ruta/absoluta/al/repo    # hay expansión, ni tuberías, ni ";" para
    timeout: 2m                        # encadenar otra cosa
```

Un agente puede pedir que se corra `go-test`. **No puede inventar qué se ejecuta.**

Y el emisor que produce una verificación está reservado: `internal_runner` no se
puede escribir por la puerta común del registro, y una sola función lo emite. Un
`grep` de `AppendEjecucion` muestra todos los lugares del sistema capaces de
producir un `verified`.

## Dos procesos sobre el mismo vault

El número de secuencia del registro y su encadenado viven **en memoria** de cada
proceso. Con dos COGO sobre el mismo volumen, los dos creerían que el último evento
es el N y los dos escribirían el N+1: dos eventos con el mismo número y dos ramas
de la cadena de hashes.

No es hipotético — un despliegue rodante levanta el contenedor nuevo antes de bajar
el viejo. Está cubierto: escribir toma un cerrojo del sistema operativo (`flock` en
Unix, `LockFileEx` en Windows) y, con él, relee la punta del registro del disco.

Se eligió un cerrojo del kernel y no un archivo centinela porque el kernel lo
libera cuando el proceso muere, muera como muera. Un centinela dejado por un
proceso que se cayó trabaría todas las escrituras futuras para siempre.

**Lo que no cubre:** dos máquinas distintas contra un NFS compartido. Ahí los
cerrojos de red no son confiables. Si necesitás varias instancias, dales un vault a
cada una y federalas.

## El olvido no borra

Una nota que sale de circulación por falta de uso (§ el manual) **sigue en disco**.
No hay ninguna ruta de COGO que borre una nota sin que alguien lo pida
explícitamente con `remove`, y esa deja lápida en el log.
