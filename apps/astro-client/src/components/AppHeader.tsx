import { useState, useEffect } from "react";
import { Link, NavLink as RRNavLink, useLocation } from "react-router";
import { cn } from "@/lib/utils";
import {
  Bars3Icon,
  ArrowRightStartOnRectangleIcon,
  BuildingOffice2Icon,
  Cog6ToothIcon,
  WrenchScrewdriverIcon,
  EllipsisHorizontalIcon,
  PlusIcon,
} from "@heroicons/react/24/outline";
import astroLogo from "@/assets/astro-logo.svg";
import astroLogoDark from "@/assets/astro-logo-dark.svg";
import { useAuth, getUserDisplayName } from "@/lib/auth";
import { UserCircleIcon } from "@heroicons/react/24/outline";
import { useIsMobile } from "@/hooks/use-mobile";
import { UserAvatar } from "@/components/UserAvatar";
import { UserCard } from "@/components/UserCard";
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

interface NavItem {
  label: string;
  to: string;
  external?: boolean;
}

const publicNav: NavItem[] = [
  { label: "Browse", to: "/browse" },
  { label: "Docs", to: "https://docs.astropods.ai", external: true },
  { label: "Blog", to: "https://blog.astropods.ai", external: true },
];

const authenticatedNav: NavItem[] = [
  { label: "Dashboard", to: "/agents" },
  ...publicNav,
];

function Logo() {
  return (
    <>
      <img src={astroLogo} alt="Astro" className="h-4 dark:hidden" />
      <img src={astroLogoDark} alt="Astro" className="hidden h-4 dark:block" />
    </>
  );
}

function ExternalOrNavLink({ to, external, children, className }: { to: string; external?: boolean; children?: React.ReactNode; className?: string | (({ isActive }: { isActive: boolean }) => string) }) {
  if (external) {
    return <a href={to} target="_blank" rel="noopener noreferrer" className={typeof className === "function" ? className({ isActive: false }) : className}>{children}</a>;
  }
  return <RRNavLink to={to} className={className}>{children}</RRNavLink>;
}

/** Number of nav items always visible; the rest collapse below `lg`. */
const ALWAYS_VISIBLE = 2;

export function AppHeader() {
  const { user, accounts, isLoading, isAuthenticated, login, logout, hasPermission, personalAccount } = useAuth();
  const location = useLocation();
  const isMobile = useIsMobile();
  const [sheetOpen, setSheetOpen] = useState(false);

  // Close sheet on navigation
  useEffect(() => {
    setSheetOpen(false);
  }, [location.pathname]);

  const navItems: NavItem[] = isAuthenticated ? authenticatedNav : publicNav;

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
              {navItems.map((item) => (
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
                <UserCard user={user} handle={personalAccount?.name} avatarVersion={personalAccount?.avatar_version} onSignOut={logout} />
              ) : (
                <>
                  <Button
                    variant="ghost"
                    className="w-full justify-start"
                    onClick={login}
                  >
                    Log in
                  </Button>
                  <Button
                    className="w-full justify-start"
                    onClick={login}
                  >
                    Sign up
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
          {navItems.map((item, i) => (
            <ExternalOrNavLink
              key={item.to}
              to={item.to}
              external={item.external}
              className={({ isActive }) =>
                cn(
                  "whitespace-nowrap text-[13px] transition-colors hover:text-foreground",
                  !item.external && isActive
                    ? "font-semibold text-primary"
                    : "font-normal text-[var(--muted-foreground)]",
                  i >= ALWAYS_VISIBLE && "hidden lg:block",
                )
              }
            >
              {item.label}
            </ExternalOrNavLink>
          ))}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="shrink-0 text-[var(--muted-foreground)] lg:hidden">
                <EllipsisHorizontalIcon className="size-5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {navItems.slice(ALWAYS_VISIBLE).map((item) => (
                <DropdownMenuItem key={item.to} asChild>
                  <ExternalOrNavLink to={item.to} external={item.external}>{item.label}</ExternalOrNavLink>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </nav>
      </div>

      {/* Right: auth */}
      <div className="ml-auto flex items-center gap-4">
        <div className="flex items-center gap-1">
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
                <UserAvatar handle={personalAccount?.name ?? user.id} name={getUserDisplayName(user)} avatarVersion={personalAccount?.avatar_version} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64 p-3">
              <div className="flex items-center gap-3 pb-3">
                <UserAvatar handle={personalAccount?.name ?? user.id} name={getUserDisplayName(user)} avatarVersion={personalAccount?.avatar_version} />
                <div className="flex min-w-0 flex-col leading-tight">
                  <span className="truncate text-sm font-semibold">
                    {getUserDisplayName(user)}
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
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <>
            <Button variant="ghost" onClick={login}>
              Log in
            </Button>
            <Button onClick={login}>
              Sign up
            </Button>
          </>
        )}
        </div>
      </div>
    </header>
  );
}
