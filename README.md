# Kron Wallet Web

Billetera web multiusuario, mobile-first (PWA), para **Kron Smart Chain (KSC)**.

- Backend: Go + Gin + GORM + SQLite3.
- Frontend: React + Vite + TypeScript + PWA.
- Custodia: una wallet cifrada por usuario dentro de un `krond` dedicado.
- Auth: usuario/contraseña + TOTP opcional + códigos de recuperación.
- Tokens: KRC20 (alias de QRC20).
- Contratos: leer, escribir y desplegar EVM.
- Sin staking.

> **Advertencia de custodia:** el servidor administra las claves. Si se compromete el
> backend, la master key o el nodo, los fondos de los usuarios están en riesgo. Esto
> no es una wallet no custodial. Para producción se requiere hardening, backups,
> auditoría y revisión legal.

## Funcionalidades

- Registro abierto con límites por IP, tier y límites de gasto.
- Login con cookie de sesión, CSRF, rate limiting y bloqueo por intentos.
- 2FA opcional y deshabilitado por defecto; se activa desde Seguridad.
- Códigos de recuperación y reset por administrador.
- Dashboard con saldo total, saldo pendiente, dirección principal y distribución por dirección.
- Envíos KSC que usan UTXOs de todas las direcciones y devuelven el cambio a la dirección principal.
- Consolidación de KSC en la dirección principal con `sendall` y botón “Preparar gas” para tokens.
- Enviar/recibir KSC con QR `kron:`.
- Historial de transacciones y detalle.
- Agregar tokens KRC20 por contrato, portfolio, transfer, approve, transferFrom, burn, burnFrom. El backend usa `qrc20*` canónico y cae a `krc20*` si el nodo solo tiene los alias de marca.
- Contratos: importar ABI, llamadas de lectura, escritura, deploy, recibos, logs y bytecode.
- Address book.
- API keys por usuario: máximo 3 activas, expiración configurable, 30 llamadas/min por key, acceso total a la wallet.
- Documentación OpenAPI 3.1 y `llms.txt` para integraciones y modelos.
- Panel admin: usuarios, estados, límites, reset de password/2FA, tokens verificados, auditoría y salud del nodo.
- PWA instalable con service worker, íconos para Android/iOS y aviso de instalación en móviles.

## Arquitectura

```text
PWA React
   │ HTTPS
   ▼
Caddy / reverse proxy
   │
   ▼
API Go (Gin) ─────► SQLite (WAL)
   │
   │ JSON-RPC 127.0.0.1
   ▼
krond dedicado (wallets u_*)
```

- El frontend nunca habla con `krond`.
- El nodo web corre con `-staking=0` y RPC solo en localhost.
- Cada usuario tiene una wallet `u_<uuid>` y una passphrase aleatoria cifrada con AES-256-GCM.
- La master key no vive en SQLite.

## Requisitos

- Go 1.26+.
- Node.js 22+ y npm (solo para construir el frontend).
- `krond`/`kron-cli` compilados desde este repo o binarios oficiales.
- Un nodo Kron Smart Chain dedicado y sincronizado.

## Desarrollo rápido

```bash
cd webwallet

# 1. Frontend
cd web
npm install
npm run build
cd ..

# 2. Backend
cp settings.example.json settings.json
# editar settings.json: RPC, master key, admin
go run ./cmd/server -config settings.json
```

Abrir `http://localhost:8080`.

Para desarrollo con recarga del frontend:

```bash
# terminal 1
cd web && npm run dev

# terminal 2
cd webwallet && go run ./cmd/server -config settings.json
```

Vite proxya `/api`, `/healthz` y `/metrics` a `127.0.0.1:8080`.

## Build de producción

```bash
cd webwallet
make build
# bin/webwallet
```

El build de Vite escribe en `internal/webui/dist` y queda embebido en el binario Go.
El binario es estático, no necesita Node, Docker ni el directorio `web/` en runtime.

