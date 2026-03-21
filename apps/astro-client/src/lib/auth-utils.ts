import type { User } from './api';

// Utility to get user display name
export function getUserDisplayName(user: User | null): string {
  if (!user) return '';
  if (user.first_name && user.last_name) {
    return `${user.first_name} ${user.last_name}`;
  }
  if (user.first_name) return user.first_name;
  return user.email;
}

// Inverse of getUserDisplayName — splits a display name into first/last for API calls
export function splitDisplayName(name: string): { first_name: string; last_name: string } {
  const trimmed = name.trim();
  const spaceIndex = trimmed.indexOf(" ");
  if (spaceIndex === -1) return { first_name: trimmed, last_name: "" };
  return {
    first_name: trimmed.slice(0, spaceIndex),
    last_name: trimmed.slice(spaceIndex + 1),
  };
}

// Utility to get user initials
export function getUserInitials(user: User | null): string {
  if (!user) return '?';
  if (user.first_name && user.last_name) {
    return `${user.first_name[0]}${user.last_name[0]}`.toUpperCase();
  }
  if (user.first_name) return user.first_name[0].toUpperCase();
  return user.email[0].toUpperCase();
}
