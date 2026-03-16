import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { slugToTitle, useDeployForm } from './useDeployForm';
import { mockAuthContext } from '@/test/test-utils';
import { mockTemplate } from '@/test/msw/handlers';
import type { DeploymentTemplate } from '@/lib/api';
import { AuthContext } from '@/lib/auth-context';
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

describe('slugToTitle', () => {
  it('converts hyphenated slug to title case', () => {
    expect(slugToTitle('code-reviewer')).toBe('Code Reviewer');
  });

  it('converts underscored slug to title case', () => {
    expect(slugToTitle('code_reviewer')).toBe('Code Reviewer');
  });

  it('handles mixed delimiters', () => {
    expect(slugToTitle('my-agent_v2')).toBe('My Agent V2');
  });

  it('handles single word', () => {
    expect(slugToTitle('agent')).toBe('Agent');
  });

  it('handles consecutive delimiters', () => {
    expect(slugToTitle('code--reviewer')).toBe('Code Reviewer');
  });

  it('handles leading and trailing delimiters', () => {
    expect(slugToTitle('-code-reviewer-')).toBe('Code Reviewer');
  });

  it('returns empty string for empty input', () => {
    expect(slugToTitle('')).toBe('');
  });

  it('returns empty string for delimiter-only input', () => {
    expect(slugToTitle('---')).toBe('');
  });
});

/** Wrapper that includes auth context + query client + router */
function createAuthWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity, staleTime: 0 }, mutations: { retry: false } },
  });

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <AuthContext.Provider value={mockAuthContext}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>{children}</MemoryRouter>
        </QueryClientProvider>
      </AuthContext.Provider>
    );
  }

  return { wrapper: Wrapper, queryClient };
}

describe('useDeployForm with pre-filled template', () => {
  it('initializes variable values from initialValues', () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'My Agent', deployment_id: 'dep-123' },
      variables: {
        OPENAI_API_KEY: { value: 'sk-test-key-123', default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI API key' },
        SENTRY_DSN: { value: 'https://sentry.example.com/123', default: '', targets: ['agent'], secret: false, optional: true, description: 'Sentry DSN' },
      },
      interfaces: { adapters: ['web', 'slack'] },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplate: prefilledTemplate,
          skipTemplateFetch: true,
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
            variableValues: {
              OPENAI_API_KEY: 'sk-test-key-123',
              SENTRY_DSN: 'https://sentry.example.com/123',
            },
            selectedAdapters: ['web', 'slack'],
            adapterCredentials: {},
          },
        }),
      { wrapper },
    );

    expect(result.current.deployName).toBe('My Agent');
    expect(result.current.variableValues).toEqual({
      OPENAI_API_KEY: 'sk-test-key-123',
      SENTRY_DSN: 'https://sentry.example.com/123',
    });
    expect(result.current.selectedAdapters).toEqual(['web', 'slack']);
  });

  it('preserves pre-filled values after variableEntries useEffect runs', async () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'My Agent', deployment_id: 'dep-123' },
      variables: {
        OPENAI_API_KEY: { value: 'sk-test-key-123', default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI API key' },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplate: prefilledTemplate,
          skipTemplateFetch: true,
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
            variableValues: { OPENAI_API_KEY: 'sk-test-key-123' },
            selectedAdapters: ['web'],
            adapterCredentials: {},
          },
        }),
      { wrapper },
    );

    // After effects settle, values should still be there
    await act(async () => {});

    expect(result.current.variableValues.OPENAI_API_KEY).toBe('sk-test-key-123');
  });

  it('bulkSetVariables fills matching variable keys and returns matched/skipped', async () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        OPENAI_API_KEY: { value: '', default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI key' },
        SENTRY_DSN: { value: '', default: '', targets: ['agent'], secret: false, optional: true, description: 'Sentry DSN' },
        SLACK_BOT_TOKEN: { value: '', default: '', targets: ['interface.slack'], secret: true, optional: true, description: 'Slack token' },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplate: prefilledTemplate,
          skipTemplateFetch: true,
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
            variableValues: {},
            selectedAdapters: ['web'],
            adapterCredentials: {},
          },
        }),
      { wrapper },
    );

    await act(async () => {});

    let importResult: { matched: string[]; skipped: string[] };
    act(() => {
      importResult = result.current.bulkSetVariables({
        OPENAI_API_KEY: 'sk-imported-123',
        SENTRY_DSN: 'https://sentry.io/456',
        UNKNOWN_KEY: 'should-be-skipped',
        ANOTHER_UNKNOWN: 'also-skipped',
      });
    });

    expect(importResult!.matched).toContain('OPENAI_API_KEY');
    expect(importResult!.matched).toContain('SENTRY_DSN');
    expect(importResult!.matched).toHaveLength(2);
    expect(importResult!.skipped).toContain('UNKNOWN_KEY');
    expect(importResult!.skipped).toContain('ANOTHER_UNKNOWN');
    expect(importResult!.skipped).toHaveLength(2);

    expect(result.current.variableValues.OPENAI_API_KEY).toBe('sk-imported-123');
    expect(result.current.variableValues.SENTRY_DSN).toBe('https://sentry.io/456');
  });

  it('bulkSetVariables does not overwrite keys that were not imported', async () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        OPENAI_API_KEY: { value: '', default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI key' },
        SENTRY_DSN: { value: '', default: '', targets: ['agent'], secret: false, optional: true, description: 'Sentry DSN' },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplate: prefilledTemplate,
          skipTemplateFetch: true,
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
            variableValues: { SENTRY_DSN: 'existing-value' },
            selectedAdapters: ['web'],
            adapterCredentials: {},
          },
        }),
      { wrapper },
    );

    await act(async () => {});

    act(() => {
      result.current.bulkSetVariables({ OPENAI_API_KEY: 'sk-new' });
    });

    expect(result.current.variableValues.OPENAI_API_KEY).toBe('sk-new');
    expect(result.current.variableValues.SENTRY_DSN).toBe('existing-value');
  });

  it('uses template as form template when skipTemplateFetch is true', () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'My Agent', deployment_id: 'dep-123' },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplate: prefilledTemplate,
          skipTemplateFetch: true,
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
          },
        }),
      { wrapper },
    );

    expect(result.current.template).toBe(prefilledTemplate);
    expect(result.current.template?.target.deployment_id).toBe('dep-123');
  });

  it('treats overlapping slack token keys as filled when value is set in either form map', async () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        OPENAI_API_KEY: { value: '', default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI key' },
        SLACK_BOT_TOKEN: { value: '', default: '', targets: ['agent', 'interface.slack'], secret: true, optional: false, description: 'Slack token' },
      },
      interfaces: { adapters: ['slack'] },
    };

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplate: prefilledTemplate,
          skipTemplateFetch: true,
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
            variableValues: { OPENAI_API_KEY: 'sk-test-key-123' },
            selectedAdapters: ['slack'],
            adapterCredentials: { SLACK_BOT_TOKEN: 'xoxb-token' },
          },
        }),
      { wrapper },
    );

    await act(async () => {});
    expect(result.current.trySubmit()).toBe(true);
  });
});