Para compilar para VPS Linux desde otra máquina:

```bash
make build-linux
# dist/webwallet-linux-amd64
# dist/webwallet-linux-arm64
```

## Despliegue directo en VPS

No hace falta Docker. Solo el binario, `settings.json` y el nodo `krond`.

```bash
# En el VPS
sudo useradd --system --home /opt/kron-wallet --shell /usr/sbin/nologin kronwallet
sudo install -d -o kronwallet -g kronwallet -m 0700 /opt/kron-wallet
sudo install -d -o kronwallet -g kronwallet -m 0700 /etc/kron-wallet
sudo install -d -o kronwallet -g kronwallet -m 0700 /var/lib/kron-wallet

sudo install -o kronwallet -g kronwallet -m 0755 dist/webwallet-linux-amd64 /opt/kron-wallet/webwallet
sudo install -o kronwallet -g kronwallet -m 0600 settings.example.json /etc/kron-wallet/settings.json
sudoedit /etc/kron-wallet/settings.json
```

Ajustar en `settings.json`:

```json
{
  "environment": "production",
  "http_addr": ":8080",
  "data_dir": "/var/lib/kron-wallet",
  "db_path": "/var/lib/kron-wallet/webwallet.db",
  "cookie_secure": true
}
```

Luego:

```bash
sudo /opt/kron-wallet/webwallet -config /etc/kron-wallet/settings.json
```

Para dejarlo como servicio, usar `deploy/kron-wallet.service`:

```bash
sudo cp deploy/kron-wallet.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now kron-wallet
```

Para HTTPS, poner Caddy o Nginx delante de `127.0.0.1:8080`. El
`deploy/Caddyfile` incluido es opcional.

## Configuración

Toda la configuración vive en `settings.json`. No se usan variables de entorno
para los parámetros de la aplicación.

```bash
cp settings.example.json settings.json
chmod 600 settings.json
```

Si el archivo no existe, la aplicación crea uno con valores por defecto y una
master key aleatoria. En producción conviene generarlo/editarlo antes de arrancar.

```bash
./bin/webwallet -config /etc/kron-wallet/settings.json
```

Claves principales:

| Clave JSON | Descripción |
|---|---|
| `http_addr` | Puerto/escucha, por ejemplo `:8080`. |
| `public_url` | URL pública detrás del reverse proxy. |
| `data_dir` / `db_path` | Datos y SQLite. |
| `rpc_url` | URL del `krond` dedicado. |
| `rpc_user` / `rpc_password` | Credenciales RPC. |
| `master_key` | Clave de 32 bytes en base64. Si está vacía, se genera y se guarda. |
| `master_key_file` | Alternativa opcional para leer la clave desde un archivo. |
| `trusted_proxies` | IPs del reverse proxy que pueden enviar `X-Forwarded-For`. |
| `cookie_secure` | `true` en producción con HTTPS. |
| `session_ttl` | Duración de sesión, por ejemplo `12h`. |
| `mfa_reverify_after` | Ventana para reutilizar 2FA en operaciones sensibles. |
| `registration_enabled` | Abrir/cerrar registro. |
| `require_totp` | Reservado; 2FA se controla por usuario desde Seguridad. |
| `default_per_tx_limit` / `default_daily_limit` | Límites en KSC decimal; sufijo `sat` para satoshis. |
| `max_accounts_per_ip` | Cuentas por IP en 24 h. |
| `admin_username` / `admin_password` | Bootstrap inicial del admin. |
| `metrics_token` | Si se define, `/metrics` exige `Authorization: Bearer`. |

Generar master key manualmente:

```bash
openssl rand -base64 32
```

Protegé `settings.json` como un secreto: contiene la master key, la contraseña
RPC y, si se configura, la contraseña del admin.

Para systemd, usá algo como:

