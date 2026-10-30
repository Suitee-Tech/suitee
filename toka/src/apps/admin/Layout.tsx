import { NavLink, Outlet, useLocation, useNavigate } from "react-router";
import {
  Building2,
  Search,
  Menu,
  X,
  Users,
  Handshake,
  LogOut,
  PanelLeftClose,
  PanelLeft,
  Focus,
  Sparkles,
  UserCog,
  Plus,
  ChevronDown,
} from "lucide-react";
import { useState, useEffect, useCallback, useRef, lazy, Suspense } from "react";
import { clsx } from "clsx";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "../../shared/ui/tooltip";
import { Button } from "../../shared/ui/button";
import { SuiteeIntelligenceMark } from "../../shared/components/ai/SuiteeIntelligenceMark";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../../shared/ui/dialog";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandShortcut } from "../../shared/ui/command";
import { CompanyLogo } from "../../shared/components/CompanyLogo";
import { UserAvatar } from "../../shared/components/UserAvatar";
import { useGlobalSearch } from "../../shared/hooks/useGlobalSearch";
import { NotificationBell } from "../../shared/components/NotificationBell";
import { SystemNotificationPermissionPrompt } from "../../shared/components/SystemNotificationPermissionPrompt";
import { TermsAcceptanceModal } from "../../shared/components/TermsAcceptanceModal";
import { AcceptLeadModal } from "../../shared/components/AcceptLeadModal";
import { useTheme } from "../../shared/context/ThemeContext";
import { useAuth } from "../../shared/context/AuthContext";
import { useFocusMode } from "../../shared/context/FocusModeContext";
import { useIsDesktop } from "../../shared/hooks/useMediaQuery";
const AIChat = lazy(() => import("../../shared/components/ai/AIChat").then(module => ({ default: module.AIChat })));
import { usePermissionsContext } from "../../lib/permissions/PermissionsContext";
import { getVisibleAdminNavGroups } from "../../shared/admin/nav";
import { useSubscriptionAccess } from "../../shared/context/SubscriptionAccessContext";
import { SidebarPlanIndicator, planIndicatorView } from "../../shared/components/subscription/SidebarPlanIndicator";

