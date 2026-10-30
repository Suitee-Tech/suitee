import { useNavigate } from "react-router";
import { Building2, Plus, ArrowRight, LogOut, ChevronRight } from "lucide-react";
import { motion } from "motion/react";
import { useAuth } from "../context/AuthContext";
import suiteeLogo from "../../assets/logo.png";
import { CompanyLogo } from "../components/CompanyLogo";
import { Button } from "../ui/button";
// removed suiteeIcon

export function CompanySelection() {
 const navigate = useNavigate();
 const { companies, selectCompany, logout } = useAuth();

 const handleSelectCompany = async (id: string) => {
 try {
 await selectCompany(id);
 navigate("/");
 } catch (error) {
 console.error("Erro ao selecionar empresa:", error);
 }
 };

 return (
 <div className="min-h-screen bg-muted flex flex-col font-sans">

 {/* Top Bar */}
 <div className="flex items-center justify-between px-6 lg:px-10 py-5 bg-card border-b border-border">
 <div className="flex items-center gap-2 text-foreground">
 <img src={suiteeLogo} alt="suitee Logo" className="h-6 object-contain" />
 </div>
 <Button
 variant="ghost"
 size="sm"
 onClick={logout}
 className="font-bold text-muted-foreground hover:text-status-critical"
 >
 <LogOut className="w-4 h-4" />
 Sair da conta
 </Button>
 </div>

 {/* Main Content */}
 <div className="flex-1 flex flex-col items-center justify-center p-6 sm:p-10">
 <motion.div
 initial={{ opacity: 0, y: 20 }}
 animate={{ opacity: 1, y: 0 }}
 className="w-full max-w-2xl"
 >
 <div className="text-center mb-10">
 <div className="w-16 h-16 rounded-full flex items-center justify-center mx-auto mb-4">
 <CompanyLogo className="w-10 h-10 object-contain opacity-80" />
 </div>
 <h1 className="text-3xl font-bold text-foreground mb-2">Selecione seu Workspace</h1>
 <p className="text-muted-foreground">Sua conta está vinculada a múltiplos ambientes. Escolha qual imobiliária deseja acessar agora.</p>
 </div>

 <div className="grid gap-4 mb-8">
 {companies.map((company, index) => (
 <motion.button
 key={company.id}
 initial={{ opacity: 0, x: -20 }}
 animate={{ opacity: 1, x: 0 }}
 transition={{ delay: index * 0.1 }}
 onClick={() => handleSelectCompany(company.id)}
 className="w-full bg-card p-5 rounded-2xl border border-border hover:border-primary-400 hover:shadow-md transition-all flex items-center gap-5 group text-left relative overflow-hidden"
 >
 <div className="w-16 h-16 rounded-xl bg-muted shrink-0 overflow-hidden border border-border flex items-center justify-center">
 <span className="text-2xl font-bold text-muted-foreground">
 {company.name ? company.name.charAt(0).toUpperCase() : 'C'}
 </span>
 </div>
 <div className="flex-1 min-w-0">
 <h3 className="text-lg font-bold text-foreground truncate group-hover:text-primary-600 transition-colors">{company.name}</h3>
 <div className="flex items-center gap-3 mt-1">
 <span className="inline-flex items-center px-2 py-0.5 rounded text-caption font-bold uppercase tracking-wider bg-primary-50 text-primary-700">
 {company.profile || 'Membro'}
 </span>
 <span className="text-xs text-muted-foreground font-medium">
 {company.email}
 </span>
 </div>
 </div>
 <div className="w-10 h-10 rounded-full bg-muted group-hover:bg-primary-50 flex items-center justify-center text-muted-foreground group-hover:text-primary-600 transition-colors shrink-0">
 <ArrowRight className="w-5 h-5 group-hover:translate-x-0.5 transition-transform" />
 </div>
 </motion.button>
 ))}
 </div>

 <div className="relative mb-8">
 <div className="absolute inset-0 flex items-center">
 <div className="w-full border-t border-border"></div>
 </div>
 <div className="relative flex justify-center text-xs">
 <span className="px-4 bg-muted text-muted-foreground font-bold uppercase tracking-wider">Ou</span>
 </div>
 </div>

 <Button
 variant="outline"
 onClick={() => navigate("/onboarding")}
 className="w-full h-14 border-2 border-dashed rounded-2xl text-muted-foreground font-bold hover:bg-muted hover:border-primary-300 hover:text-primary-600"
 >
 <Plus className="w-5 h-5" />
 Cadastrar Nova Empresa
 </Button>
 </motion.div>
 </div>
 </div>
 );
}
