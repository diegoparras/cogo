# COGO — Instalación (paso a paso, sin vueltas)

Al final vas a tener **COGO corriendo en tu navegador** (el visor) y, si querés,
**conectado a Claude Code** (el MCP). Elegí tu caso:

- **[A. En tu compu](#a-en-tu-compu-local)** — para probarlo, 2 minutos.
- **[B. En un servidor con EasyPanel](#b-en-un-servidor-con-easypanel-recomendado)** — para usarlo desde cualquier lado. **Recomendado.**
- **[C. En un VPS a mano](#c-en-un-vps-a-mano)** — si no usás EasyPanel.

> **Regla de oro de seguridad:** en un servidor, COGO **se niega a arrancar sin
> un token** (para que nadie entre a tu vault). Así que para B y C vas a necesitar
> uno. Generá uno largo y guardalo:
>
> ```bash
> openssl rand -hex 32
> ```
> (o inventá una clave larga, tipo 40+ caracteres al azar). A eso le vamos a
> llamar **TU-TOKEN**.

---

## A. En tu compu (local)

Necesitás **Docker Desktop** instalado. Copiá y pegá esto en una terminal:

```bash
docker run -d --name cogo -p 127.0.0.1:8095:8080 \
  -v cogo-vault:/vault -e COGO_ALLOW_INSECURE=1 \
  ghcr.io/diegoparras/cogo
```

Abrí **<http://localhost:8095>**. Listo.

Qué dice cada parte, en criollo:
- `-d` → corre en segundo plano.
- `-p 127.0.0.1:8095:8080` → lo publica **solo en tu máquina** (nadie de tu red lo ve).
- `-v cogo-vault:/vault` → guarda tus notas en un volumen para que **no se borren**.
- `-e COGO_ALLOW_INSECURE=1` → "sé que es local, no me pidas token". **Solo para local.**

Para apagarlo: `docker stop cogo`. Para prenderlo: `docker start cogo`.

---

## B. En un servidor con EasyPanel (recomendado)

EasyPanel **no se lleva bien con docker-compose** — así que **no** uses el tipo
"Compose". Usá el tipo **App**, que es más simple y le pone HTTPS solo.

### 1. Crear la app
1. En tu proyecto de EasyPanel, tocá **+ Add Service → App**.
2. Ponele un nombre, p. ej. `cogo`.

### 2. De dónde sale la imagen (elegí una)
- **Opción fácil — desde imagen** (cuando esté publicada): en **Source** elegí
  **Docker Image** y pegá `ghcr.io/diegoparras/cogo:latest`.
- **Opción desde el código** (siempre funciona): en **Source** elegí **GitHub**,
  repo `diegoparras/cogo`, branch `main`, y en **Build** elegí **Dockerfile**.

### 3. Variables de entorno
En la pestaña **Environment**, pegá esto (cambiá `TU-TOKEN` por el tuyo):

```
COGO_MCP_TOKEN=TU-TOKEN
COOKIE_SECURE=1
```

(`COGO_VAULT=/vault` ya viene puesto por defecto — no hace falta tocarlo.)

### 4. Que las notas NO se borren al actualizar (importante)
En la pestaña **Mounts** → **Add Mount**:
- Tipo: **Volume**
- Nombre: `cogo-vault`
- Mount path: `/vault`

> Si te salteás este paso, **cada vez que actualices se te borran las notas.**

### 5. Dominio + HTTPS
En la pestaña **Domains** → **Add Domain**:
- Poné tu dominio (o el que te ofrece EasyPanel).
- **Port: `8080`** (el puerto interno de COGO).
- EasyPanel le pone el **certificado HTTPS solo** (Let's Encrypt).

### 6. Deploy
Tocá **Deploy**. Esperá a que quede en verde.

### 7. Entrar
Abrí tu dominio en el navegador. COGO te va a **pedir el token** → pegá el mismo
`TU-TOKEN`. Y ya estás adentro.

### Si algo sale mal
- **La app no arranca / crashea** → te faltó `COGO_MCP_TOKEN`. COGO **a propósito**
  se niega a estar en internet sin protección. Ponelo y volvé a deployar.
- **El visor me pide un token** → es el de `COGO_MCP_TOKEN`. Es lo esperado.
- **Perdí mis notas al actualizar** → no montaste el volumen en `/vault` (paso 4).
- **Health check** (opcional): si EasyPanel lo pide, usá el path `/healthz`.

---

## C. En un VPS a mano

La forma más segura (nivel banco): **no expongas el puerto**, atalo a loopback y
llegá por un túnel.

```bash
# en el VPS:
docker run -d --name cogo -p 127.0.0.1:8080:8080 \
  -v cogo-vault:/vault -e COGO_MCP_TOKEN=TU-TOKEN \
  ghcr.io/diegoparras/cogo

# en tu compu (túnel SSH):
ssh -N -L 8080:127.0.0.1:8080 usuario@tu-vps
```

Abrí <http://localhost:8080>. El detalle de despliegue seguro (túnel/VPN, TLS por
proxy, límites) está en **[docs/seguridad.md](seguridad.md)**.

---

## Conectar Claude Code (el MCP)

**Local (caso A), lo más simple** — sin red, cada sesión levanta su COGO:

```json
{ "mcpServers": { "cogo": { "command": "cogo", "args": ["serve", "-vault", "./vault"] } } }
```

**Remoto (casos B y C)** — por HTTP, con tu token en el header:

```json
{
  "mcpServers": {
    "cogo": {
      "type": "http",
      "url": "https://tu-dominio/mcp",
      "headers": { "Authorization": "Bearer TU-TOKEN" }
    }
  }
}
```

Desde ahí, en cualquier sesión le decís *"buscá en COGO…"*, *"guardá en COGO…"* y
listo. Lo que aprende Claude hoy, mañana lo lee Cursor: **el mismo vault.**

---

## Actualizar COGO

- **EasyPanel:** tocá **Deploy** (o **Rebuild**) de nuevo. Tus notas quedan (por el
  volumen en `/vault`).
- **Docker a mano:** `docker pull ghcr.io/diegoparras/cogo` y recreá el contenedor
  con el mismo `-v cogo-vault:/vault`.

## Modelo de IA (opcional)
COGO anda perfecto sin modelo. Si querés que detecte **contradicciones** entre
notas y active el **Guard** anti-manipulación, entrá al visor → menú (⋮) →
**Ajustes · Modelo IA** y conectá un modelo local (Ollama) o remoto (OpenRouter,
DeepSeek…). Los tokens que gaste los ves en el mismo menú (**≈ N tokens IA**).
