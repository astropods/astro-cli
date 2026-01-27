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

// Utility to get user initials
export function getUserInitials(user: User | null): string {
  if (!user) return '?';
  if (user.first_name && user.last_name) {
    return `${user.first_name[0]}${user.last_name[0]}`.toUpperCase();
  }
  if (user.first_name) return user.first_name[0].toUpperCase();
  return user.email[0].toUpperCase();
}
