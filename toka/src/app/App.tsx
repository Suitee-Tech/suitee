import { RouterProvider } from "react-router";
import { Toaster } from "../shared/ui/sonner";
import { router } from "./routes";
import { ThemeProvider } from "./context/ThemeContext";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ReactQueryProvider } from "../lib/reactQuery";

export default function App() {
 return (
 <ErrorBoundary variant="page">
 <ReactQueryProvider>
 {/* O Toaster vive DENTRO do ThemeProvider: ele decide claro/escuro pelo
     `resolvedMode` do tema da aplicação — fora daqui leria o contexto default
     e os toasts ficariam claros no modo noturno. */}
 <ThemeProvider>
 <RouterProvider router={router} />
 <Toaster position="bottom-right" richColors closeButton mobileOffset={{ bottom: "calc(4rem + env(safe-area-inset-bottom) + 16px)" }} />
 </ThemeProvider>
 </ReactQueryProvider>
 </ErrorBoundary>
 );
}
