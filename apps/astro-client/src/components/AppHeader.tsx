import { useState, useEffect } from "react";
import { Link, NavLink as RRNavLink, useLocation } from "react-router";
import { cn } from "@/lib/utils";
import {
  Bars3Icon,
  ArrowRightStartOnRectangleIcon,
  Cog6ToothIcon,
  WrenchScrewdriverIcon,
  SunIcon,
  MoonIcon,
  ComputerDesktopIcon,
} from "@heroicons/react/24/outline";
import { Telescope } from "lucide-react";
import astroLogo from "@/assets/astro-logo.svg";
import astroLogoDark from "@/assets/astro-logo-dark.svg";
import { useAuth } from "@/lib/auth";
import { useExperiments } from "@/lib/experiments";
import { UserCircleIcon } from "@heroicons/react/24/outline";
import { useMediaBreakpoint } from "@/hooks/use-compact-layout";
import { UserAvatar } from "@/components/UserAvatar";
import { FeedbackModal } from "@/components/FeedbackModal";
import { useTheme, type Theme } from "@/lib/theme";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { dashboardPath, explorePath } from "@/lib/routes";

interface NavItem {
  label: string;
  to: string;
  external?: boolean;
}

const publicNav: NavItem[] = [
  { label: "Blueprints", to: "/blueprints" },
];

const externalNav: NavItem[] = [
  { label: "Docs", to: "https://docs.astropods.com", external: true },
  { label: "Blog", to: "https://blog.astropods.com", external: true },
];

function Logo() {
  return (
    <>
      <img src={astroLogo} alt="Astro" className="h-6 dark:hidden" />
      <img src={astroLogoDark} alt="Astro" className="hidden h-6 dark:block" />
    </>
  );
}

function ExternalOrNavLink({ to, external, children, className }: { to: string; external?: boolean; children?: React.ReactNode; className?: string | (({ isActive }: { isActive: boolean }) => string) }) {
  if (external) {
    return <a href={to} target="_blank" rel="noopener noreferrer" className={typeof className === "function" ? className({ isActive: false }) : className}>{children}</a>;
  }
  return <RRNavLink to={to} className={className}>{children}</RRNavLink>;
}

const themeOptions: { value: Theme; icon: React.ElementType; label: string }[] = [
  { value: "light", icon: SunIcon, label: "Light" },
  { value: "dark", icon: MoonIcon, label: "Dark" },
  { value: "auto", icon: ComputerDesktopIcon, label: "System" },
];

function ThemeSwitcher() {
  const { theme, setTheme } = useTheme();
  return (
    <TooltipProvider>
      <div className="flex items-center gap-1 px-2 py-1.5">
        {themeOptions.map(({ value, icon: Icon, label }) => (
          <Tooltip key={value}>
            <TooltipTrigger asChild>
              <button
                aria-label={`Use ${label.toLowerCase()} theme`}
                onClick={(e) => {
                  e.preventDefault();
                  setTheme(value);
                }}
                className={cn(
                  "rounded p-1.5 transition-colors",
                  theme === value
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted",
                )}
              >
                <Icon className="size-4" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">{label}</TooltipContent>
          </Tooltip>
        ))}
      </div>
    </TooltipProvider>
  );
}