```json
{
  "environment": "production",
  "http_addr": ":8080",
  "data_dir": "/var/lib/kron-wallet",
  "db_path": "/var/lib/kron-wallet/webwallet.db",
  "cookie_secure": true
}
```

## Nodo dedicado

Ver [`docs/krond-web.md`](docs/krond-web.md). Resumen:

- Datadir separado, por ejemplo `/var/lib/kron-web`.
- Puertos propios (ej. P2P `20892`, RPC `20893`).
- `server=1`, `rpcbind=127.0.0.1`, `rpcallowip=127.0.0.1`.
- `logevents=1` para recibos/logs.
- `staking=0`.
- No reutilizar la wallet `ppal` ni la tesorería.

## API

Todas las rutas cuelgan de `/api/v1` y usan cookies de sesión + `X-CSRF-Token`.

- `GET /auth/csrf`, `/auth/session`
- `POST /auth/register`, `/auth/login`, `/auth/logout`
- `GET /auth/me`, `/auth/sessions`
- `POST /auth/totp/setup`, `/auth/totp/verify`, `/auth/totp/disable`, `/auth/recovery`
- `POST /auth/password`, `/auth/recovery-codes/regenerate`
- `GET /wallet`, `/wallet/address`, `/wallet/transactions`, `/wallet/fee-estimate`
- `POST /wallet/send` (requiere `Idempotency-Key`)
- `POST /wallet/consolidate` (mueve todo el KSC a la dirección principal)
- `POST /wallet/prepare-gas` (envía KSC de gas a una dirección de origen)
- `GET/POST /tokens`; `GET/DELETE /tokens/:contract`
- `POST /tokens/:contract/transfer|approve|transfer-from|burn|burn-from`
- `GET/POST /contracts`; `GET/DELETE /contracts/:address`
- `POST /contracts/read|write|deploy`
- `GET /contracts/:address/receipts|logs|code`
- `GET/POST/DELETE /address-book`
- `GET/POST/DELETE /api-keys` (solo sesión web; no API key)
- `GET /api/v1/openapi.json` y `/llms.txt` (públicos)
- `GET/POST /api/v1/app/*` (API key, acceso total a wallet/tokens/contratos/address book)

Las operaciones de escritura requieren `Idempotency-Key`; si el usuario habilitó 2FA, también requieren 2FA reciente.
Las API keys usan `Authorization: Bearer ksc_...` o `X-API-Key`, no usan 2FA, y tienen un límite de 30 requests por minuto.

## Seguridad operativa

- Usar HTTPS con Caddy/Nginx. `cookie_secure=true` en `settings.json`.
- Mantener `krond` y la app en el mismo host o en una red privada.
- No exponer el RPC del nodo a internet.
- Guardar la master key en un KMS/secret manager; no en el repositorio.
- Disco cifrado (LUKS) para SQLite y wallets.
- Backups:
  - SQLite: `sqlite3 webwallet.db "VACUUM INTO '/backup/webwallet-$(date +%F).db'"`.
  - Wallets: `backupwallet` por wallet o copia de `wallets/` con el nodo detenido.
  - Cifrar y copiar fuera del servidor; probar restauración.
- Rotar credenciales RPC y master key con un plan de migración.
- Monitorear `/healthz`, `/metrics`, logs y alertas de gastos.
- Revisar legalmente el modelo custodial (KYC/AML según jurisdicción).

## Tests

```bash
cd webwallet
go test ./...
go vet ./...
```

El test de integración usa un RPC falso y SQLite temporal; no toca mainnet.

## Limitaciones

- SQLite y una sola instancia del backend; para escalar, migrar a PostgreSQL.
- No incluye staking.
- KRC20 es QRC20 con branding; no hay KRC721.
- Los contratos importados por usuarios no están auditados.
- La red es joven; reorgs y disponibilidad son riesgos reales.
- Sin email: la recuperación depende del administrador y de los códigos.

## Licencia

MIT, igual que el resto del proyecto.
