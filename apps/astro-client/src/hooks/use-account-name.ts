import { useCheckAccountName } from '@/api/queries/accounts';
import { useDebouncedValue } from '@/hooks/use-debounced-value';

export function validateAccountName(name: string, minLength = 4): string | null {
  if (name.length < minLength) return `Must be at least ${minLength} characters`;
  if (name.length > 39) return 'Must be at most 39 characters';
  if (!/^[a-z]/.test(name)) return 'Must start with a letter';
  if (name.endsWith('-')) return 'Must not end with a hyphen';
  if (/--/.test(name)) return 'Must not contain consecutive hyphens';
  if (!/^[a-z0-9-]+$/.test(name)) return 'Only lowercase letters, numbers, and hyphens';
  return null;
}

export function sanitizeAccountName(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9-]/g, '');
}

export function useAccountNameValidation(name: string, minLength = 4) {
  const clientError = name.length > 0 ? validateAccountName(name, minLength) : null;
  const shouldCheck = name.length >= minLength && !clientError;

  const debouncedName = useDebouncedValue(shouldCheck ? name : '', 300);

  const nameCheck = useCheckAccountName(debouncedName);
  const isChecking = shouldCheck && (debouncedName !== name || nameCheck.isFetching);
  const serverAvailable = nameCheck.data?.available === true && debouncedName === name;
  const serverReason =
    nameCheck.data?.available === false && debouncedName === name
      ? nameCheck.data?.reason || 'Already taken'
      : null;

  const isAvailable = shouldCheck && !isChecking && serverAvailable;
  const displayError = clientError || serverReason;

  return { isChecking, isAvailable, displayError };
}
