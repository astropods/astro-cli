import { useState, useEffect, useRef } from 'react';
import { useCheckAccountName } from '@/api/queries/accounts';

export function validateAccountName(name: string): string | null {
  if (name.length < 2) return 'Must be at least 2 characters';
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

export function useAccountNameValidation(name: string) {
  const [debouncedName, setDebouncedName] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const clientError = name.length > 0 ? validateAccountName(name) : null;
  const shouldCheck = name.length >= 2 && !clientError;

  useEffect(() => {
    if (!shouldCheck) {
      setDebouncedName('');
      return;
    }
    timerRef.current = setTimeout(() => setDebouncedName(name), 300);
    return () => clearTimeout(timerRef.current);
  }, [name, shouldCheck]);

  const nameCheck = useCheckAccountName(debouncedName);
  const isChecking = shouldCheck && (debouncedName !== name || nameCheck.isFetching);
  const serverAvailable = nameCheck.data?.available === true && debouncedName === name;
  const serverReason =
    nameCheck.data?.available === false && debouncedName === name
      ? (nameCheck.data as { reason?: string }).reason || 'Already taken'
      : null;

  const isAvailable = shouldCheck && !isChecking && serverAvailable;
  const displayError = clientError || serverReason;

  return { isChecking, isAvailable, displayError };
}