export function AppHeader() {
  const { user, isLoading, isAuthenticated, logout, hasPermission, personalAccount } = useAuth();
  const { experiments } = useExperiments();
  const location = useLocation();
  const isMobile = useMediaBreakpoint(1024);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const displayName = personalAccount?.display_name || personalAccount?.name || user?.email || "";

  // Close sheet on navigation
  useEffect(() => {
    setSheetOpen(false);
  }, [location.pathname]);

  // Include authenticated nav items during loading too — WaitlistGuard ensures
  // only logged-in users reach the app, so isLoading just means auth hasn't
  // resolved client-side yet. This prevents "My Agents" from popping in.
  const navItems: NavItem[] = isAuthenticated || isLoading
    ? [...publicNav, { label: "Agents", to: dashboardPath }]
    : [...publicNav];

  if (isMobile) {
    return (
      <header className="border-b border-border bg-background">
        {/* Row 1: logo | explore | hamburger */}
        <div className="flex h-14 items-center gap-2 px-4">
          <div className="flex min-w-0 items-center gap-2.5">
            <Link to="/" className="flex shrink-0 items-center">
              <Logo />
            </Link>
          </div>

          <div className="ml-auto flex items-center gap-4">
            {(isAuthenticated || isLoading) && (
              <>
                <RRNavLink to={explorePath} className="group hidden min-[700px]:flex items-center gap-1.5 whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground">
                  <Telescope className="size-4 transition-transform duration-300 group-hover:rotate-12" strokeWidth={1.5} />
                  Explore
                </RRNavLink>
                {externalNav.map((item) => (
                  <ExternalOrNavLink
                    key={item.to}
                    to={item.to}
                    external={item.external}
                    className="hidden min-[700px]:block whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {item.label}
                  </ExternalOrNavLink>
                ))}
                <button
                  type="button"
                  className="hidden min-[700px]:block cursor-pointer whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground"
                  onClick={() => setFeedbackOpen(true)}
                >
                  Feedback
                </button>
                <FeedbackModal open={feedbackOpen} onOpenChange={setFeedbackOpen} />
              </>
            )}
            {!isAuthenticated && !isLoading && (
              <>
                {/* Explore + Docs + Blog: visible at 700px+, in sheet below that */}
                <RRNavLink to={explorePath} className="group hidden min-[700px]:flex items-center gap-1.5 whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground">
                  <Telescope className="size-4 transition-transform duration-300 group-hover:rotate-12" strokeWidth={1.5} />
                  Explore
                </RRNavLink>
                {externalNav.map((item) => (
                  <ExternalOrNavLink
                    key={item.to}
                    to={item.to}
                    external={item.external}
                    className="hidden min-[700px]:block whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {item.label}
                  </ExternalOrNavLink>
                ))}
                <div className="flex items-center gap-2">
                  <Button variant="ghost" size="sm" asChild className="hidden min-[380px]:inline-flex text-[13px] font-normal">
                    <Link to="/login">Log in</Link>
                  </Button>
                  <Button size="sm" asChild>
                    <Link to="/signup">Get started</Link>
                  </Button>
                </div>
              </>
            )}
          </div>

          <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
            <SheetTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className={(!isAuthenticated && !isLoading) ? "min-[700px]:hidden" : ""}
              >
                <Bars3Icon className="size-5" />
                <span className="sr-only">Open menu</span>
              </Button>
            </SheetTrigger>
            <SheetContent side="right">
              <SheetHeader>
                <SheetTitle>Menu</SheetTitle>
              </SheetHeader>
              <div className="flex flex-col gap-1 px-4">
                {isLoading ? (
                  <Skeleton className="h-12 w-full" />
                ) : isAuthenticated && user ? (
                  <>
                    <div className="flex items-center gap-3 py-2">
                      <UserAvatar handle={personalAccount?.name ?? user.id} name={displayName} />
                      <div className="flex min-w-0 flex-col leading-tight">
                        <span className="truncate text-sm font-semibold">{displayName}</span>
                        <span className="truncate text-xs text-muted-foreground">{user.email}</span>
                      </div>
                    </div>
                    <Separator className="my-1" />
                    {personalAccount && (
                      <>
                        <Button variant="ghost" className="w-full justify-start gap-2" asChild>
                          <Link to={`/${personalAccount.name}`}>
                            <UserCircleIcon className="size-4" />
                            Profile
                          </Link>
                        </Button>
                        <Button variant="ghost" className="w-full justify-start gap-2" asChild>
                          <Link to="/settings">
                            <Cog6ToothIcon className="size-4" />
                            Settings
                          </Link>
                        </Button>
                        <Separator className="my-1" />
                      </>
                    )}
                    {hasPermission('admin:view') && (
                      <Button variant="ghost" className="w-full justify-start gap-2" asChild>
                        <Link to="/admin">
                          <WrenchScrewdriverIcon className="size-4" />
                          Admin
                        </Link>
                      </Button>
                    )}
                    <div className="flex items-center justify-between">
                      <Button variant="ghost" className="justify-start gap-2" onClick={logout}>
                        <ArrowRightStartOnRectangleIcon className="size-4" />
                        Sign out
                      </Button>
                      {experiments.theming && <ThemeSwitcher />}
                    </div>
                    <Separator className="my-1" />
                    <Button variant="ghost" className="w-full justify-start min-[700px]:hidden" asChild>
                      <Link to={explorePath}>Explore</Link>
                    </Button>
                    <Button variant="ghost" className="w-full justify-start min-[700px]:hidden" onClick={() => setFeedbackOpen(true)}>
                      Feedback
                    </Button>
                    {externalNav.map((item) => (
                      <Button key={item.to} variant="ghost" className="w-full justify-start gap-2 min-[700px]:hidden" asChild>
                        <a href={item.to} target="_blank" rel="noopener noreferrer">{item.label}</a>
                      </Button>
                    ))}
                  </>
                ) : (
                  <>
                    <Button variant="ghost" className="w-full justify-start min-[380px]:hidden" asChild>
                      <Link to="/login">Log in</Link>
                    </Button>
                    <Button variant="ghost" className="w-full justify-start min-[700px]:hidden" asChild>
                      <Link to={explorePath}>Explore</Link>
                    </Button>
                    {externalNav.map((item) => (
                      <Button key={item.to} variant="ghost" className="w-full justify-start gap-2 min-[700px]:hidden" asChild>
                        <a href={item.to} target="_blank" rel="noopener noreferrer">{item.label}</a>
                      </Button>
                    ))}
                  </>
                )}
              </div>
            </SheetContent>
          </Sheet>
        </div>

        {/* Row 2: nav tabs */}
        <nav className="flex items-center overflow-x-auto px-4 scrollbar-none">
          {navItems.map((item) => (
            <ExternalOrNavLink
              key={item.to}
              to={item.to}
              external={item.external}
              className={({ isActive }) =>
                cn(
                  "flex shrink-0 items-center whitespace-nowrap border-b-2 px-3 py-[11px] text-heading-4 transition-colors",
                  !item.external && isActive
                    ? "border-[var(--primary)] font-medium text-foreground"
                    : "border-transparent font-normal text-faint-foreground hover:text-foreground",
                )
              }
            >
              {item.label}
            </ExternalOrNavLink>
          ))}
        </nav>
      </header>
    );
  }

  return (
    <header className="flex h-14 items-center border-b border-border bg-background px-6">
      {/* Left: logo + nav links */}
      <div className="flex items-center gap-6">
        <Link to="/" className="flex shrink-0 items-center">
          <Logo />
        </Link>
        <nav className="flex items-center gap-6">
          {navItems.map((item) => (
            <ExternalOrNavLink
              key={item.to}
              to={item.to}
              external={item.external}
              className={({ isActive }) =>
                cn(
                  "whitespace-nowrap text-[13px] transition-colors",
                  !item.external && isActive
                    ? "font-medium text-foreground"
                    : "font-normal text-muted-foreground hover:text-foreground",
                )
              }
            >
              {item.label}
            </ExternalOrNavLink>
          ))}
        </nav>
      </div>

      {/* Right: external nav + auth */}
      <div className="ml-auto flex items-center gap-4">
        <RRNavLink to={explorePath} className="group flex items-center gap-1.5 whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground">
          <Telescope className="size-4 transition-transform duration-300 group-hover:rotate-12" strokeWidth={1.5} />
          Explore
        </RRNavLink>
        {externalNav.map((item) => (
          <ExternalOrNavLink
            key={item.to}
            to={item.to}
            external={item.external}
            className="whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground"
          >
            {item.label}
          </ExternalOrNavLink>
        ))}
        {(isAuthenticated || isLoading) && (
          <>
            <button
              type="button"
              className="cursor-pointer whitespace-nowrap text-[13px] font-normal text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
              onClick={() => setFeedbackOpen(true)}
              disabled={isLoading}
            >
              Feedback
            </button>
            <FeedbackModal open={feedbackOpen} onOpenChange={setFeedbackOpen} />
          </>
        )}
        <div className="flex items-center gap-2">
        {isLoading ? (
          <Skeleton className="size-8 rounded-full" />
        ) : isAuthenticated && user ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="rounded-full"
                aria-label={`User menu for ${displayName}`}
              >
                <UserAvatar handle={personalAccount?.name ?? user.id} name={displayName} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64 p-3">
              <div className="flex items-center gap-3 pb-3">
                <UserAvatar handle={personalAccount?.name ?? user.id} name={displayName} />
                <div className="flex min-w-0 flex-col leading-tight">
                  <span className="truncate text-sm font-semibold">
                    {displayName}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {user.email}
                  </span>
                </div>
              </div>
              <DropdownMenuSeparator />
              {personalAccount && (
                <>
                  <DropdownMenuItem asChild className="gap-2">
                    <Link to={`/${personalAccount.name}`}>
                      <UserCircleIcon className="size-4" />
                      Profile
                    </Link>
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild className="gap-2">
                    <Link to="/settings">
                      <Cog6ToothIcon className="size-4" />
                      Settings
                    </Link>
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                </>
              )}
              {hasPermission('admin:view') && (
                <DropdownMenuItem asChild className="gap-2">
                  <Link to="/admin">
                    <WrenchScrewdriverIcon className="size-4" />
                    Admin
                  </Link>
                </DropdownMenuItem>
              )}
              <div className="flex items-center justify-between">
                <DropdownMenuItem onClick={logout} className="gap-2">
                  <ArrowRightStartOnRectangleIcon className="size-4" />
                  Sign out
                </DropdownMenuItem>
                {experiments.theming && (
                  <>
                    <div className="w-px h-5 bg-border shrink-0" />
                    <ThemeSwitcher />
                  </>
                )}
              </div>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <>
            <Button variant="ghost" size="sm" asChild className="text-[13px] font-normal">
              <Link to="/login">Log in</Link>
            </Button>
            <Button size="sm" asChild>
              <Link to="/signup">Get started</Link>
            </Button>
          </>
        )}
        </div>
      </div>
    </header>
  );
}
