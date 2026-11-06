# Nodo `krond` dedicado para la web wallet

La web wallet debe usar una instancia de `krond` **exclusiva**. No debe compartir
datadir, wallets ni credenciales RPC con la tesorería, el staking principal o la
wallet `ppal`.

## Puertos

Ejemplo para un servidor que ya tiene otro nodo Kron en los puertos estándar:

| Uso | Puerto |
|---|---:|
| P2P del nodo web | `20892` |
| RPC del nodo web | `20893` |
| P2P estándar (otro nodo) | `20888` |
| RPC estándar (otro nodo) | `20889` |

El nodo web puede usar un puerto P2P no estándar y aun así conectarse a la red.

## Datadir y usuario

```bash
sudo useradd --system --home /var/lib/kron-web --shell /usr/sbin/nologin kronweb
sudo install -d -o kronweb -g kronweb -m 0700 /var/lib/kron-web
sudo install -d -o kronweb -g kronweb -m 0700 /var/lib/kron-web/wallets
sudo install -d -o root -g kronweb -m 0750 /etc/kron
```

## `/etc/kron/kron-web.conf`

```ini
# Red
server=1
listen=1
port=20892
rpcport=20893
rpcbind=127.0.0.1
rpcallowip=127.0.0.1

# Credenciales RPC dedicadas (usar una contraseña larga y aleatoria)
rpcuser=webwallet
rpcpassword=CAMBIAR-POR-UNA-CLAVE-LARGA

# Wallets
disablewallet=0
walletdir=/var/lib/kron-web/wallets
# Las operaciones de contrato/token requieren sender P2PKH.
addresstype=legacy

# Eventos EVM para gettransactionreceipt/searchlogs
logevents=1

# No staking en el nodo web
staking=0

# Opcional: índice de transacciones para consultas arbitrarias.
# Aumenta el uso de disco.
txindex=1

# Límites razonables
maxconnections=64
rpcworkqueue=64
```

## Arranque manual

```bash
sudo -u kronweb /opt/kron/krond -datadir=/var/lib/kron-web -conf=/etc/kron/kron-web.conf
sudo -u kronweb /opt/kron/kron-cli -datadir=/var/lib/kron-web -conf=/etc/kron/kron-web.conf getblockchaininfo
```

Esperar a que `initialblockdownload` sea `false` antes de abrir el registro público.

## systemd

Ver `deploy/krond-web.service`. Ajustar rutas si los binarios no están en `/opt/kron`.

```bash
sudo cp deploy/krond-web.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now krond-web
sudo journalctl -u krond-web -f
```

## Firewall

- Abrir solo `20892/tcp` si se quieren conexiones entrantes P2P.
- **No abrir** `20893/tcp`; el RPC debe quedar en loopback.
- Abrir `80/443` para Caddy.

## Backups

- `backupwallet` para cada wallet `u_*`.
- Copiar `walletdir` con el nodo detenido o usando `backupwallet`.
- Cifrar los backups y probar restauración en un entorno aislado.

## Comprobaciones

```bash
kron-cli -datadir=/var/lib/kron-web -conf=/etc/kron/kron-web.conf getblockchaininfo
kron-cli -datadir=/var/lib/kron-web -conf=/etc/kron/kron-web.conf listwallets
kron-cli -datadir=/var/lib/kron-web -conf=/etc/kron/kron-web.conf getdgpinfo
```

La app crea wallets con `createwallet`, las carga bajo demanda con `loadwallet` y
las respalda con `backupwallet` si `WALLET_BACKUP_DIR` está configurado.
