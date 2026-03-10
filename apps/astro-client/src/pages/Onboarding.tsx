import { useState, useCallback, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { useCreateAccount } from '../api/queries/accounts';
import { useAuth } from '../lib/auth';
import { AccountNameInput } from '@/components/AccountNameInput';
import { useAccountNameValidation } from '@/hooks/use-account-name';

export default function Onboarding() {
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { checkAuth, isAuthenticated, isLoading, login } = useAuth();

  // Onboarding requires authentication (can't use ProtectedRoute here because
  // its personalAccount check would create a redirect loop back to /onboarding).
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      login();
    }
  }, [isLoading, isAuthenticated, login]);

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

  const handleChange = useCallback(
    (value: string) => {
      setName(value);
      setError(null);
    },
    []
  );

  return (
    <div className="mx-auto max-w-[480px] px-6 pt-20">
      <h1 className="text-heading-1 mb-2">Choose your username</h1>
      <p className="text-muted-foreground mb-8 leading-relaxed">
        Your username is how others will find your agents. It must be unique and
        can contain lowercase letters, numbers, and hyphens.
      </p>

      <form onSubmit={handleSubmit}>
        <div className="mb-4">
          <AccountNameInput
            value={name}
            onChange={handleChange}
            placeholder="username"
            autoFocus
            isChecking={isChecking}
            isAvailable={isAvailable}
            displayError={displayError}
          />
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
