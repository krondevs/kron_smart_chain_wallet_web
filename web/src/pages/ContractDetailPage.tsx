import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import { api, newIdempotencyKey, type Contract } from '../lib/api';
import { CopyButton, ErrorBanner, Loading } from '../components/common';
import { useToast } from '../lib/toast';

type Tab = 'read' | 'write' | 'receipts' | 'logs' | 'code';

export function ContractDetailPage() {
  const { address = '' } = useParams();
  const toast = useToast();
  // Keep a usable stub from the URL so the address can still be shown and
  // copied if the contract is not imported yet or the API returns an error.
  const [contract, setContract] = useState<Contract | null>(() => address
    ? { address, name: '', abi: '', deployed: false, imported: false }
    : null);
  const [tab, setTab] = useState<Tab>('read');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState('');
  const [txid, setTxid] = useState('');
  const [abiInput, setAbiInput] = useState('');
  const [importName, setImportName] = useState('');

  const [method, setMethod] = useState('');
  const [args, setArgs] = useState('[]');
  const [amount, setAmount] = useState('');
  const [gasLimit, setGasLimit] = useState('');
  const [gasPrice, setGasPrice] = useState('');
  const [block, setBlock] = useState('');
  const [receiptTx, setReceiptTx] = useState('');
  const [fromBlock, setFromBlock] = useState('0');
  const [toBlock, setToBlock] = useState('-1');

  const load = useCallback(async () => {
    setError('');
    try {
      const data = await api.get<Contract>(`/api/v1/contracts/${address}`);
      setContract(data);
      if (data.methods && data.methods.length > 0) setMethod(data.methods[0].signature);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cargar el contrato.');
    } finally {
      setLoading(false);
    }
  }, [address]);

  useEffect(() => {
    setContract({ address, name: '', abi: '', deployed: false, imported: false });
    load();
  }, [address, load]);

  const methods = useMemo(() => contract?.methods ?? [], [contract]);
  const selected = methods.find((item) => item.signature === method || item.name === method);

  const importABI = async () => {
    setBusy(true);
    setError('');
    try {
      await api.post('/api/v1/contracts', {
        address,
        name: importName || contract?.name || '',
        abi: abiInput,
      });
      toast.push('ABI importado', 'success');
      setAbiInput('');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo importar el ABI.');
    } finally {
      setBusy(false);
    }
  };

  const parseArgs = (): unknown[] => {
    if (!args.trim()) return [];
    const parsed = JSON.parse(args);
    if (!Array.isArray(parsed)) throw new Error('Los argumentos deben ser un array JSON.');
    return parsed;
  };

  const call = async () => {
    setBusy(true);
    setError('');
    setResult('');
    try {
      const data = await api.post<Record<string, unknown>>('/api/v1/contracts/read', {
        address,
        method,
        args: parseArgs(),
        gaslimit: gasLimit,
        amount,
        block,
      });
      setResult(JSON.stringify(data, null, 2));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo ejecutar la llamada.');
    } finally {
      setBusy(false);
    }
  };

  const write = async () => {
    setBusy(true);
    setError('');
    setTxid('');
    try {
      const data = await api.post<{ txid: string; data: string }>('/api/v1/contracts/write', {
        address,
        method,
        args: parseArgs(),
        amount,
        gaslimit: gasLimit,
        gasprice: gasPrice,
      }, newIdempotencyKey());
      setTxid(data.txid);
      setResult(JSON.stringify(data, null, 2));
      toast.push('Transacción enviada', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo escribir al contrato.');
    } finally {
      setBusy(false);
    }
  };

  const loadReceipts = async () => {
    setBusy(true);
    setError('');
    setResult('');
    try {
      const data = await api.get<unknown>(`/api/v1/contracts/${address}/receipts?txid=${encodeURIComponent(receiptTx)}`);
      setResult(JSON.stringify(data, null, 2));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo obtener el recibo.');
    } finally {
      setBusy(false);
    }
  };

  const loadLogs = async () => {
    setBusy(true);
    setError('');
    setResult('');
    try {
      const data = await api.get<unknown>(`/api/v1/contracts/${address}/logs?fromblock=${encodeURIComponent(fromBlock)}&toblock=${encodeURIComponent(toBlock)}`);
      setResult(JSON.stringify(data, null, 2));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudieron obtener los logs.');
    } finally {
      setBusy(false);
    }
  };

  const loadCode = async () => {
    setBusy(true);
    setError('');
    setResult('');
    try {
      const data = await api.get<{ code: string }>(`/api/v1/contracts/${address}/code`);
      setResult(data.code);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo obtener el código.');
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <div className="page"><Loading /></div>;
  if (!contract) return <div className="page"><ErrorBanner message={error || 'Contrato no encontrado'} /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>{contract.name || 'Contrato'}</h2>
          <p className="mono wrap">{contract.address}</p>
        </div>
        <CopyButton text={contract.address} label="Copiar dirección" />
      </div>

      <ErrorBanner message={error} />
      {txid ? <div className="alert success mb"><strong>Tx:</strong> <span className="mono wrap">{txid}</span></div> : null}

      {!contract.abi ? (
        <div className="card mb">
          <h3 style={{ marginTop: 0 }}>Importar ABI</h3>
          <p className="muted small">
            {contract.imported === false
              ? 'El contrato existe en la cadena, pero no está importado en esta cuenta.'
              : 'El contrato está importado, pero no tiene ABI guardado.'}
            {' '}Pegá el ABI JSON para habilitar lectura y escritura.
          </p>
          <div className="field">
            <label>Nombre (opcional)</label>
            <input className="input" value={importName} onChange={(event) => setImportName(event.target.value)} placeholder={contract.name || 'Mi contrato'} />
          </div>
          <div className="field">
            <label>ABI</label>
            <textarea className="textarea" value={abiInput} onChange={(event) => setAbiInput(event.target.value)} placeholder='[{"type":"function","name":"balanceOf",...}]' />
          </div>
          <button className="btn" disabled={busy || !abiInput.trim()} onClick={importABI}>Importar ABI</button>
        </div>
      ) : null}

      <div className="card mb">
        <div className="auth-tabs">
          <button className={tab === 'read' ? 'active' : ''} onClick={() => setTab('read')}>Leer</button>
          <button className={tab === 'write' ? 'active' : ''} onClick={() => setTab('write')}>Escribir</button>
          <button className={tab === 'receipts' ? 'active' : ''} onClick={() => setTab('receipts')}>Recibos</button>
          <button className={tab === 'logs' ? 'active' : ''} onClick={() => setTab('logs')}>Logs</button>
          <button className={tab === 'code' ? 'active' : ''} onClick={() => setTab('code')}>Código</button>
        </div>

        {tab === 'read' || tab === 'write' ? (
          <>
            <div className="field">
              <label>Método</label>
              <select className="select" value={method} onChange={(e) => setMethod(e.target.value)}>
                {methods.length === 0 ? <option value="">Sin métodos en el ABI</option> : null}
                {methods.map((item) => <option key={item.signature} value={item.signature}>{item.signature}</option>)}
              </select>
              {selected ? <span className="hint">Mutabilidad: {selected.state_mutability || 'nonpayable'} · {(selected.inputs ?? []).length} argumentos</span> : null}
            </div>

            <div className="field">
              <label>Argumentos (JSON array)</label>
              <textarea className="textarea" value={args} onChange={(e) => setArgs(e.target.value)} placeholder={selected ? argumentPlaceholder((selected.inputs ?? []).map((input) => input.type)) : '[]'} />
              <span className="hint">Ejemplo: ["0x...", "1000000000000000000", true]</span>
            </div>

            <div className="grid two" style={{ gap: 8 }}>
              <div className="field"><label>Gas limit</label><input className="input" inputMode="numeric" value={gasLimit} onChange={(e) => setGasLimit(e.target.value.replace(/\D/g, ''))} placeholder="250000" /></div>
              {tab === 'write' ? <div className="field"><label>Gas price KSC</label><input className="input" inputMode="decimal" value={gasPrice} onChange={(e) => setGasPrice(e.target.value.replace(',', '.'))} placeholder="0.00000040" /></div> : <div className="field"><label>Bloque</label><input className="input" inputMode="numeric" value={block} onChange={(e) => setBlock(e.target.value.replace(/\D/g, ''))} placeholder="-1" /></div>}
            </div>

            <div className="field"><label>Valor KSC (opcional)</label><input className="input" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value.replace(',', '.'))} placeholder="0" /></div>

            <button className="btn wide" disabled={busy || !method} onClick={tab === 'read' ? call : write}>
              {busy ? <span className="spinner" /> : null}
              {tab === 'read' ? 'Ejecutar llamada' : 'Enviar transacción'}
            </button>
          </>
        ) : null}

        {tab === 'receipts' ? (
          <>
            <div className="field"><label>TXID</label><input className="input mono" value={receiptTx} onChange={(e) => setReceiptTx(e.target.value)} placeholder="64 caracteres hex" /></div>
            <button className="btn" disabled={busy || receiptTx.length !== 64} onClick={loadReceipts}>Obtener recibo</button>
          </>
        ) : null}

        {tab === 'logs' ? (
          <>
            <div className="grid two" style={{ gap: 8 }}>
              <div className="field"><label>Desde bloque</label><input className="input" value={fromBlock} onChange={(e) => setFromBlock(e.target.value)} /></div>
              <div className="field"><label>Hasta bloque</label><input className="input" value={toBlock} onChange={(e) => setToBlock(e.target.value)} /></div>
            </div>
            <button className="btn" disabled={busy} onClick={loadLogs}>Buscar logs</button>
          </>
        ) : null}

        {tab === 'code' ? (
          <button className="btn" disabled={busy} onClick={loadCode}>Obtener bytecode</button>
        ) : null}
      </div>

      <div className="card">
        <div className="card-head"><h3>Resultado</h3>{result ? <CopyButton text={result} label="Copiar" /> : null}</div>
        {result ? <pre className="mono small wrap" style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{result}</pre> : <div className="alert">Ejecutá una acción para ver el resultado.</div>}
      </div>

      <div className="card mt">
        <h3 style={{ marginTop: 0 }}>ABI</h3>
        <div className="row">
          <span className="badge accent">{methods.length} métodos</span>
          <span className="badge">{contract.events?.length ?? 0} eventos</span>
          <span className={`badge ${contract.deployed ? 'green' : 'yellow'}`}>{contract.deployed ? 'Desplegado' : 'Sin código'}</span>
        </div>
        {methods.length > 0 ? (
          <div className="list mt">
            {methods.map((item) => (
              <div className="list-item" key={item.signature}>
                <div className="main">
                  <div className="title mono">{item.signature}</div>
                  <div className="sub">{item.state_mutability || 'nonpayable'}</div>
                </div>
                <span className="badge">{(item.inputs ?? []).length} in / {(item.outputs ?? []).length} out</span>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function argumentPlaceholder(types: string[]): string {
  return JSON.stringify(types.map((type) => {
    if (type.startsWith('uint') || type.startsWith('int')) return '0';
    if (type === 'bool') return false;
    if (type === 'address') return '0x0000000000000000000000000000000000000000';
    if (type.startsWith('bytes')) return '0x';
    if (type.endsWith('[]')) return [];
    return '';
  }));
}
