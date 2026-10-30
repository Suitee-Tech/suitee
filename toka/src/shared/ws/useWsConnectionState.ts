import { useEffect, useState } from "react";
import { wsManager } from "./wsManager";

export function useWsConnectionState() {
  const [connected, setConnected] = useState(wsManager.getIsConnected());

  useEffect(() => {
    const id = wsManager.subscribeStatus(setConnected);
    return () => wsManager.unsubscribeStatus(id);
  }, []);

  return connected;
}
