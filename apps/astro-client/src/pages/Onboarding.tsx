import { useState, useCallback, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router';
import { useCreateAccount, useCheckAccountName } from '../api/queries/accounts';
import { useAuth } from '../lib/auth';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

function validateName(name: string): string | null {
  if (name.length < 2) return 'Must be at least 2 characters';
  if (name.length > 39) return 'Must be at most 39 characters';
  if (!/^[a-z]/.test(name)) return 'Must start with a letter';
  if (name.endsWith('-')) return 'Must not end with a hyphen';
  if (/--/.test(name)) return 'Must not contain consecutive hyphens';
  if (!/^[a-z0-9-]+$/.test(name)) return 'Only lowercase letters, numbers, and hyphens';
  return null;
}

export default function Onboarding() {
  const [name, setName] = useState('');
  const [debouncedName, setDebouncedName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { checkAuth } = useAuth();
  const createAccount = useCreateAccount();
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const clientError = name.length > 0 ? validateName(name) : null;
  const shouldCheck = name.length >= 2 && !clientError;

  // Debounce the server check
  useEffect(() => {
    timerRef.current = setTimeout(
      () => setDebouncedName(shouldCheck ? name : ''),
      shouldCheck ? 300 : 0,
    );
    return () => clearTimeout(timerRef.current);
  }, [name, shouldCheck]);

  const nameCheck = useCheckAccountName(debouncedName);
  const isChecking = shouldCheck && (debouncedName !== name || nameCheck.isFetching);
  const serverAvailable = nameCheck.data?.available === true && debouncedName === name;
  const serverReason = nameCheck.data?.available === false && debouncedName === name
    ? (nameCheck.data as { reason?: string }).reason || 'Already taken'
    : null;

  const isAvailable = shouldCheck && !isChecking && serverAvailable;
  const displayError = clientError || serverReason;

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError(null);

      if (!isAvailable) return;

      try {
        await createAccount.mutateAsync({ name, type: 'personal' });
      } catch (err: unknown) {
        const apiErr = err as { error?: string; error_description?: string };
        setError(
          apiErr.error_description || apiErr.error || 'Failed to create account'
        );
        return;
      }

      // Account created — refresh auth state then navigate regardless.
      // If checkAuth fails the next page load will reconcile auth state.
      try {
        await checkAuth();
      } catch {
        // ignore — account was created successfully
      }
      navigate('/');
    },
    [name, isAvailable, createAccount, checkAuth, navigate]
  );

  return (
    <div className="mx-auto max-w-[480px] px-6 pt-20">
      <h1 className="text-heading-1 font-semibold mb-2">Choose your username</h1>
      <p className="text-muted-foreground mb-8 leading-relaxed">
        Your username is how others will find your agents. It must be unique and
        can contain lowercase letters, numbers, and hyphens.
      </p>

      <form onSubmit={handleSubmit}>
        <div className="mb-4">
          <div className="relative">
            <Input
              type="text"
              value={name}
              onChange={(e) => {
                setName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''));
                setError(null);
              }}
              placeholder="username"
              autoFocus
              maxLength={39}
              aria-invalid={!!displayError || undefined}
              className={cn(
                'pr-9',
                isAvailable && 'border-green-600 focus-visible:border-green-600',
              )}
            />
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-lg leading-none">
              {name.length === 0 ? '' : isChecking ? '\u2026' : isAvailable ? '\u2713' : displayError ? '\u2717' : ''}
            </span>
          </div>

          <div className="mt-1.5 min-h-6 text-xs">
            {name.length > 0 && displayError && (
              <p className="text-destructive">{displayError}</p>
            )}
            {isChecking && (
              <p className="text-muted-foreground">Checking availability...</p>
            )}
            {isAvailable && (
              <p className="text-green-600">Available</p>
            )}
          </div>
        </div>

        {error && (
          <p className="text-destructive mb-4 text-sm">{error}</p>
        )}

        <button
          type="submit"
          disabled={createAccount.isPending || !isAvailable}
          className="w-full rounded-sm px-6 py-3 font-medium text-white transition-colors bg-primary hover:bg-primary/90 disabled:bg-muted-foreground disabled:cursor-not-allowed"
        >
          {createAccount.isPending ? 'Creating...' : 'Claim username'}
        </button>
      </form>
    </div>
  );
}