export function Layout() {
  const location = useLocation();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [openNavGroups, setOpenNavGroups] = useState<Record<string, boolean>>({});
  const sidebarRef = useRef<HTMLElement | null>(null);
  const menuButtonRef = useRef<HTMLButtonElement | null>(null);
  const isDesktop = useIsDesktop();
  const mobileNavOpen = !isDesktop && sidebarOpen;
  const [isAIPanelOpen, setIsAIPanelOpen] = useState(false);
  const [hasOpenedAI, setHasOpenedAI] = useState(false);
  useEffect(() => { if (isAIPanelOpen) setHasOpenedAI(true); }, [isAIPanelOpen]);
  const [isScanning, setIsScanning] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const { theme } = useTheme();
  const { user, companies, logout } = useAuth();
  const { focusMode, toggleFocusMode } = useFocusMode();
  const { can } = usePermissionsContext();
  const { access, hasFeature } = useSubscriptionAccess();
  // P1/A8: capacidade nomeada do assistente (o servidor exige `ai:use`).
  const canUseAI = can("ai:use");
  const canUseAIPlan = hasFeature("intelligence");

  const [isExpanded, setIsExpanded] = useState(() => theme.sidebarStyle === "default");

  const search = useGlobalSearch(searchQuery);

  useEffect(() => {
    setIsExpanded(theme.sidebarStyle === "default");
  }, [theme.sidebarStyle]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      const key = e.key.toLowerCase();
      if (key === "k") {
        e.preventDefault();
        setSearchOpen((prev) => !prev);
        return;
      }
      if (key === "j") {
        e.preventDefault();
        setIsAIPanelOpen((prev) => !prev);
        return;
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    if (!mobileNavOpen) return;
    const opener = menuButtonRef.current;
    sidebarRef.current?.querySelector<HTMLElement>("[data-sidebar-close]")?.focus();
    return () => {
      if (opener?.isConnected && opener.offsetParent !== null) opener.focus();
    };
  }, [mobileNavOpen]);

  useEffect(() => {
    // Ao voltar para desktop a sidebar vira permanente; não faz sentido manter
    // o estado "aberto" do drawer e reabri-lo se a janela encolher de novo.
    if (isDesktop) setSidebarOpen(false);
  }, [isDesktop]);

  useEffect(() => {
    if (!mobileNavOpen) return;
    const node = sidebarRef.current;
    if (!node) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setSidebarOpen(false);
        return;
      }
      if (event.key !== "Tab") return;
      const focusable = Array.from(
        node.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'),
      ).filter((element) => element.offsetParent !== null);
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    node.addEventListener("keydown", onKeyDown);
    return () => node.removeEventListener("keydown", onKeyDown);
  }, [mobileNavOpen]);

  const navigate = useNavigate();

  const handleSearchOpenChange = useCallback((open: boolean) => {
    setSearchOpen(open);
    if (!open) setSearchQuery("");
  }, []);

  const handleSearchSelect = useCallback(
    (route: string) => {
      setSearchOpen(false);
      setSearchQuery("");
      navigate(route);
    },
    [navigate],
  );

  const hasAnySearchResult =
    search.groups.leads.length +
    search.groups.properties.length +
    search.groups.brokers.length +
    search.groups.deals.length >
    0;

  const isMinimal = theme.sidebarStyle === "minimal";

  const handleAIToggle = () => {
    if (!canUseAI) return;
    if (!canUseAIPlan) {
      navigate("/assinatura");
      return;
    }
    if (!isAIPanelOpen && !isScanning) {
      setIsScanning(true);
      setTimeout(() => {
        setIsScanning(false);
        setTimeout(() => setIsAIPanelOpen(true), 300);
      }, 2500);
    } else if (isAIPanelOpen) {
      setIsAIPanelOpen(false);
    }
  };

  const filteredNavGroups = getVisibleAdminNavGroups({ user, can });
  const canManageSubscription = can("module:admin") && can("settings:manage_general");
  // O indicador conta o estado do plano (teste com prazo, plano ativo, upgrade
  // disponível) em vez de repetir "Assine o Pro" para quem já está no Pro.
  const planIndicator = canManageSubscription ? planIndicatorView(access) : null;

  // `h-dvh` (dynamic viewport height) em vez de `h-screen` (100vh): no mobile o
  // 100vh é maior que a área visível enquanto a barra de endereço aparece, então
  // a página inteira rolava e empurrava o cabeçalho e o input do chat para fora
  // da tela. Com dvh o shell acompanha o viewport visível.
  return (
    <div className="flex h-dvh w-full overflow-hidden bg-muted text-foreground">
      <a
        href="#admin-content"
        className="sr-only z-[60] rounded-lg bg-card px-4 py-2 font-semibold text-foreground shadow-lg focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        Ir para o conteúdo
      </a>
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-foreground/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
          aria-hidden="true"
        />
      )}

      <aside
        ref={sidebarRef}
        id="admin-sidebar"
        aria-label="Menu de navegação"
        aria-hidden={!isDesktop && !sidebarOpen ? true : undefined}
        inert={!isDesktop && !sidebarOpen ? true : undefined}
        role={mobileNavOpen ? "dialog" : undefined}
        aria-modal={mobileNavOpen ? true : undefined}
        className={clsx(
          "fixed inset-y-0 left-0 z-50 border-r transform transition-all duration-300 ease-in-out flex flex-col bg-card border-border",
          sidebarOpen ? "translate-x-0 w-64" : "-translate-x-full w-64",
          "lg:relative lg:translate-x-0",
          isExpanded ? "lg:w-64" : isMinimal ? "lg:w-16" : "lg:w-20",
        )}
      >
        <div
          className={clsx(
            "flex items-center h-16 border-b border-border transition-all duration-300",
            isExpanded ? "px-6 justify-between" : "px-6 justify-between lg:px-0 lg:justify-center",
          )}
        >
          <div
            className={clsx(
              "flex items-center overflow-hidden transition-all duration-300 whitespace-nowrap",
              isExpanded ? "w-auto opacity-100" : "lg:w-0 lg:opacity-0",
            )}
          >
            <CompanyLogo
              logoUrl={theme.logoUrl || null}
              alt="Logo da empresa"
              className="h-8 w-auto object-contain shrink-0"
            />
          </div>

          <Button
            variant="ghost"
            size="icon"
            data-sidebar-close
            onClick={() => setSidebarOpen(false)}
            className="lg:hidden size-9 shrink-0 text-muted-foreground hover:text-foreground"
            aria-label="Fechar menu"
            title="Fechar menu"
          >
            <X className="w-5 h-5" />
          </Button>

          <Button
            variant="ghost"
            size="icon"
            onClick={() => setIsExpanded(!isExpanded)}
            className="hidden lg:flex size-9 shrink-0 text-muted-foreground hover:text-primary hover:bg-primary/10"
            title={isExpanded ? "Recolher menu" : "Expandir menu"}
          >
            {isExpanded ? <PanelLeftClose className="w-5 h-5" /> : <PanelLeft className="w-5 h-5" />}
          </Button>
        </div>

        <nav className={clsx("flex-1 py-6 space-y-6 overflow-y-auto overflow-x-hidden scrollbar-none", isMinimal ? "px-2" : "px-3")}>
          {planIndicator && (
            <SidebarPlanIndicator
              view={planIndicator}
              expanded={isExpanded}
              onNavigate={() => setSidebarOpen(false)}
            />
          )}
          {filteredNavGroups.map((group, index) => {
            const groupId = group.id ?? `nav-group-${index}`;
            const isActiveGroup = group.items.some((item) =>
              location.pathname === item.to || location.pathname.startsWith(`${item.to}/`),
            );
            const storedOpen = group.id ? openNavGroups[group.id] : undefined;
            const groupOpen = !group.collapsible || !isExpanded || (storedOpen ?? isActiveGroup);
            return (
              <div key={groupId} className="flex flex-col space-y-1">
                {group.title && !group.collapsible && (
                  <div
                    className={clsx(
                      "px-3 text-xs font-semibold mb-1 transition-all duration-300 text-muted-foreground",
                      isExpanded ? "opacity-100" : "lg:opacity-0 lg:h-0 lg:overflow-hidden",
                    )}
                  >
                    {group.title}
                  </div>
                )}
                {group.title && group.collapsible && isExpanded && (
                  <button
                    type="button"
                    onClick={() => setOpenNavGroups((current) => ({ ...current, [groupId]: !groupOpen }))}
                    aria-expanded={groupOpen}
                    aria-controls={`${groupId}-items`}
                    aria-label={`${groupOpen ? "Recolher" : "Expandir"} ${group.title} (${group.items.length} itens)`}
                    title={`${groupOpen ? "Recolher" : "Expandir"} ${group.title}`}
                    className={clsx(
                      "group flex w-full items-center justify-between gap-2 rounded-xl border px-3 py-2.5 text-left transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                      groupOpen
                        ? "border-border bg-card shadow-sm"
                        : "border-border/80 bg-muted/70 hover:border-primary-200 hover:bg-muted",
                    )}
                  >
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="flex size-5 shrink-0 items-center justify-center rounded-md border border-border bg-card text-muted-foreground shadow-sm">
                        <ChevronDown className={clsx("h-3.5 w-3.5 transition-transform", !groupOpen && "-rotate-90")} />
                      </span>
                      <span className="truncate text-xs font-bold text-foreground">{group.title}</span>
                    </span>
                    <span className="flex shrink-0 items-center gap-1.5">
                      <span className="rounded-full bg-muted px-1.5 py-0.5 text-caption font-bold text-muted-foreground">{group.items.length}</span>
                      <span className="text-caption font-bold text-primary-700"></span>
                    </span>
                  </button>
                )}
                {groupOpen && (
                  <div id={`${groupId}-items`} className="flex flex-col space-y-1">
                    {group.items.map((item) => {
                      return (
                        <NavLink
                          key={item.to}
                          to={item.to}
                          onClick={() => setSidebarOpen(false)}
                          className={({ isActive }) =>
                            clsx(
                              "flex items-center rounded-lg text-sm font-medium transition-colors relative group",
                              isExpanded
                                ? "px-3 py-2.5 gap-3"
                                : isMinimal
                                  ? "lg:justify-center lg:px-0 lg:py-3 px-3 py-2.5 gap-3"
                                  : "lg:justify-center lg:px-0 lg:py-3 px-3 py-2.5 gap-3",
                              isActive ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                            )
                          }
                          title={!isExpanded ? item.label : undefined}
                        >
                          <item.icon className="w-5 h-5 shrink-0" />
                          <span
                            className={clsx(
                              "transition-all duration-300 whitespace-nowrap",
                              isExpanded ? "opacity-100 w-auto" : "lg:w-0 lg:opacity-0 opacity-100 w-auto",
                            )}
                          >
                            {item.label}
                          </span>
                        </NavLink>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </nav>

        <div className="p-4 border-t border-border overflow-hidden">
          <div
            className={clsx(
              "flex items-center transition-all duration-300",
              isExpanded ? "gap-3" : "lg:flex-col lg:gap-3 lg:justify-center gap-3",
            )}
          >
            <UserAvatar
              name={user?.fullname || user?.firstname || "Usuário"}
              avatarUrl={user?.avatar_url || null}
              className={clsx(
                "bg-primary-100 text-primary-700 font-bold shrink-0 transition-all duration-300 overflow-hidden",
                isExpanded ? "w-10 h-10 text-sm" : "lg:w-8 lg:h-8 w-10 h-10 text-xs",
              )}
              fallbackClassName="bg-primary-100 text-primary-700 font-bold"
            />
            <div
              className={clsx(
                "flex-1 min-w-0 transition-all duration-300",
                isExpanded ? "opacity-100 w-auto block" : "lg:hidden opacity-100 w-auto block",
              )}
            >
              <p className="text-sm font-medium truncate text-foreground">{user?.fullname || user?.firstname || "Usuário"}</p>
              <p className="text-xs truncate text-muted-foreground">{user?.email || "usuario@suitee.com"}</p>
            </div>
            {companies.length > 1 && (
              <Button
                variant="ghost"
                size="icon"
                onClick={() => navigate("/company-selection")}
                className={clsx(
                  "size-9 shrink-0 text-muted-foreground hover:text-primary-700 hover:bg-primary-100",
                  !isExpanded && "lg:w-full",
                )}
                title="Trocar empresa"
                aria-label="Trocar empresa"
              >
                <Building2 className="w-4 h-4" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              onClick={() => logout()}
              className={clsx(
                "size-9 shrink-0 text-muted-foreground hover:text-destructive hover:bg-destructive/10",
                !isExpanded && "lg:w-full",
              )}
              title="Sair"
            >
              <LogOut className="w-4 h-4" />
            </Button>
          </div>
        </div>
      </aside>

      <div
        className="flex-1 flex flex-col min-w-0 overflow-hidden relative"
        inert={mobileNavOpen ? true : undefined}
        aria-hidden={mobileNavOpen ? true : undefined}
      >
        <header className="flex items-center justify-between h-16 px-4 sm:px-6 lg:px-8 border-b border-border bg-card z-40 shrink-0">
          <div className="flex items-center gap-2 sm:gap-4 flex-1 min-w-0">
            <Button
              ref={menuButtonRef}
              variant="ghost"
              size="icon"
              className="lg:hidden -ml-2 size-9 text-muted-foreground"
              onClick={() => setSidebarOpen(true)}
              aria-label="Abrir menu de navegação"
              aria-expanded={sidebarOpen}
              aria-controls="admin-sidebar"
            >
              <Menu className="w-5 h-5" />
            </Button>

            <TooltipProvider delayDuration={200}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label="Buscar"
                    className="sm:hidden size-9 text-muted-foreground"
                    onClick={() => setSearchOpen(true)}
                  >
                    <Search className="w-5 h-5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="bottom" sideOffset={8}>
                  Buscar (⌘K)
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <button
              type="button"
              onClick={() => setSearchOpen(true)}
              className="hidden sm:flex items-center max-w-md w-full relative text-left"
              aria-label="Abrir busca geral"
            >
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 pointer-events-none" />
              <span className="w-full pl-10 pr-16 py-2 border-transparent rounded-lg text-sm bg-muted text-muted-foreground hover:bg-muted/80 transition-colors truncate">
                Buscar leads, imóveis, corretores, negócios...
              </span>
              <kbd className="pointer-events-none absolute right-3 hidden sm:inline-flex items-center gap-1 rounded border border-border bg-card px-1.5 py-0.5 text-caption font-mono font-medium text-muted-foreground">
                ⌘K
              </kbd>
            </button>
          </div>

          <div className="flex items-center gap-2 sm:gap-4">
            <TooltipProvider delayDuration={200}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    aria-label={canUseAIPlan ? "Assistente de inteligência artificial" : "Conheça o plano Pro para usar a inteligência artificial"}
                    aria-expanded={isAIPanelOpen}
                    onClick={handleAIToggle}
                    disabled={isScanning}
                    className={clsx(
                      // `p-1.5` + marca de 28px mantém o botão nos MESMOS 40px de
                      // altura dos ícones vizinhos: a marca fica mais rica sem
                      // mexer no ritmo da barra.
                      "relative p-1.5 rounded-full transition-all duration-500 flex items-center justify-center group",
                      isAIPanelOpen || isScanning ? "text-primary" : "text-muted-foreground hover:text-primary",
                      !isAIPanelOpen && !isScanning && "hover:animate-living-glow",
                      isScanning && "animate-ai-pulse",
                    )}
                  >
                    <SuiteeIntelligenceMark
                      size="sm"
                      tone="prominent"
                      locked={!canUseAIPlan}
                      live={isAIPanelOpen || isScanning}
                      pulse={isScanning ? "always" : "hover"}
                      overlay
                      shimmer
                    />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom" sideOffset={8}>
                  {!canUseAIPlan ? "Suitee Intelligence — conheça o plano Pro" : isScanning ? "Analisando tela..." : "Suitee Intelligence (⌘J)"}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <TooltipProvider delayDuration={200}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={toggleFocusMode}
                    // O tooltip não é nome acessível: sem aria-label, leitor de
                    // tela anuncia só "botão" e o ícone não diz o que faz.
                    aria-label={focusMode ? "Desativar Modo Foco" : "Ativar Modo Foco"}
                    aria-pressed={focusMode}
                    className={clsx(
                      "relative size-9 rounded-full",
                      focusMode ? "bg-primary/15 text-primary hover:bg-primary/15" : "text-muted-foreground",
                    )}
                  >
                    {focusMode ? <Focus className="w-5 h-5" /> : <Focus className="w-5 h-5 opacity-40" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="bottom" sideOffset={8}>
                  {focusMode ? "Desativar Modo Foco" : "Ativar Modo Foco"}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <NotificationBell userId={user?.id} />
            <SystemNotificationPermissionPrompt />
            {!location.pathname.startsWith("/leads") && (
              <Button onClick={() => navigate("/leads")} className="hidden sm:flex" variant="default">
                <Plus className="w-4 h-4" />
                Novo Lead
              </Button>
            )}
          </div>
        </header>

        <Dialog open={searchOpen} onOpenChange={handleSearchOpenChange}>
          <DialogContent className="overflow-hidden p-0 sm:max-w-lg" data-testid="global-search-dialog">
            <DialogHeader className="sr-only">
              <DialogTitle>Busca geral</DialogTitle>
              <DialogDescription>Busque por leads, imóveis, corretores e negócios</DialogDescription>
            </DialogHeader>
            <Command
              shouldFilter={false}
              className="[&_[cmdk-group-heading]]:text-muted-foreground **:data-[slot=command-input-wrapper]:h-12 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group]]:px-2 [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-input-wrapper]_svg]:h-5 [&_[cmdk-input-wrapper]_svg]:w-5 [&_[cmdk-input]]:h-12 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-3 [&_[cmdk-item]_svg]:h-5 [&_[cmdk-item]_svg]:w-5"
            >
              <CommandInput
                placeholder="Buscar leads, imóveis, corretores, negócios..."
                value={searchQuery}
                onValueChange={setSearchQuery}
              />
              <CommandList className="max-h-[60vh] sm:max-h-[420px]">
                {!search.enabled && <CommandEmpty>Digite ao menos 2 caracteres para buscar.</CommandEmpty>}
                {search.enabled && search.isFetching && !hasAnySearchResult && (
                  <div className="py-6 text-center text-sm text-muted-foreground">Buscando…</div>
                )}
                {search.isEmpty && <CommandEmpty>Nenhum resultado para "{search.debouncedTerm}".</CommandEmpty>}

                {search.groups.leads.length > 0 && (
                  <CommandGroup heading="Leads">
                    {search.groups.leads.map((r) => (
                      <CommandItem key={`lead-${r.id}`} value={`lead-${r.id}`} onSelect={() => handleSearchSelect(r.route)}>
                        <Users className="size-4 text-muted-foreground" />
                        <div className="flex min-w-0 flex-col">
                          <span className="truncate">{r.title}</span>
                          {r.subtitle && <span className="truncate text-xs text-muted-foreground">{r.subtitle}</span>}
                        </div>
                        {r.badge && <CommandShortcut>{r.badge}</CommandShortcut>}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}

                {search.groups.properties.length > 0 && (
                  <CommandGroup heading="Imóveis">
                    {search.groups.properties.map((r) => (
                      <CommandItem
                        key={`property-${r.id}`}
                        value={`property-${r.id}`}
                        onSelect={() => handleSearchSelect(r.route)}
                      >
                        <Building2 className="size-4 text-muted-foreground" />
                        <div className="flex min-w-0 flex-col">
                          <span className="truncate">{r.title}</span>
                          {r.subtitle && <span className="truncate text-xs text-muted-foreground">{r.subtitle}</span>}
                        </div>
                        {r.badge && <CommandShortcut>{r.badge}</CommandShortcut>}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}

                {search.groups.brokers.length > 0 && (
                  <CommandGroup heading="Corretores">
                    {search.groups.brokers.map((r) => (
                      <CommandItem
                        key={`broker-${r.id}`}
                        value={`broker-${r.id}`}
                        onSelect={() => handleSearchSelect(r.route)}
                      >
                        <UserCog className="size-4 text-muted-foreground" />
                        <div className="flex min-w-0 flex-col">
                          <span className="truncate">{r.title}</span>
                          {r.subtitle && <span className="truncate text-xs text-muted-foreground">{r.subtitle}</span>}
                        </div>
                        {r.badge && <CommandShortcut>{r.badge}</CommandShortcut>}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}

                {search.groups.deals.length > 0 && (
                  <CommandGroup heading="Negócios">
                    {search.groups.deals.map((r) => (
                      <CommandItem key={`deal-${r.id}`} value={`deal-${r.id}`} onSelect={() => handleSearchSelect(r.route)}>
                        <Handshake className="size-4 text-muted-foreground" />
                        <div className="flex min-w-0 flex-col">
                          <span className="truncate">{r.title}</span>
                          {r.subtitle && <span className="truncate text-xs text-muted-foreground">{r.subtitle}</span>}
                        </div>
                        {r.badge && <CommandShortcut>{r.badge}</CommandShortcut>}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}
              </CommandList>
            </Command>
          </DialogContent>
        </Dialog>

        <div className="flex-1 flex overflow-hidden relative">
          <main id="admin-content" tabIndex={-1} className="flex-1 overflow-auto relative focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary/60">
            <AcceptLeadModal userId={user?.id} isAuthenticated />
            <TermsAcceptanceModal policyPath="/privacy" termsPath="/terms" onLogout={logout} />
            <Outlet />

            <div
              className={clsx(
                "absolute inset-0 z-50 pointer-events-none overflow-hidden transition-opacity duration-1000 ease-in-out",
                isScanning ? "opacity-100" : "opacity-0",
              )}
            >
              <div className="absolute inset-0 bg-background/5 dark:bg-background/2 backdrop-blur-[2px]" />

              <div
                className="absolute inset-0 shadow-[inset_0_0_100px_hsl(var(--primary)_/_0.2)] dark:shadow-[inset_0_0_120px_hsl(var(--primary)_/_0.15)] animate-pulse"
                style={{ animationDuration: "3s" }}
              />

              <div className="absolute inset-0 opacity-80 dark:opacity-60 mix-blend-screen dark:mix-blend-lighten">
                <div className="absolute -top-[20%] -right-[10%] w-[60%] h-[60%] bg-gradient-to-b from-primary/80 via-accent/80 to-primary/40 rounded-full blur-[100px] animate-aurora-1" />
                <div className="absolute -top-[10%] -left-[10%] w-[50%] h-[50%] bg-gradient-to-br from-accent/80 via-primary/80 to-accent/40 rounded-full blur-[120px] animate-aurora-2" />
                <div className="absolute -bottom-[20%] -left-[10%] w-[60%] h-[60%] bg-gradient-to-tr from-primary/80 via-secondary/80 to-accent/80 rounded-full blur-[100px] animate-aurora-3" />
                <div className="absolute -bottom-[10%] -right-[10%] w-[50%] h-[50%] bg-gradient-to-tl from-accent/80 via-primary/80 to-primary/40 rounded-full blur-[120px] animate-aurora-4" />
              </div>

              <div className="absolute inset-2 rounded-[2rem] border border-primary/20 dark:border-primary/10 shadow-[inset_0_0_60px_hsl(var(--primary)_/_0.1)] dark:shadow-[inset_0_0_60px_hsl(var(--primary)_/_0.05)]" />

              <div
                className={clsx(
                  "absolute top-10 left-1/2 -translate-x-1/2 transition-all duration-700 ease-[cubic-bezier(0.16,1,0.3,1)]",
                  isScanning ? "opacity-100 translate-y-0 scale-100 blur-none" : "opacity-0 -translate-y-4 scale-95 blur-sm",
                )}
              >
                <div className="flex items-center gap-3.5 px-6 py-3 bg-background/80 dark:bg-card/80 backdrop-blur-2xl border border-border/50 rounded-full shadow-[0_16px_40px_-12px_hsl(var(--primary)_/_0.2)] dark:shadow-[0_16px_40px_-12px_hsl(var(--primary)_/_0.3)]">
                  <div className="relative flex items-center justify-center">
                    <Sparkles className="w-4 h-4 text-primary relative z-10" />
                    <div className="absolute inset-[-4px] rounded-full border border-primary/30 animate-ai-pulse" />
                    <div className="absolute inset-0 bg-primary/20 blur-md rounded-full animate-ai-core" />
                  </div>
                  <span className="text-sm font-medium tracking-tight bg-gradient-to-r from-primary to-primary/70 bg-clip-text text-transparent">
                    Analisando o contexto da tela...
                  </span>
                </div>
              </div>
            </div>
          </main>

          {/* P1/A8: o servidor exige `ai:use` nas rotas do assistente; o painel
              só aparece para quem tem a permissão (antes aparecia para todos e o
              botão viraria 403 depois de uma revogação por tenant). */}
          {canUseAI && (isAIPanelOpen || hasOpenedAI) && <Suspense fallback={<p role="status">Carregando assistente…</p>}><AIChat isOpen={isAIPanelOpen} onClose={() => setIsAIPanelOpen(false)} /></Suspense>}
        </div>
      </div>
    </div>
  );
}
