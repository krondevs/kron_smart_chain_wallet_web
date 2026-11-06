import { useEffect, useRef, useState } from 'react';
import jsQR from 'jsqr';
import { Modal } from './Modal';

type BarcodeDetectorLike = {
  detect: (source: HTMLVideoElement) => Promise<Array<{ rawValue: string }>>;
};

export function QRScanner({ onResult, onClose }: { onResult: (value: string) => void; onClose: () => void }) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [error, setError] = useState('');
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let stream: MediaStream | null = null;
    let timer: number | null = null;
    let cancelled = false;

    const start = async () => {
      try {
        stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false });
        if (cancelled) return;
        if (!videoRef.current) return;
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
        setReady(true);

        const Detector = (window as unknown as {
          BarcodeDetector?: new (options?: { formats?: string[] }) => BarcodeDetectorLike;
        }).BarcodeDetector;

        if (Detector) {
          const detector = new Detector({ formats: ['qr_code'] });
          timer = window.setInterval(async () => {
            if (!videoRef.current || cancelled) return;
            try {
              const codes = await detector.detect(videoRef.current);
              if (codes.length > 0 && codes[0].rawValue) {
                onResult(codes[0].rawValue);
                onClose();
              }
            } catch {
              // Ignore transient decode errors.
            }
          }, 400);
        } else {
          // Fallback for Safari/iOS/Firefox, which do not expose BarcodeDetector.
          const canvas = document.createElement('canvas');
          const context = canvas.getContext('2d', { willReadFrequently: true });
          timer = window.setInterval(() => {
            const video = videoRef.current;
            if (!video || !context || cancelled || video.videoWidth === 0) return;
            canvas.width = video.videoWidth;
            canvas.height = video.videoHeight;
            context.drawImage(video, 0, 0, canvas.width, canvas.height);
            const image = context.getImageData(0, 0, canvas.width, canvas.height);
            const code = jsQR(image.data, image.width, image.height, { inversionAttempts: 'dontInvert' });
            if (code?.data) {
              onResult(code.data);
              onClose();
            }
          }, 350);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'No se pudo abrir la cámara. Verificá los permisos del navegador.');
      }
    };

    start();
    return () => {
      cancelled = true;
      if (timer) window.clearInterval(timer);
      stream?.getTracks().forEach((track) => track.stop());
    };
  }, [onResult, onClose]);

  return (
    <Modal title="Escanear QR" subtitle="Apuntá la cámara al código de la dirección" onClose={onClose}>
      {error ? <div className="alert error mb">{error}</div> : null}
      <div style={{ position: 'relative', borderRadius: 16, overflow: 'hidden', background: '#000', minHeight: 240 }}>
        <video ref={videoRef} playsInline muted style={{ width: '100%', display: 'block', maxHeight: 360, objectFit: 'cover' }} />
        {!ready && !error ? <div className="center muted" style={{ padding: 24 }}>Abriendo cámara…</div> : null}
      </div>
      <div className="alert mt small">
        Si la cámara no abre, verificá el permiso del navegador o pegá la dirección / URI <span className="kbd">kron:</span> manualmente.
      </div>
    </Modal>
  );
}
