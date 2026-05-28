import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function formatDateTime(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export function truncateUUID(uuid: string): string {
  if (!uuid) return "-";
  if (uuid.length > 12 && uuid.includes("-")) {
    return uuid.slice(0, 8) + "\u2026";
  }
  return uuid;
}

/** Display label for cluster routing/placement ids (empty and "primary" → primary). */
export function formatClusterId(id: string | undefined): string {
  if (!id || id === "primary") return "primary";
  return id;
}

/** AWS ECR region segment from a container image reference, if present. */
export function ecrRegionFromImage(image: string): string | null {
  const match = image.match(/\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com\//i);
  return match ? match[1] : null;
}
