import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api, newIdempotencyKey, type Contract } from '../lib/api';
import { ErrorBanner, Loading } from '../components/common';
import { Modal } from '../components/Modal';
import { useToast } from '../lib/toast';
import { shorten } from '../lib/format';

const ABI_PLACEHOLDER = `[
  {
    "type": "function",
    "name": "balanceOf",
    "stateMutability": "view",
    "inputs": [{"name":"owner","type":"address"}],
    "outputs": [{"name":"","type":"uint256"}]
  }
]`;

export function ContractsPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const [contracts, setContracts] = useState<Contract[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [deployOpen, setDeployOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const [address, setAddress] = useState('');
  const [name, setName] = useState('');
  const [abi, setAbi] = useState('');
  const [bytecode, setBytecode] = useState('');
  const [constructorArgs, setConstructorArgs] = useState('[]');
  const [gasLimit, setGasLimit] = useState('');
  const [gasPrice, setGasPrice] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const data = await api.get<{ contracts: Contract[] }>('/api/v1/contracts');
      setContracts(data.contracts ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudieron cargar los contratos.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const importContract = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.post('/api/v1/contracts', { address, name, abi });
      toast.push('Contrato importado', 'success');
      setImportOpen(false);
      setAddress('');
      setName('');
      setAbi('');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo importar el contrato.');
    } finally {
      setBusy(false);
    }
  };

  const deploy = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      let args: unknown[] = [];
      if (constructorArgs.trim()) args = JSON.parse(constructorArgs);
      const result = await api.post<{ address: string; txid: string }>('/api/v1/contracts/deploy', {
        name,
        abi,
        bytecode,
        constructor_args: args,
        gaslimit: gasLimit,
        gasprice: gasPrice,
      }, newIdempotencyKey());
      toast.push(`Contrato desplegado: ${result.address}`, 'success');
      setDeployOpen(false);
      setBytecode('');
      setName('');
      setAbi('');
      setConstructorArgs('[]');
      await load();
      navigate(`/contracts/${result.address}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo desplegar el contrato.');
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Contratos</h2>
          <p>Importá, leé, escribí y desplegá contratos EVM</p>
        </div>
        <div className="btn-group">
          <button className="btn secondary" onClick={() => setImportOpen(true)}>Importar</button>
          <button className="btn" onClick={() => setDeployOpen(true)}>Desplegar</button>
        </div>
      </div>

      <ErrorBanner message={error} />

      {contracts.length === 0 ? (
        <div className="card center" style={{ padding: 34 }}>
          <div style={{ fontSize: 30, marginBottom: 8 }}>▤</div>
          <div style={{ fontWeight: 700 }}>No hay contratos importados</div>
          <p className="muted small mt">Importá un contrato con su ABI para leer y escribir funciones.</p>
          <button className="btn secondary mt" onClick={() => setImportOpen(true)}>Importar contrato</button>
        </div>
      ) : (
        <div className="grid two">
          {contracts.map((contract) => (
            <Link key={contract.address} to={`/contracts/${contract.address}`} className="card" style={{ color: 'inherit', textDecoration: 'none' }}>
              <div className="spread">
                <div>
                  <div style={{ fontWeight: 750 }}>{contract.name || 'Contrato sin nombre'}</div>
                  <div className="mono muted small mt">{shorten(contract.address, 16, 10)}</div>
                </div>
                <span className={`badge ${contract.deployed ? 'green' : 'yellow'}`}>{contract.deployed ? 'Desplegado' : 'Sin código'}</span>
              </div>
              <div className="row mt">
                <span className="badge accent">{contract.methods?.length ?? 0} métodos</span>
                <span className="badge">{contract.events?.length ?? 0} eventos</span>
              </div>
            </Link>
          ))}
        </div>
      )}

      <div className="card mt">
        <h3 style={{ marginTop: 0 }}>Contratos y EVM</h3>
        <p className="muted small">
          Kron Smart Chain usa EVM con Chain ID 2026. Las direcciones de contrato son de 20 bytes (40 hex).
          Las llamadas de escritura requieren 2FA y se firman con tu wallet.
        </p>
      </div>

      {importOpen ? (
        <Modal title="Importar contrato" subtitle="Dirección + ABI JSON" onClose={() => !busy && setImportOpen(false)}>
          <form onSubmit={importContract}>
            <div className="field"><label>Dirección</label><input className="input mono" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="40 caracteres hex" required minLength={40} maxLength={42} /></div>
            <div className="field"><label>Nombre (opcional)</label><input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Mi contrato" /></div>
            <div className="field"><label>ABI</label><textarea className="textarea" value={abi} onChange={(e) => setAbi(e.target.value)} placeholder={ABI_PLACEHOLDER} required /></div>
            {error ? <div className="alert error mb">{error}</div> : null}
            <div className="btn-group">
              <button className="btn secondary" type="button" disabled={busy} onClick={() => setImportOpen(false)}>Cancelar</button>
              <button className="btn" type="submit" disabled={busy}>{busy ? <span className="spinner" /> : null} Importar</button>
            </div>
          </form>
        </Modal>
      ) : null}

      {deployOpen ? (
        <Modal title="Desplegar contrato" subtitle="Bytecode + ABI y argumentos del constructor" onClose={() => !busy && setDeployOpen(false)}>
          <form onSubmit={deploy}>
            <div className="field"><label>Nombre (opcional)</label><input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="MiToken" /></div>
            <div className="field"><label>Bytecode</label><textarea className="textarea" value={bytecode} onChange={(e) => setBytecode(e.target.value)} placeholder="0x6060..." required /></div>
            <div className="field"><label>ABI (opcional)</label><textarea className="textarea" value={abi} onChange={(e) => setAbi(e.target.value)} placeholder={ABI_PLACEHOLDER} /></div>
            <div className="field"><label>Argumentos del constructor (JSON array)</label><textarea className="textarea" value={constructorArgs} onChange={(e) => setConstructorArgs(e.target.value)} placeholder='["MiToken","MTK",18,"1000000000000000000000000"]' /></div>
            <div className="grid two" style={{ gap: 8 }}>
              <div className="field"><label>Gas limit</label><input className="input" inputMode="numeric" value={gasLimit} onChange={(e) => setGasLimit(e.target.value.replace(/\D/g, ''))} placeholder="2500000" /></div>
              <div className="field"><label>Gas price KSC</label><input className="input" inputMode="decimal" value={gasPrice} onChange={(e) => setGasPrice(e.target.value.replace(',', '.'))} placeholder="0.00000040" /></div>
            </div>
            {error ? <div className="alert error mb">{error}</div> : null}
            <div className="btn-group">
              <button className="btn secondary" type="button" disabled={busy} onClick={() => setDeployOpen(false)}>Cancelar</button>
              <button className="btn" type="submit" disabled={busy}>{busy ? <span className="spinner" /> : null} Desplegar</button>
            </div>
          </form>
        </Modal>
      ) : null}
    </div>
  );
}
