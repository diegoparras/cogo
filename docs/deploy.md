# COGO — Deploy (la guía completa, sin vueltas)

Todo lo que necesitás para poner COGO a andar y mantenerlo: en tu compu, en un
servidor, o para todo un equipo. Está pensado para que **no tengas que adivinar
nada** — cada variable, cada volumen, cada modo, explicado en criollo.

> Si solo querés probarlo en 2 minutos, andá directo a **[§2 Local](#2-local-en-tu-compu-2-minutos)**.
> Si querés el detalle del **modelo de amenaza** y el endurecimiento nivel banco,
> está en **[seguridad.md](seguridad.md)**. Este documento es el operativo completo.

---

## 1. El modelo mental (leé esto primero, son 6 líneas)

- **COGO es un solo binario Go** (imagen Docker `scratch`, ~12 MB) que es **tres
  cosas a la vez**: el **visor web**, el **servidor MCP** (para tus agentes) y el
  **CLI**. No hay base de datos, no hay runtime, no hay dependencias.
- **El vault manda.** Tu memoria es una carpeta de archivos `.md`. Es la **única
  fuente de verdad**: portable, diffeable, sobrevive a que COGO muera. Todo lo
  demás es caché reconstruible. **Persistí esa carpeta y respaldala** (§7).
- **COGO se niega a arrancar público sin auth.** Si lo ponés en `0.0.0.0` sin
  token ni SSO, **no levanta** — a propósito, para que tu vault no termine en
  internet por descuido (§4, §14).

---

## 2. Local (en tu compu, 2 minutos)

### Con Docker (recomendado)

```bash
docker run -d --name cogo -p 127.0.0.1:8095:8080 \
  -v cogo-vault:/vault -e COGO_ALLOW_INSECURE=1 \
  ghcr.io/diegoparras/cogo
```

Abrí **<http://localhost:8095>**. Listo — ves tu vault, capturás y editás notas,
todo desde la web, **cero terminal**.

Qué dice cada parte:

| Parte | Qué hace |
|---|---|
| `-d` | corre en segundo plano |
| `-p 127.0.0.1:8095:8080` | publica **solo en tu máquina** (`8095` afuera → `8080` adentro); nadie de tu red lo ve |
| `-v cogo-vault:/vault` | guarda las notas en un volumen para que **no se borren** |
| `-e COGO_ALLOW_INSECURE=1` | "sé que es local, no me pidas token" — **solo para loopback** |

Apagar: `docker stop cogo`. Prender: `docker start cogo`.

> **Windows + Git Bash:** si el vault sale **vacío**, es que Git Bash convirtió
> `-e COGO_VAULT=/vault` en una ruta de Windows. Corré el comando desde
> **PowerShell**, o prefijalo con `MSYS_NO_PATHCONV=1` (ver [§14](#14-problemas-comunes)).

### Sin Docker (un solo binario)

```bash
cogo serve -http 127.0.0.1:8080 -vault ./vault
```

Un binario estático, sin runtime. `cogo init -vault ./vault` primero si el vault
no existe.

---

## 3. Servidor con EasyPanel (recomendado para usarlo desde cualquier lado)

EasyPanel **no se lleva bien con docker-compose** — **no** uses el tipo "Compose".
Usá el tipo **App**, que es más simple y le pone HTTPS solo.

**Antes:** generá un token largo y guardalo. A esto le decimos **TU-TOKEN**:

```bash
openssl rand -hex 32
```

### 3.1 Crear la app
1. En tu proyecto de EasyPanel → **+ Add Service → App**.
2. Nombre, p. ej. `cogo`.

### 3.2 De dónde sale la imagen (elegí una)
- **Desde el código (la más simple, siempre funciona):** **Source → GitHub**, repo
  `diegoparras/cogo`, branch `main`, **Build → Dockerfile**. EasyPanel construye
  solo. **Usá esta si dudás.**
- **Desde imagen:** **Source → Docker Image**, pegá `ghcr.io/diegoparras/cogo:latest`.
  Requiere que el paquete GHCR esté **público** (GitHub → repo → Packages → `cogo`
  → Package settings → Change visibility → Public).

### 3.3 Variables de entorno
Pestaña **Environment** (cambiá `TU-TOKEN`):

```
COGO_MCP_TOKEN=TU-TOKEN
COOKIE_SECURE=1
```

(`COGO_VAULT=/vault` ya viene por defecto.)

### 3.4 Que las notas NO se borren al actualizar (¡importante!)
Pestaña **Mounts → Add Mount**:
- Tipo: **Volume** · Nombre: `cogo-vault` · Mount path: **`/vault`**

> Sin este paso, **cada actualización te borra las notas.**

### 3.5 Dominio + HTTPS
Pestaña **Domains → Add Domain**: tu dominio, **Port `8080`** (el interno de COGO).
EasyPanel le pone el certificado Let's Encrypt solo.

### 3.6 Deploy y entrar
**Deploy** → esperá el verde → abrí tu dominio. COGO te **pide el token** → pegá
`TU-TOKEN`. Adentro.

---

## 4. VPS a mano

Dos formas, de más a menos segura. La regla es apilar capas (el detalle está en
[seguridad.md](seguridad.md)).

### 4.1 Loopback + túnel (la más segura — el `/mcp` nunca ve internet)

```bash
# en el VPS:
docker run -d --name cogo -p 127.0.0.1:8080:8080 \
  -v cogo-vault:/vault -e COGO_MCP_TOKEN=TU-TOKEN \
  ghcr.io/diegoparras/cogo

# en tu compu (túnel SSH):
ssh -N -L 8080:127.0.0.1:8080 usuario@tu-vps
```

Abrí <http://localhost:8080>. (Con WireGuard/Tailscale es igual de bueno.)

### 4.2 Reverse proxy con TLS + token

Atá COGO a loopback y ponele **Caddy** (o nginx) adelante con Let's Encrypt:

```bash
docker run -d --name cogo -p 127.0.0.1:8080:8080 \
  -v cogo-vault:/vault -e COGO_MCP_TOKEN=TU-TOKEN -e COOKIE_SECURE=1 \
  ghcr.io/diegoparras/cogo
```

`Caddyfile`:

```
cogo.tu-dominio {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy resuelve el HTTPS solo. `COOKIE_SECURE=1` activa cookies seguras + HSTS.

---

## 5. Federado (SSO para un equipo, vía Lockatus)

Para que un equipo entre con **login OIDC** en vez de un token compartido. Requiere
un proveedor OIDC (Lockatus, el de la Suite Escriba).

```bash
AUTH_MODE=federado
LOCKATUS_ISSUER=https://lockatus.tu-dominio      # debe ser alcanzable desde el navegador
LOCKATUS_CLIENT_ID=cogo
LOCKATUS_REDIRECT_URI=https://cogo.tu-dominio/auth/callback
SECRET_KEY=<algo-fijo-y-largo>                   # ¡FIJO! si no, las sesiones se caen al reiniciar
COOKIE_SECURE=1
```

- Registrá la app `cogo` en Lockatus con ese `redirect_uri`.
- **`SECRET_KEY` tiene que ser fija.** Si la dejás vacía, COGO usa una aleatoria por
  arranque y **todas las sesiones se invalidan cada reinicio**.
- **Token y SSO componen.** Poné `AUTH_MODE=federado` **y** `COGO_MCP_TOKEN`
  juntos: los **humanos** entran por Lockatus (SSO) y los **agentes** (Claude Code)
  por **Bearer token**. En la pantalla de login hay además un *"o entrá con un token
  de acceso"*.

> **Honestidad:** en federado, **todo usuario autenticado ve todo el vault** — no
> hay ACL por nota ni aislamiento por usuario. Si necesitás compartimentar, corré
> vaults separados.

---

## 6. Referencia completa de variables de entorno

Ninguna es obligatoria en local. Agrupadas por función.

### Núcleo

| Variable | Default | Qué hace |
|---|---|---|
| `COGO_VAULT` | `/vault` (Docker) · `./vault` (CLI) | carpeta del vault. En Docker ya viene puesta. |
| `COGO_ALLOW_INSECURE` | — | `1` = permití servir sin auth en interfaz pública. **Solo si el puerto ya está firewalleado/túnel.** |
| `COOKIE_SECURE` | — | `1` bajo TLS: cookies `Secure` + header HSTS. Ponelo detrás de HTTPS. |

### Autenticación

| Variable | Default | Qué hace |
|---|---|---|
| `COGO_MCP_TOKEN` | — | token Bearer raíz (bootstrap/break-glass). Prende el **modo token**. |
| `AUTH_MODE` | `standalone` | `federado` prende el login OIDC (Lockatus). |
| `LOCKATUS_ISSUER` | — | URL del proveedor OIDC (debe ser alcanzable desde el **navegador**). |
| `LOCKATUS_CLIENT_ID` | — | id de la app registrada en Lockatus. |
| `LOCKATUS_REDIRECT_URI` | — | `https://tu-cogo/auth/callback`. |
| `SECRET_KEY` | aleatoria | firma la cookie de sesión (federado). **Fijala** o las sesiones se caen al reiniciar. |

### Modelo IA (opcional — detecta contradicciones y potencia el Guard)

| Variable | Default | Qué hace |
|---|---|---|
| `COGO_LLM_BASE_URL` | — | endpoint OpenAI-compatible (OpenRouter, Ollama, DeepSeek…). |
| `COGO_LLM_MODEL` | — | id del modelo (`deepseek/deepseek-chat`, `qwen2.5:7b`…). |
| `COGO_LLM_API_KEY` | — | la key. **Nunca la commitees** — solo por env/secret. |
| `COGO_LLM_REFERER` | — | header `HTTP-Referer` para atribución en OpenRouter (opcional). |
| `COGO_LLM_STRONG_BASE_URL` | — | modelo **fuerte e independiente** para el *steelman* del Guard (Tier 2). |
| `COGO_LLM_STRONG_MODEL` | — | id del modelo fuerte. |
| `COGO_LLM_STRONG_API_KEY` | — | key del modelo fuerte. |
| `COGO_EMBED_MODEL` | — | modelo de **embeddings** (ej. `text-embedding-3-small`) — prende la **búsqueda semántica** del tool `search` (por significado, no por palabra). Reusa el mismo base/key del LLM; cachea en `.cogo/embeddings.json`. Opcional. |

> Podés configurar el modelo IA **desde el visor** (menú ⋮ → *Ajustes · Modelo IA*)
> en vez de env vars: se guarda en `<vault>/.cogo/llm.json` (gitignoreado, texto
> plano — cifrá el disco del VPS).

### Accesorios de la Suite

| Variable | Default | Qué hace |
|---|---|---|
| `ANONIMAL_URL` | — | prende el **scrub** (saca secretos/PII antes de persistir). **Fail-closed.** |
| `ANONIMAL_TOKEN` | — | token del servicio Anonimal. |
| `COGO_EVIDENCE_ROOT` | — | raíz **global** para resolver refs de evidencia relativas (ver §9; en Docker, ruta **del contenedor**). |

---

## 7. Persistencia y respaldo (no pierdas tus notas)

### Qué se guarda dónde
- **`<vault>/*.md`** — tus notas. La verdad. Esto es lo que importa.
- **`<vault>/.cogo/`** — estado local reconstruible/secreto: `tokens.json` (hasheados),
  `llm.json` (config del modelo), `contradictions.json`, `history/`, `trash/`,
  `audit.jsonl`, `usage.json`, `evidence-roots.json`, `mandate.json`.

Por eso el **volumen va montado en `/vault`** (no en una subcarpeta): incluye las
notas *y* el estado.

### Respaldo (tres formas)
1. **Desde el visor** — menú ⋮ → **Exportar (backup)** → baja un
   `cogo-vault-<fecha>.zip` con **todas las notas** (excluye `.cogo`, así el zip es
   portable y **no lleva secretos**).
2. **Por API** — `GET /api/export` (mismo zip; requiere auth).
3. **A mano** — copiá la carpeta del vault, o el volumen Docker:
   ```bash
   docker run --rm -v cogo-vault:/v -v "$PWD:/out" busybox \
     tar czf /out/cogo-backup.tgz -C /v .
   ```

### Restaurar
Descomprimí los `.md` en la carpeta del vault (o en el volumen) y reiniciá COGO.
Como el vault es la fuente de verdad, con eso alcanza — COGO recomputa los colores
al leer.

---

## 8. Tokens por app (menú ⋮ → Conexiones MCP)

Además del `COGO_MCP_TOKEN` raíz, desde el visor emitís **tokens con nombre**, uno
por app/agente:

- Cada token se **revoca solo**, sin tocar los demás.
- Se guardan **hasheados** (sha256) en `.cogo/tokens.json`; el texto plano se
  muestra **una sola vez**.
- Opcionales: **vencimiento** (30/90 días, 1 año) y **solo lectura** (un token
  read-only solo puede `pack`/`search`/`open`; las escrituras se rechazan con 403).
- Al emitir uno, COGO te da la **config lista para el `.mcp.json`** con ese token.

Administrarlos exige estar autenticado (root o SSO); un token read-only no puede.

---

## 9. Raíces de evidencia por proyecto (menú ⋮ → Raíces de evidencia)

COGO puede **verificar que una cita de evidencia apunta a algo real** (un archivo
que existe). Para refs **relativas** (`cmd/main.go`) necesita saber contra qué
carpeta resolverlas. Como cada proyecto vive en su repo, la raíz es **por proyecto**:

- Configuralas en el visor (o en `.cogo/evidence-roots.json`), con un **default
  global** de reserva (o la env `COGO_EVIDENCE_ROOT`).
- Una ref que **no resuelve** (archivo borrado) deja de contar para el color — así
  un verde no se apoya en una cita rota.

> **En Docker**, las rutas son las **del contenedor**. Si querés que COGO vea tus
> repos, montalos (`-v /mis/repos:/repos`) y usá `/repos/...` como raíz.

---

## 10. Conectar tus agentes (MCP)

**Lo más rápido — `cogo install`** cablea el `.mcp.json` por vos (mergea sin pisar
otros servers):

```bash
cogo install                                   # stdio local (este binario + ./vault)
cogo install --http https://tu-dominio/mcp --token TU-TOKEN --claude   # remoto + CLAUDE.md
```

**Local (stdio), a mano** — sin red, cada sesión levanta su COGO:

```json
{ "mcpServers": { "cogo": { "command": "cogo", "args": ["serve", "-vault", "./vault"] } } }
```

**Remoto (HTTP + Bearer)**:

```json
{ "mcpServers": { "cogo": {
    "type": "http",
    "url": "https://tu-dominio/mcp",
    "headers": { "Authorization": "Bearer TU-TOKEN" } } } }
```

### El archivo que le enseña el protocolo al agente (AGENTS.md / CLAUDE.md)

Que el agente esté conectado no significa que sepa **usar** COGO. Generá el archivo
bootstrap que se lo explica (consultar antes de actuar, obedecer el color, capturar
lo verificado):

- **Desde el visor:** menú ⋮ → **Instrucciones para agentes** → elegí `AGENTS.md`
  o `CLAUDE.md`, con o sin instantánea de la memoria → **copiar** o **descargar**.
- **Desde el CLI:** `cogo agents --claude --http https://tu-dominio/mcp > CLAUDE.md`

Poné ese archivo en la raíz de tu repo y el agente arranca sabiendo el protocolo.

---

## 11. Modelo IA (opcional)

COGO anda **perfecto sin modelo** — es 100% determinista. El modelo solo suma:
detectar **contradicciones** entre notas (aparecen rojas en el visor **y en el MCP**)
y los **tiers opcionales del Guard**.

- Prendelo por env (§6) o desde *Ajustes · Modelo IA* en el visor.
- **Gasto casi nulo:** modelo **local (Ollama)** para el Guard; el *steelman* (Tier 2)
  apagado salvo que lo pidas. Vía MCP, **solo `guard` gasta tokens**; todo lo demás
  es determinista (0 tokens). El gasto lo ves en el menú (**≈ N tokens IA**).

---

## 12. Salud, logs y actualización

- **Health check:** `GET /healthz` → `ok`. Usalo en EasyPanel/Kubernetes/uptime.
- **Logs:** `docker logs -f cogo`. Al arrancar dice el modo de auth y el vault.
- **Actualizar:**
  - **EasyPanel:** botón **Deploy** / **Rebuild**. Las notas quedan (volumen).
  - **Docker a mano:** `docker pull ghcr.io/diegoparras/cogo` y recreá el
    contenedor con el **mismo** `-v cogo-vault:/vault`.

---

## 13. Auditoría (quién llamó a qué)

COGO deja una traza **append-only** de cada llamada MCP y cada escritura por API en
`.cogo/audit.jsonl`: **quién** (`root` / `user:<email>` / `token:<label>`), qué
herramienta, cuándo y desde qué IP. La ves en el menú ⋮ → **Auditoría MCP** (o `GET
/api/audit`; es admin-only). Sirve para saber qué agente tocó qué.

---

## 14. Problemas comunes

| Síntoma | Causa y solución |
|---|---|
| **La app no arranca / crashea en el server** | Te falta `COGO_MCP_TOKEN` (o SSO). COGO **se niega** a estar público sin auth. Ponelo y redeployá. |
| **Perdí las notas al actualizar** | No montaste el volumen en **`/vault`** (§3.4). |
| **En Docker el vault sale vacío (Windows/Git Bash)** | Git Bash mangeó `-e COGO_VAULT=/vault` a una ruta de Windows. Usá **PowerShell**, o prefijá el comando con **`MSYS_NO_PATHCONV=1`**. El `-v "...:/vault"` no se rompe, solo el `-e`. |
| **Las sesiones se caen cada reinicio (federado)** | `SECRET_KEY` no está fija (§5). Ponele un valor fijo y largo. |
| **El visor me pide un token** | Es el esperado en modo token. Pegá el `COGO_MCP_TOKEN` (o uno emitido). |
| **El login OIDC no vuelve / falla** | `LOCKATUS_ISSUER` debe ser alcanzable **desde el navegador** (no `host.docker.internal`), y el `redirect_uri` debe coincidir con el registrado. |
| **EasyPanel pide health check** | Path: `/healthz`. |

---

## 15. Endurecimiento (nivel banco)

Las capas, el modelo de amenaza y lo que COGO **no** hace (limitaciones honestas)
están en **[seguridad.md](seguridad.md)**. En resumen: no expongas el puerto (túnel
o proxy TLS), token o SSO siempre, `SECRET_KEY` fija, disco cifrado, y el scrub
(`ANONIMAL_URL`) prendido para que no queden secretos en las notas. Ya vienen de
fábrica: rate-limit por IP, security headers, y comparación de token en tiempo
constante.

---

## 16. Verificar autenticidad (cadena de suministro)

Cada imagen publicada lleva **procedencia SLSA** + una **firma keyless de Sigstore
(cosign)**, así que podés probar que salió del CI de este repo y no de otro lado.

**La imagen** (firma + procedencia):
```bash
IMG=ghcr.io/diegoparras/cogo
cosign verify "$IMG" \
  --certificate-identity-regexp "https://github.com/diegoparras/cogo/.github/workflows/.+" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://$IMG --repo diegoparras/cogo   # procedencia SLSA
```

**Los binarios** (en las releases por tag): cada release trae `SHA256SUMS` +
`SHA256SUMS.sig` + `SHA256SUMS.pem`. Verificás el checksum firmado y después el
binario:
```bash
cosign verify-blob --signature SHA256SUMS.sig --certificate SHA256SUMS.pem \
  --certificate-identity-regexp "https://github.com/diegoparras/cogo/.+" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com SHA256SUMS
sha256sum -c SHA256SUMS
```

> Esto es la respuesta honesta al falso positivo de antivirus: el binario no está
> firmado con un cert de Windows/Apple (cuesta plata), pero **sí** con Sigstore,
> que es verificable por cualquiera y prueba el origen.
