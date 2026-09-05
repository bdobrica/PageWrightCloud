import { useEffect, useRef, useState } from 'react';
import { config } from '../config';
import type { JobStatusUpdate } from '../types/api';

export const useWebSocket = (onMessage: (update: JobStatusUpdate) => void) => {
  const [isConnected, setIsConnected] = useState(false);
  const onMessageRef = useRef(onMessage);

  useEffect(() => { onMessageRef.current = onMessage; }, [onMessage]);

  useEffect(() => {
    let disposed = false;
    let socket: WebSocket | null = null;
    let reconnectTimeout: number | undefined;

    function connect() {
      const token = localStorage.getItem('token');
      if (disposed || !token) return;
      const ws = new WebSocket(`${config.wsUrl}?token=${encodeURIComponent(token)}`);
      socket = ws;
      ws.onopen = () => { if (!disposed) setIsConnected(true); };
      ws.onmessage = (event) => {
        if (disposed) return;
        try {
          onMessageRef.current(JSON.parse(event.data) as JobStatusUpdate);
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };
      ws.onerror = (error) => { if (!disposed) console.error('WebSocket error:', error); };
      ws.onclose = () => {
        if (disposed) return;
        setIsConnected(false);
        socket = null;
        reconnectTimeout = window.setTimeout(connect, 5000);
      };
    }

    connect();
    return () => {
      disposed = true;
      window.clearTimeout(reconnectTimeout);
      if (socket) {
        socket.onclose = null;
        socket.close();
      }
    };
  }, []);

  return { isConnected };
};
