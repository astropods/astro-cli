import { useState, useCallback, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router';
import { useCreateAccount, useCheckAccountName } from '../api/queries/accounts';
import { useAuth } from '../lib/auth';

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

  const borderColor = name.length === 0
    ? '#d1d5db'
    : displayError
      ? '#ef4444'
      : isChecking
        ? '#d1d5db'
        : isAvailable
          ? '#22c55e'
          : '#d1d5db';

  return (
    <div style={{ maxWidth: 480, margin: '80px auto', padding: '0 24px' }}>
      <h1 style={{ fontSize: 24, fontWeight: 600, marginBottom: 8 }}>Choose your username</h1>
      <p style={{ color: '#6b7280', marginBottom: 32, lineHeight: 1.5 }}>
        Your username is how others will find your agents. It must be unique and
        can contain lowercase letters, numbers, and hyphens.
      </p>

      <form onSubmit={handleSubmit}>
        <div style={{ marginBottom: 16 }}>
          <div style={{ position: 'relative' }}>
            <input
              type="text"
              value={name}
              onChange={(e) => {
                setName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''));
                setError(null);
              }}
              placeholder="username"
              autoFocus
              maxLength={39}
              style={{
                width: '100%',
                padding: '12px 40px 12px 16px',
                fontSize: 16,
                border: `2px solid ${borderColor}`,
                borderRadius: 8,
                boxSizing: 'border-box',
                outline: 'none',
                transition: 'border-color 0.15s',
              }}
            />
            <span style={{
              position: 'absolute',
              right: 12,
              top: '50%',
              transform: 'translateY(-50%)',
              fontSize: 18,
              lineHeight: 1,
            }}>
              {name.length === 0 ? '' : isChecking ? '\u2026' : isAvailable ? '\u2713' : displayError ? '\u2717' : ''}
            </span>
          </div>

          <div style={{ minHeight: 24, marginTop: 6 }}>
            {name.length > 0 && displayError && (
              <p style={{ margin: 0, fontSize: 13, color: '#ef4444' }}>
                {displayError}
              </p>
            )}
            {isChecking && (
              <p style={{ margin: 0, fontSize: 13, color: '#9ca3af' }}>
                Checking availability...
              </p>
            )}
            {isAvailable && (
              <p style={{ margin: 0, fontSize: 13, color: '#22c55e' }}>
                Available
              </p>
            )}
          </div>
        </div>

        {error && (
          <p style={{ color: '#ef4444', marginBottom: 16, fontSize: 14 }}>{error}</p>
        )}

        <button
          type="submit"
          disabled={createAccount.isPending || !isAvailable}
          style={{
            width: '100%',
            padding: '12px 24px',
            fontSize: 16,
            fontWeight: 500,
            backgroundColor: isAvailable ? '#2563eb' : '#94a3b8',
            color: '#fff',
            border: 'none',
            borderRadius: 8,
            cursor: isAvailable ? 'pointer' : 'not-allowed',
            transition: 'background-color 0.15s',
          }}
        >
          {createAccount.isPending ? 'Creating...' : 'Claim username'}
        </button>
      </form>
    </div>
  );
}
