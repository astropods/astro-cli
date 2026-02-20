import { useState, useEffect } from "react";
import { Link, useLocation } from "react-router";
import {
  Bars3Icon,
  ArrowLeftStartOnRectangleIcon,
  MagnifyingGlassIcon,
  EllipsisHorizontalIcon,
} from "@heroicons/react/24/outline";
import { Input } from "@/components/ui/input";
import astroLogo from "@/assets/astro-logo.svg";
import astroLogoDark from "@/assets/astro-logo-dark.svg";
import { useAuth, getUserDisplayName } from "@/lib/auth";
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

const publicNav = [
  { label: "Browse", to: "/hire" },
  { label: "Pricing", to: "/pricing" },
  { label: "Docs", to: "/docs" },
  { label: "Blog", to: "/blog" },
  { label: "Enterprise", to: "/enterprise" },
];

/** Number of nav items always visible; the rest collapse below `lg`. */
const ALWAYS_VISIBLE = 2;

export function AppHeader() {
  const { user, isLoading, isAuthenticated, login, logout } = useAuth();
  const location = useLocation();
  const isMobile = useIsMobile();
  const [sheetOpen, setSheetOpen] = useState(false);

  // Close sheet on navigation
  useEffect(() => {
    setSheetOpen(false);
  }, [location.pathname]);

  const mobileNavItems = isAuthenticated
    ? [publicNav[0], { label: "Home", to: "/operator" }, ...publicNav.slice(1)]
    : publicNav;

  if (isMobile) {
    return (
      <header className="flex items-center justify-between border-b border-border bg-stone-100 px-5 py-3">
        <Link to="/">
          <img src={astroLogo} alt="Astro" className="h-5 dark:hidden" />
          <img src={astroLogoDark} alt="Astro" className="hidden h-5 dark:block" />
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
              {mobileNavItems.map((item) => (
                <Link key={item.to} to={item.to}>
                  <Button
                    variant="ghost"
                    className="w-full justify-start font-normal"
                  >
                    {item.label}
                  </Button>
                </Link>
              ))}
              <Separator className="my-2" />
              {isLoading ? (
                <Skeleton className="h-12 w-full" />
              ) : isAuthenticated && user ? (
                <UserCard user={user} onSignOut={logout} />
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

  const navItems = isAuthenticated
    ? [publicNav[0], { label: "Home", to: "/operator" }, ...publicNav.slice(1)]
    : publicNav;

  return (
    <header className="flex items-center gap-1 border-b border-border bg-stone-100 px-5 py-3">
      <Link to="/" className="mr-4">
        <img src={astroLogo} alt="Astro" className="h-5 dark:hidden" />
        <img src={astroLogoDark} alt="Astro" className="hidden h-5 dark:block" />
      </Link>

      <nav className="flex items-center gap-1">
        {navItems.map((item, i) => (
          <Link
            key={item.to}
            to={item.to}
            className={i >= ALWAYS_VISIBLE ? "hidden lg:block" : undefined}
          >
            <Button variant="ghost" className="whitespace-nowrap font-normal">
              {item.label}
            </Button>
          </Link>
        ))}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="shrink-0 lg:hidden">
              <EllipsisHorizontalIcon className="size-5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {navItems.slice(ALWAYS_VISIBLE).map((item) => (
              <DropdownMenuItem key={item.to} asChild>
                <Link to={item.to}>{item.label}</Link>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </nav>

      <div className="relative ml-auto mr-2 max-w-56 flex-1">
        <MagnifyingGlassIcon className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          placeholder="Search..."
          className="h-8 border-stone-300 pl-8"
        />
      </div>

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
                <UserAvatar user={user} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64 p-3">
              <div className="flex items-center gap-3 pb-3">
                <UserAvatar user={user} />
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
              <DropdownMenuItem onClick={logout} className="gap-2">
                <ArrowLeftStartOnRectangleIcon className="size-4" />
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
    </header>
  );
}
