import type { ReactNode } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";

type IdentityBadgeLink =
  | { type: "internal"; to: string }
  | { type: "external"; href: string; rel?: string };

interface IdentityBadgeProps {
  avatar: ReactNode;
  label: string;
  link?: IdentityBadgeLink;
  display?: "flex" | "inline-flex";
  className?: string;
  labelTitle?: string | false;
}

export function IdentityBadge({
  avatar,
  label,
  link,
  display = "inline-flex",
  className,
  labelTitle,
}: IdentityBadgeProps) {
  const content = (
    <>
      {avatar}
      <span className="truncate text-foreground" title={labelTitle === false ? undefined : labelTitle ?? label}>
        {label}
      </span>
    </>
  );
  const rootClassName = cn(
    display,
    "min-w-0 items-center gap-2",
    link && "hover:underline",
    className,
  );

  if (link?.type === "internal") {
    return (
      <Link to={link.to} className={rootClassName}>
        {content}
      </Link>
    );
  }

  if (link?.type === "external") {
    return (
      <a href={link.href} rel={link.rel} className={rootClassName}>
        {content}
      </a>
    );
  }

  return <div className={rootClassName}>{content}</div>;
}
