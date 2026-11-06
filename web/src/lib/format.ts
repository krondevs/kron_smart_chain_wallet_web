export function shorten(value: string | undefined, head = 8, tail = 6): string {
  if (!value) return '';
  if (value.length <= head + tail + 3) return value;
  return `${value.slice(0, head)}…${value.slice(-tail)}`;
}

export function formatTime(value?: string | number | null): string {
  if (!value) return '—';
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('es', {
    dateStyle: 'short',
    timeStyle: 'short',
  }).format(date);
}

export function formatDate(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('es', { dateStyle: 'medium' }).format(date);
}

export function formatAmount(value: string | number | undefined, maxDecimals = 8): string {
  if (value === undefined || value === null || value === '') return '0';
  const number = typeof value === 'number' ? value : Number(value);
  if (Number.isNaN(number)) return String(value);
  return new Intl.NumberFormat('es', {
    maximumFractionDigits: maxDecimals,
    minimumFractionDigits: 0,
  }).format(number);
}

export function formatFiat(value: number): string {
  return new Intl.NumberFormat('es', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }).format(value);
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

export function downloadText(filename: string, content: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function classNames(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(' ');
}

export function transactionTitle(tx: { category?: string; address?: string; comment?: string }): string {
  const category = (tx.category ?? '').toLowerCase();
  if (tx.comment) return tx.comment;
  if (category === 'receive') return 'Recibido';
  if (category === 'send') return 'Enviado';
  if (category === 'stake' || category === 'staked') return 'Recompensa';
  if (category === 'immature') return 'Maduración';
  if (category === 'generate' || category === 'mined') return 'Minería';
  return tx.category ?? 'Transacción';
}

export function transactionSign(tx: { category?: string; amount?: string; amount_sat?: number }): 1 | -1 {
  const category = (tx.category ?? '').toLowerCase();
  if (category === 'send') return -1;
  if (typeof tx.amount_sat === 'number') return tx.amount_sat < 0 ? -1 : 1;
  if (tx.amount && tx.amount.startsWith('-')) return -1;
  return 1;
}
