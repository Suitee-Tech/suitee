import { useState, useEffect, useRef, useCallback } from 'react';
import { logger } from '../../lib/logger';

type WebSocketEvent = {
 type: string;
 payload: any;
};

export function useWebSocket(url: string, token: string | null) {
 const [isConnected, setIsConnected] = useState(false);
 const wsRef = useRef<WebSocket | null>(null);
 const reconnectTimeoutRef = useRef<NodeJS.Timeout | undefined>(undefined);

 const onMessageRef = useRef<((event: WebSocketEvent) => void) | undefined>(undefined);

 const connect = useCallback(() => {
 if (!token || !url) return;

 if (wsRef.current?.readyState === WebSocket.OPEN) {
 return;
 }

 const wsUrl = `${url}?token=${token}`;
 const ws = new WebSocket(wsUrl);

 ws.onopen = () => {
 logger.debug('WebSocket connected');
 setIsConnected(true);
 if (reconnectTimeoutRef.current) {
 clearTimeout(reconnectTimeoutRef.current);
 }
 };

 ws.onmessage = (event) => {
 try {
 // O backend pode enviar múltiplos eventos no mesmo frame (separados por '\n').
 // Ex.: o Hub concatena mensagens pendentes na mesma escrita do websocket.
 const dataStr = typeof event.data === 'string' ? event.data : '';
 const chunks = dataStr.split('\n').map((s) => s.trim()).filter(Boolean);
 for (const chunk of chunks) {
 const parsed: WebSocketEvent = JSON.parse(chunk);
 if (onMessageRef.current) {
 onMessageRef.current(parsed);
 }
 }
 } catch (err) {
 console.error('Failed to parse WebSocket message', err);
 }
 };

 ws.onclose = () => {
 logger.debug('WebSocket disconnected');
 setIsConnected(false);
 // Try to reconnect after 3 seconds
 reconnectTimeoutRef.current = setTimeout(connect, 3000);
 };

 ws.onerror = (error) => {
 console.error('WebSocket error:', error);
 ws.close();
 };

 wsRef.current = ws;
 }, [url, token]);

 useEffect(() => {
 connect();
 return () => {
 if (wsRef.current) {
 wsRef.current.close();
 }
 if (reconnectTimeoutRef.current) {
 clearTimeout(reconnectTimeoutRef.current);
 }
 };
 }, [connect]);

 const setMessageHandler = useCallback((handler: (event: WebSocketEvent) => void) => {
 onMessageRef.current = handler;
 }, []);

 return { isConnected, setMessageHandler };
}
