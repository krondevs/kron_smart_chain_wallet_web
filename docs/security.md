# Modelo de seguridad

## Activos

- Claves privadas de las wallets `u_*` (custodiadas por el nodo).
- Passphrases de wallet cifradas en SQLite.
- Master key de la aplicación.
- Contraseñas, secretos TOTP y códigos de recuperación.
- Sesiones y cookies.
- Capacidad de firmar y transmitir transacciones.

## Límites de confianza

- El backend es confiable: puede desbloquear cualquier wallet y firmar.
- El nodo `krond` dedicado es confiable y solo acepta RPC desde loopback.
- El navegador del usuario no es confiable para custodiar claves; solo
  transporta la sesión y las órdenes.
- Los contratos y tokens de terceros no son confiables.

## Amenazas principales

| Amenaza | Mitigación |
|---|---|
| Robo de contraseña | Argon2id, rate limit, bloqueo, TOTP, sesiones revocables |
| XSS en la PWA | CSP estricta, sin scripts de terceros, sanitización de metadatos |
| CSRF | Cookie `SameSite=Strict` + token `X-CSRF-Token` |
| Robo de sesión | Cookie `HttpOnly`/`Secure`, token opaco hasheado, rotación y revocación |
| Fuerza bruta | Rate limit por IP/ruta y bloqueo temporal |
| DB comprometida | Passphrases y TOTP cifrados; master key fuera de la DB |
| Master key comprometida | Rotación planificada, KMS/HSM, acceso restringido |
| Backend comprometido | Es el peor caso custodial: límites, auditoría, alertas, aislamiento de red |
| Nodo comprometido | Datadir dedicado, usuario OS separado, firewall, sin staking |
| Doble gasto por reintento | `Idempotency-Key` y bloqueo por wallet |
| Reorg | Confirmaciones, estados pendientes y polling |
| Token malicioso | Metadatos no ejecutables, verificación manual, límites de gas |
| Contrato malicioso | Advertencias, confirmación explícita, límites de gas, sin auto-firma |

## Controles obligatorios en producción

1. HTTPS con HSTS.
2. `cookie_secure: true` en `settings.json`.
3. RPC del nodo solo en `127.0.0.1`.
4. `settings.json` con permisos `0600`; master key fuera del repositorio y, si es posible, en KMS/secret manager.
5. Disco cifrado (LUKS) para SQLite y wallets.
6. Backups cifrados y prueba de restauración.
7. 2FA recomendado (opcional) para operaciones sensibles; si está activo, exigir código y reautenticación.
8. Límites por transacción, diarios y por IP.
9. Revisión manual para montos altos.
10. Monitoreo de `/healthz`, `/metrics`, errores y auditoría.
11. Actualizaciones de dependencias y escaneo de vulnerabilidades.
12. Revisión legal del modelo custodial.

## Respuesta a incidentes

1. Activar modo mantenimiento / deshabilitar registro y envíos.
2. Revocar sesiones y rotar credenciales RPC.
3. Si la master key se comprometió, asumir compromiso de todas las wallets:
   detener el nodo, preservar evidencia y migrar fondos a wallets nuevas.
4. Si el backend se comprometió, aislar el host, rotar secretos y revisar
   `audit_events` y logs del nodo.
5. Comunicar a los usuarios y publicar un post-mortem.
