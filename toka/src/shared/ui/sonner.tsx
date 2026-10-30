"use client";

import { Toaster as Sonner, type ToasterProps } from "sonner";
import { useTheme } from "../context/ThemeContext";

const Toaster = ({ ...props }: ToasterProps) => {
  // O tema vem do ThemeContext da aplicação, não do `next-themes`: este projeto
  // é Vite + React e não monta um NextThemesProvider, então o `useTheme` do
  // next-themes devolvia sempre "system" e os toasts seguiam o tema do SISTEMA
  // OPERACIONAL. Com o app em modo noturno num SO claro, o toast saía claro —
  // vermelho 4.35:1 sobre rosa claro, o oposto do resto da tela.
  const { resolvedMode } = useTheme();

  return (
    <Sonner
      theme={resolvedMode}
      className="toaster group"
      style={
        {
          // Os tokens daqui são triplas HSL (`222 47% 8%`), não cores prontas:
          // sem o `hsl(...)` o sonner recebia um valor inválido e caía no
          // default dele em vez de usar a superfície do tema.
          "--normal-bg": "hsl(var(--popover))",
          "--normal-text": "hsl(var(--popover-foreground))",
          "--normal-border": "hsl(var(--border))",
        } as React.CSSProperties
      }
      {...props}
    />
  );
};

export { Toaster };
