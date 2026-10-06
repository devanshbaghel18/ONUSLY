import { useEffect, useState, useRef, useCallback } from "react";
import { wsManager } from "../lib/websocket";

/**
 * useWebSocket hook subscribes a component to real-time events.
 *
 * @param {Function} [onEvent] Optional callback invoked on each received event { type, payload }
 * @returns {{ isConnected: boolean, lastEvent: any, send: Function, connect: Function, disconnect: Function }}
 */
export function useWebSocket(onEvent) {
  const [isConnected, setIsConnected] = useState(wsManager.isConnected);
  const [lastEvent, setLastEvent] = useState(null);
  const onEventRef = useRef(onEvent);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    // Track connection status
    const unsubStatus = wsManager.onStatusChange(setIsConnected);

    // Listen for incoming WebSocket events
    const unsubEvents = wsManager.subscribe((event) => {
      setLastEvent(event);
      if (onEventRef.current) {
        onEventRef.current(event);
      }
    });

    return () => {
      unsubStatus();
      unsubEvents();
    };
  }, []);

  const send = useCallback((data) => wsManager.send(data), []);
  const connect = useCallback(() => wsManager.connect(), []);
  const disconnect = useCallback(() => wsManager.disconnect(), []);

  return {
    isConnected,
    lastEvent,
    send,
    connect,
    disconnect,
  };
}

export default useWebSocket;
