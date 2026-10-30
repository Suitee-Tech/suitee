import { useEffect, useRef, useState, useCallback } from "react";
import { SuperDocRef } from "@superdoc-dev/react";
import { getVariableByKey, type ContractVariable } from "../contracts/contractVariables";

export interface VariableDetectionState {
  allFoundKeys: string[];
  identifiedVars: ContractVariable[];
  unidentifiedKeys: string[];
  isScanning: boolean;
}

export interface UseVariableDetectionOptions {
  /** Callback para extrair texto do editor via DOM (fallback v2 quando getHTML falha) */
  getDomText?: () => string;
}

function hashTextContent(value: string): string {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `${value.length}:${hash >>> 0}`;
}

/** Word may split a visible tag between formatting spans. Keep paragraph
 * boundaries while removing inline markup so those tags are still detected. */
function getLogicalEditorText(html: string): string {
  const parsed = new DOMParser().parseFromString(html, 'text/html');
  parsed.querySelectorAll('p, div, li, tr, td, th, br').forEach((element) => {
    element.appendChild(parsed.createTextNode('\n'));
  });
  return parsed.body.textContent || '';
}

export function useVariableDetection(
  editorRef: React.RefObject<SuperDocRef | null>,
  options?: UseVariableDetectionOptions,
) {
  const [state, setState] = useState<VariableDetectionState>({
    allFoundKeys: [],
    identifiedVars: [],
    unidentifiedKeys: [],
    isScanning: false,
  });

  const lastHashRef = useRef("");
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const scan = useCallback(() => {
    const superdoc = editorRef.current?.getInstance();
    if (!superdoc) return;

    let textContent = "";
    try {
      const htmls = superdoc.getHTML();
      textContent = htmls.map((html) => getLogicalEditorText(String(html ?? ''))).join("\n");
      if (!textContent) textContent = optionsRef.current?.getDomText?.() || '';
    } catch {
      // v2 fallback: extrai texto do DOM do editor
      const getDomText = optionsRef.current?.getDomText;
      if (getDomText) {
        textContent = getDomText();
      }
      if (!textContent) return;
    }

    const hash = hashTextContent(textContent);
    if (hash === lastHashRef.current) return;
    lastHashRef.current = hash;

    setState((prev) => ({ ...prev, isScanning: true }));

    const varRegex = /\{\{([^}]+)\}\}/g;
    const found = new Set<string>();
    const loopStack: string[] = [];

    let match;
    while ((match = varRegex.exec(textContent)) !== null) {
      const token = match[1].trim();
      if (token === "" || token.includes("{") || token.includes("}")) continue;

      const open = token.match(/^([#^])\s*([a-zA-Z0-9_.]+)$/);
      if (open) {
        const name = open[2];
        loopStack.push(name);
        found.add(name);
        continue;
      }

      const close = token.match(/^\/\s*([a-zA-Z0-9_.]+)$/);
      if (close) {
        const name = close[1];
        if (loopStack.length > 0 && loopStack[loopStack.length - 1] === name) {
          loopStack.pop();
        } else {
          const idx = loopStack.lastIndexOf(name);
          if (idx !== -1) loopStack.splice(idx, loopStack.length - idx);
        }
        continue;
      }

      if (loopStack.length > 0 && !getVariableByKey(token)) {
        continue;
      }

      if (!/^[a-zA-Z0-9_.]+$/.test(token)) continue;
      found.add(token);
    }

    const allFoundKeys = Array.from(found);
    const identifiedVars: ContractVariable[] = [];
    const unidentifiedKeys: string[] = [];

    for (const key of allFoundKeys) {
      const systemVar = getVariableByKey(key);
      if (systemVar) {
        identifiedVars.push(systemVar);
        continue;
      }

      unidentifiedKeys.push(key);
    }

    setState({
      allFoundKeys,
      identifiedVars,
      unidentifiedKeys,
      isScanning: false,
    });
  }, [editorRef]);

  const triggerScan = useCallback(() => {
    lastHashRef.current = "";
    scan();
  }, [scan]);

  useEffect(() => {
    const interval = setInterval(scan, 2000);
    return () => clearInterval(interval);
  }, [scan]);

  return { ...state, triggerScan };
}
