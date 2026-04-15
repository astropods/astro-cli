import { useState, useEffect } from "react";
import { Link, NavLink as RRNavLink, useLocation } from "react-router";
import { cn } from "@/lib/utils";
import {
  Bars3Icon,
  ArrowRightStartOnRectangleIcon,
  BuildingOffice2Icon,
  Cog6ToothIcon,
  WrenchScrewdriverIcon,
  PlusIcon,
  ChatBubbleLeftEllipsisIcon,
  SunIcon,
  MoonIcon,
  ComputerDesktopIcon,
} from "@heroicons/react/24/outline";
import astroLogo from "@/assets/astro-logo.svg";
import astroLogoDark from "@/assets/astro-logo-dark.svg";
import { useAuth } from "@/lib/auth";
import { UserCircleIcon } from "@heroicons/react/24/outline";
import { useIsMobile } from "@/hooks/use-compact-layout";
import { UserAvatar } from "@/components/UserAvatar";
import { UserCard } from "@/components/UserCard";
import { FeedbackModal } from "@/components/FeedbackModal";
import { useExperiments } from "@/lib/experiments";
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
import { dashboardPath } from "@/lib/routes";

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
                onClick={(e) => {
                  e.preventDefault();
                  setTheme(value);
                }}
                className={cn(
                  "rounded p-1.5 transition-colors",
                  theme === value
                    ? "bg-stone-200 text-foreground dark:bg-stone-700"
                    : "text-muted-foreground hover:text-foreground hover:bg-stone-100 dark:hover:bg-stone-800",
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
  const { user, accounts, isLoading, isAuthenticated, logout, hasPermission, personalAccount } = useAuth();
  const location = useLocation();
  const isMobile = useIsMobile();
  const [sheetOpen, setSheetOpen] = useState(false);
  const [feedbackOpen, setFeedbackOpen] = useState(false);

  const { experiments } = useExperiments();
  const displayName = personalAccount?.display_name || personalAccount?.name || user?.email || "";

  // Close sheet on navigation
  useEffect(() => {
    setSheetOpen(false);
  }, [location.pathname]);

  // Include authenticated nav items during loading too — isLoading just means
  // auth hasn't resolved client-side yet. This prevents "My Agents" from
  // popping in.
  const navItems: NavItem[] = isAuthenticated || isLoading
    ? [{ label: "Dashboard", to: dashboardPath }, ...publicNav]
    : publicNav;

  if (isMobile) {
    return (
      <header className="flex h-14 items-center justify-between border-b border-border bg-surface px-6 dark:bg-background">
        <Link to="/">
          <Logo />
        </Link>

        <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon">
              <Bars3Icon className="size-5" />
              <span className="sr-only">Open menu</span>
            </Button>
          </SheetTrigger>
          <SheetContent side="right">
            <SheetHeader>
              <SheetTitle>Menu</SheetTitle>
            </SheetHeader>
            <nav className="flex flex-col gap-1 px-4">
              {[...navItems, ...externalNav].map((item) => (
                <ExternalOrNavLink key={item.to} to={item.to} external={item.external}>
                  <Button
                    variant="ghost"
                    className="w-full justify-start font-normal"
                  >
                    {item.label}
                  </Button>
                </ExternalOrNavLink>
              ))}
              <Separator className="my-2" />
              {isLoading ? (
                <Skeleton className="h-12 w-full" />
              ) : isAuthenticated && user ? (
                <UserCard user={user} displayName={displayName} handle={personalAccount?.name} onSignOut={logout} />
              ) : (
                <>
                  <Button
                    variant="ghost"
                    className="w-full justify-start"
                    asChild
                  >
                    <Link to="/login">Log in</Link>
                  </Button>
                  <Button
                    className="w-full justify-start"
                    asChild
                  >
                    <Link to="/signup">Sign up</Link>
                  </Button>
                </>
              )}
            </nav>
          </SheetContent>
        </Sheet>
      </header>
    );
  }

  return (
    <header className="flex h-14 items-center border-b border-border bg-surface px-6 dark:bg-background">
      {/* Left: logo + nav */}
      <div className="flex items-center gap-8">
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
                  "whitespace-nowrap text-[13px] font-normal transition-colors",
                  !item.external && isActive
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground",
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

        {/* <div className="relative">
          <MagnifyingGlassIcon className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            placeholder="Search"
            className="h-8 w-[168px] rounded-sm border-stone-300 pl-8 text-[13px] dark:border-input"
          />
        </div> */}

        <div className="flex items-center gap-1">
        {(isAuthenticated || isLoading) && (
          <>
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5 mr-2 text-[13px] font-normal"
              onClick={() => setFeedbackOpen(true)}
              disabled={isLoading}
            >
              <ChatBubbleLeftEllipsisIcon className="size-4" />
              Feedback
            </Button>
            <FeedbackModal open={feedbackOpen} onOpenChange={setFeedbackOpen} />
          </>
        )}
        {isLoading ? (
          <Skeleton className="size-8 rounded-full" />
        ) : isAuthenticated && user ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="rounded-full"
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
              {(() => {
                const orgs = accounts.filter((a) => a.type === "organization");
                return (
                  <>
                    {orgs.length > 0 && (
                      <div className="px-2 py-1.5 font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
                        Organizations
                      </div>
                    )}
                    {orgs.map((org) => (
                      <DropdownMenuItem key={org.id} asChild className="gap-2">
                        <Link to={`/${org.name}`}>
                          <BuildingOffice2Icon className="size-4" />
                          {org.name}
                        </Link>
                      </DropdownMenuItem>
                    ))}
                    <DropdownMenuItem asChild className="gap-2">
                      <Link to="/organization/new">
                        <PlusIcon className="size-4" />
                        Create organization
                      </Link>
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                  </>
                );
              })()}
              {hasPermission('admin:view') && (
                <DropdownMenuItem asChild className="gap-2">
                  <Link to="/admin">
                    <WrenchScrewdriverIcon className="size-4" />
                    Admin
                  </Link>
                </DropdownMenuItem>
              )}
              <DropdownMenuItem onClick={logout} className="gap-2">
                <ArrowRightStartOnRectangleIcon className="size-4" />
                Sign out
              </DropdownMenuItem>
              {experiments.theming && (
                <>
                  <DropdownMenuSeparator />
                  <ThemeSwitcher />
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <>
            <Button variant="ghost" asChild>
              <Link to="/login">Log in</Link>
            </Button>
            <Button asChild>
              <Link to="/signup">Sign up</Link>
            </Button>
          </>
        )}
        </div>
      </div>
    </header>
  );
}
