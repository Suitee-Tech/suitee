
 import { createRoot } from "react-dom/client";
 import App from "./app/App.tsx";
 import "./styles/index.css";
 import { isStaleChunkError, reloadForStaleChunk } from "./shared/lib/staleChunk";

 // Recuperação de chunk obsoleto após deploy. Vite emite `vite:preloadError`
 // para o caso canônico; o `unhandledrejection` é a rede de segurança para
 // imports dinâmicos que falham sem passar pelo helper do Vite.
 window.addEventListener("vite:preloadError", (event) => {
   reloadForStaleChunk("vite:preloadError", event);
 });

 window.addEventListener("unhandledrejection", (event) => {
   if (isStaleChunkError(event.reason)) reloadForStaleChunk("unhandledrejection", event.reason);
 });

 createRoot(document.getElementById("root")!).render(<App />);
