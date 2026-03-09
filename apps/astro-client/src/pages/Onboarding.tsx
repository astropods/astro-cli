import { useState, useCallback } from 'react';
import { useNavigate } from 'react-router';
import { useCreateAccount } from '../api/queries/accounts';
import { useAuth } from '../lib/auth';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { useAccountNameValidation, sanitizeAccountName } from '@/hooks/use-account-name';

export default function Onboarding() {
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { checkAuth } = useAuth();
  const createAccount = useCreateAccount();

  const { isChecking, isAvailable, displayError } = useAccountNameValidation(name);

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
                setName(sanitizeAccountName(e.target.value));
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
