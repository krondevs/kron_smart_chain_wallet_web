import { useEffect, useState } from 'react';

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>;
};

const DISMISS_KEY = 'kron_install_dismissed_v1';

export function InstallPrompt() {
  const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);
  const [visible, setVisible] = useState(false);
  const [ios, setIos] = useState(false);
  const [standalone, setStandalone] = useState(false);

  useEffect(() => {
    if (localStorage.getItem(DISMISS_KEY) === '1') return;

    const isStandalone =
      window.matchMedia('(display-mode: standalone)').matches ||
      (navigator as unknown as { standalone?: boolean }).standalone === true;
    if (isStandalone) {
      setStandalone(true);
      return;
    }

    const ua = navigator.userAgent;
    const isIOS = /iPad|iPhone|iPod/.test(ua);
    const isMobile = isIOS || /Android/i.test(ua) || (navigator.maxTouchPoints > 0 && window.innerWidth < 900);
    if (!isMobile) return;

    setIos(isIOS);

    const onPrompt = (event: Event) => {
      event.preventDefault();
      setDeferred(event as BeforeInstallPromptEvent);
      setVisible(true);
    };
    const onInstalled = () => {
      setStandalone(true);
      setVisible(false);
      setDeferred(null);
      localStorage.setItem(DISMISS_KEY, '1');
    };

    window.addEventListener('beforeinstallprompt', onPrompt);
    window.addEventListener('appinstalled', onInstalled);

    // iOS Safari never fires beforeinstallprompt. Other browsers may take a
    // moment; show a manual-install hint if no prompt arrives.
    const timer = window.setTimeout(() => setVisible(true), isIOS ? 1200 : 4000);

    return () => {
      window.clearTimeout(timer);
      window.removeEventListener('beforeinstallprompt', onPrompt);
      window.removeEventListener('appinstalled', onInstalled);
    };
  }, []);

  const dismiss = () => {
    localStorage.setItem(DISMISS_KEY, '1');
    setVisible(false);
  };

  const install = async () => {
    if (!deferred) return;
    await deferred.prompt();
    const choice = await deferred.userChoice;
    setDeferred(null);
    if (choice.outcome === 'accepted') {
      setVisible(false);
      localStorage.setItem(DISMISS_KEY, '1');
    } else {
      dismiss();
    }
  };

  if (!visible || standalone) return null;

  return (
    <div className="install-prompt" role="dialog" aria-label="Instalar Kron Wallet">
      <div className="install-icon">K</div>
      <div className="install-copy">
        <strong>Instalá Kron Wallet</strong>
        {deferred ? (
          <span>Usala como app en tu teléfono, sin abrir el navegador.</span>
        ) : ios ? (
          <span>En Safari: Compartir → “Añadir a pantalla de inicio”.</span>
        ) : (
          <span>En el menú del navegador elegí “Instalar aplicación”.</span>
        )}
      </div>
      <div className="install-actions">
        {deferred ? <button className="btn small" onClick={install}>Instalar</button> : null}
        <button className="btn ghost small" onClick={dismiss}>Ahora no</button>
      </div>
    </div>
  );
}
