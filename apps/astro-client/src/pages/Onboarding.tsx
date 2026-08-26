import { useState, useCallback, useEffect } from 'react';
import { useNavigate, type MetaFunction } from 'react-router';
import { useCreateAccount } from '../api/queries/accounts';
import { useAuth } from '../lib/auth';
import { AccountNameInput } from '@/components/AccountNameInput';
import { useAccountNameValidation } from '@/hooks/use-account-name';
import { usePendingFreeTrialModal } from '@/hooks/use-pending-free-trial-modal';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { DISPLAY_NAME_MAX_LENGTH } from '@/lib/constants';

export const meta: MetaFunction = () => [{ title: "Get Started | Astro" }];

export default function Onboarding() {
  const [name, setName] = useState('');
  const [nameTouched, setNameTouched] = useState(false);
  const [displayName, setDisplayName] = useState('');
  const [agreedToTerms, setAgreedToTerms] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { checkAuth, isAuthenticated, isLoading, login, user } = useAuth();
  const { markPending } = usePendingFreeTrialModal(user?.id);

  // Onboarding requires authentication but lives outside the ProtectedLayout
  // (OnboardingGuard would create a redirect loop back to /onboarding).
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      login();
    }
  }, [isLoading, isAuthenticated, login]);

  const createAccount = useCreateAccount();

  const { isChecking, isAvailable, displayError } = useAccountNameValidation(name);
  const usernameDisplayError = nameTouched && !name ? 'Username is required' : displayError;

  const displayNameTrimmed = displayName.trim();
  const isDisplayNameValid =
    displayNameTrimmed.length >= 1 &&
    displayNameTrimmed.length <= DISPLAY_NAME_MAX_LENGTH;

  const canSubmit =
    isAvailable && isDisplayNameValid && agreedToTerms && !createAccount.isPending;

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError(null);

      if (!canSubmit) return;

      try {
        await createAccount.mutateAsync({
          name,
          type: 'personal',
          display_name: displayNameTrimmed,
        });
      } catch (err: unknown) {
        const apiErr = err as { error?: string; error_description?: string };
        setError(
          apiErr.error_description || apiErr.error || 'Failed to create account'
        );
        return;
      }

      // Account created: flag the free trial modal for this first session,
      // then refresh auth state and navigate regardless. If checkAuth fails
      // the next page load will reconcile auth state.
      markPending();
      try {
        await checkAuth();
      } catch {
        // ignore — account was created successfully
      }
      navigate('/');
    },
    [name, displayNameTrimmed, canSubmit, createAccount, checkAuth, navigate, markPending]
  );

  const handleChange = useCallback(
    (value: string) => {
      setName(value);
      setError(null);
    },
    []
  );

  return (
    <div className="flex flex-col flex-1 bg-background">
    <div className="mx-auto max-w-[480px] px-6 pt-20">
      <h1 className="text-heading-1 mb-2">Set up your account</h1>
      <p className="text-muted-foreground mb-8 leading-relaxed">
        Choose a username and display name to get started. Your username is how
        others will find your agents.
      </p>

      <form onSubmit={handleSubmit}>
        <div className="mb-4">
          <Label size="md">Display name</Label>
          <Input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="Your name"
            autoFocus
            maxLength={DISPLAY_NAME_MAX_LENGTH}
          />
        </div>

        <div className="mb-4">
          <Label size="md">Username</Label>
          <AccountNameInput
            value={name}
            onChange={handleChange}
            placeholder="username"
            isChecking={isChecking}
            isAvailable={isAvailable}
            displayError={usernameDisplayError}
            onBlur={() => setNameTouched(true)}
          />
        </div>

        <div className="mb-6">
          <label className="flex items-start gap-2.5 text-sm text-foreground cursor-pointer">
            <input
              type="checkbox"
              checked={agreedToTerms}
              onChange={(e) => setAgreedToTerms(e.target.checked)}
              className="mt-0.5 size-4 shrink-0 accent-primary"
            />
            <span>
              I agree to the Astro AI{' '}
              <a href="https://www.postman.com/legal/astro-ai-terms-of-service/" target="_blank" rel="noopener noreferrer" className="text-foreground-accent underline hover:text-foreground-accent/80">
                terms of service
              </a>{' '}
              and{' '}
              <a href="https://privacy.postman.com/policies/" target="_blank" rel="noopener noreferrer" className="text-foreground-accent underline hover:text-foreground-accent/80">
                privacy policy
              </a>
            </span>
          </label>
        </div>

        {error && (
          <p className="text-destructive mb-4 text-sm">{error}</p>
        )}

        <Button
          type="submit"
          size="lg"
          disabled={!canSubmit}
          className="w-full"
        >
          {createAccount.isPending ? 'Creating...' : 'Get started'}
        </Button>
      </form>
    </div>
    </div>
  );
}
