import { describe, it, expect } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { slugToTitle, computeInitialValues, useDeployForm } from './useDeployForm';
import { mockAuthContext } from '@/test/test-utils';
import { mockTemplate, wrapTemplateResponse } from '@/test/msw/handlers';
import { server } from '@/test/msw/server';
import type { DeploymentTemplate } from '@/lib/api';
import { AuthContext, type AuthContextType } from '@/lib/auth-context';
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH } from './constants';

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
function createAuthWrapper(auth: AuthContextType = mockAuthContext) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity, staleTime: 0 }, mutations: { retry: false } },
  });

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <AuthContext.Provider value={auth}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>{children}</MemoryRouter>
        </QueryClientProvider>
      </AuthContext.Provider>
    );
  }

  return { wrapper: Wrapper, queryClient };
}

describe('useDeployForm vault variable readiness', () => {
  it('hides previous account variables while the next account variables are placeholder data', async () => {
    let releaseTeamVariables: (() => void) | undefined;

    server.use(
      http.get('/api/v1/accounts/:account/variables', async ({ params }) => {
        if (params.account === 'testuser') {
          return HttpResponse.json({
            variables: [
              { name: 'ANTHROPIC_API_KEY', secret: true, description: '', created_at: '', updated_at: '' },
            ],
          });
        }
        if (params.account === 'team') {
          await new Promise<void>((resolve) => {
            releaseTeamVariables = resolve;
          });
          return HttpResponse.json({
            variables: [
              { name: 'TEAM_ANTHROPIC_API_KEY', secret: true, description: '', created_at: '', updated_at: '' },
            ],
          });
        }
        return HttpResponse.json({ variables: [] });
      }),
    );

    const auth: AuthContextType = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-personal', name: 'testuser', type: 'personal' },
        { id: 'acct-team', name: 'team', type: 'organization' },
      ],
    };

    const { wrapper } = createAuthWrapper(auth);
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(mockTemplate),
      }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.vaultEntriesLoaded).toBe(true);
    });
    expect(result.current.vaultEntries.map((entry) => entry.name)).toEqual(['ANTHROPIC_API_KEY']);

    await act(async () => {
      result.current.setTargetAccount('team');
    });

    await waitFor(() => {
      expect(result.current.targetAccount).toBe('team');
      expect(releaseTeamVariables).toBeDefined();
    });
    expect(result.current.vaultEntriesLoaded).toBe(false);
    expect(result.current.vaultEntries).toEqual([]);

    await act(async () => {
      releaseTeamVariables?.();
    });

    await waitFor(() => {
      expect(result.current.vaultEntriesLoaded).toBe(true);
    });
    expect(result.current.vaultEntries.map((entry) => entry.name)).toEqual(['TEAM_ANTHROPIC_API_KEY']);
  });

  it('keeps vault ref chips quiet but blocks submit while the next account variables are placeholder data', async () => {
    let releaseTeamVariables: (() => void) | undefined;

    server.use(
      http.get('/api/v1/accounts/:account/variables', async ({ params }) => {
        if (params.account === 'testuser') {
          return HttpResponse.json({
            variables: [
              { name: 'ANTHROPIC_API_KEY', secret: true, description: '', created_at: '', updated_at: '' },
            ],
          });
        }
        if (params.account === 'team') {
          await new Promise<void>((resolve) => {
            releaseTeamVariables = resolve;
          });
          return HttpResponse.json({
            variables: [
              { name: 'TEAM_ANTHROPIC_API_KEY', secret: true, description: '', created_at: '', updated_at: '' },
            ],
          });
        }
        return HttpResponse.json({ variables: [] });
      }),
    );

    const auth: AuthContextType = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-personal', name: 'testuser', type: 'personal' },
        { id: 'acct-team', name: 'team', type: 'organization' },
      ],
    };

    const { wrapper } = createAuthWrapper(auth);
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(mockTemplate),
        initialValues: {
          targetAccount: 'testuser',
        },
      }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.vaultEntriesLoaded).toBe(true);
    });
    await act(async () => {
      result.current.setVariableValues((prev) => ({
        ...prev,
        OPENAI_API_KEY: '{{secrets.ANTHROPIC_API_KEY}}',
      }));
    });
    expect(result.current.invalidVaultRefKeys).toEqual([]);

    await act(async () => {
      result.current.setTargetAccount('team');
    });

    await waitFor(() => {
      expect(result.current.targetAccount).toBe('team');
      expect(releaseTeamVariables).toBeDefined();
    });

    expect(result.current.vaultEntriesLoaded).toBe(false);
    expect(result.current.vaultEntries).toEqual([]);
    expect(result.current.invalidVaultRefKeys).toEqual([]);
    let valid = true;
    await act(async () => {
      valid = result.current.trySubmit();
    });
    expect(valid).toBe(false);
    expect(result.current.invalidVaultRefKeys).toEqual([]);

    await act(async () => {
      releaseTeamVariables?.();
    });

    await waitFor(() => {
      expect(result.current.vaultEntriesLoaded).toBe(true);
    });
    expect(result.current.invalidVaultRefKeys).toEqual(['OPENAI_API_KEY']);
    await act(async () => {
      valid = result.current.trySubmit();
    });
    expect(valid).toBe(false);
  });

  it('rejects a vault reference whose account-variable type does not match the field', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/variables', () => {
        return HttpResponse.json({
          variables: [
            { name: 'SHARED_KEY', secret: false, description: '', created_at: '', updated_at: '' },
          ],
        });
      }),
    );

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(mockTemplate),
      }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.vaultEntriesLoaded).toBe(true);
    });
    act(() => {
      result.current.setVariableValues((prev) => ({
        ...prev,
        OPENAI_API_KEY: '{{secrets.SHARED_KEY}}',
      }));
    });

    expect(result.current.invalidVaultRefKeys).toEqual(['OPENAI_API_KEY']);
    let valid = true;
    act(() => {
      valid = result.current.trySubmit();
    });
    expect(valid).toBe(false);
  });

  it('rejects a token prefix that misrepresents the account variable', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/variables', () => {
        return HttpResponse.json({
          variables: [
            { name: 'SHARED_KEY', secret: true, description: '', created_at: '', updated_at: '' },
          ],
        });
      }),
    );

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(mockTemplate),
      }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.vaultEntriesLoaded).toBe(true);
    });
    act(() => {
      result.current.setVariableValues((prev) => ({
        ...prev,
        OPENAI_API_KEY: '{{vars.SHARED_KEY}}',
      }));
    });

    expect(result.current.invalidVaultRefKeys).toEqual(['OPENAI_API_KEY']);
  });

  it('enables auto-fill only after fresh deploy seeding and never for configure', async () => {
    const fresh = createAuthWrapper();
    const { result: freshResult } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(mockTemplate),
      }),
      { wrapper: fresh.wrapper },
    );

    await waitFor(() => {
      expect(freshResult.current.initialValues).not.toBeNull();
      expect(freshResult.current.vaultAutoFillEnabled).toBe(true);
    });

    const configure = createAuthWrapper();
    const { result: configureResult } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(mockTemplate),
        deploymentId: 'dep-123',
      }),
      { wrapper: configure.wrapper },
    );

    await waitFor(() => {
      expect(configureResult.current.initialValues).not.toBeNull();
    });
    expect(configureResult.current.vaultAutoFillEnabled).toBe(false);
  });
});

describe('useDeployForm gateway model selector', () => {
  const gatewayResponse = {
    ...wrapTemplateResponse(mockTemplate),
    models: [{
      name: 'default',
      env_var: 'MODEL_DEFAULT',
      options: ['claude-opus-4-8', 'gpt-4o'],
      default: 'claude-opus-4-8',
      selected: 'claude-opus-4-8',
    }],
  };

  it('surfaces selectors from the response root, not as variables', async () => {
    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: gatewayResponse,
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.initialValues).not.toBeNull());

    expect(result.current.modelSelections.map((m) => m.name)).toEqual(['default']);
    expect(result.current.modelSelections[0].selected).toBe('claude-opus-4-8');
    // The model choice is not a variable — never in the credential buckets.
    expect(result.current.requiredVariables.map(([k]) => k)).not.toContain('MODEL_DEFAULT');
    expect(result.current.optionalVariables.map(([k]) => k)).not.toContain('MODEL_DEFAULT');
    expect(result.current.variableValues.MODEL_DEFAULT).toBeUndefined();
  });

  it('records a user model choice for submission', async () => {
    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: gatewayResponse,
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.initialValues).not.toBeNull());
    expect(result.current.modelChoices).toEqual({});
    act(() => result.current.setModelChoice('default', 'gpt-4o'));
    expect(result.current.modelChoices.default).toBe('gpt-4o');
  });
});

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
          initialTemplateResponse: wrapTemplateResponse(prefilledTemplate),
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

  it('preserves pre-filled values across re-renders', async () => {
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
          initialTemplateResponse: wrapTemplateResponse(prefilledTemplate),
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

    // Values should be stable across re-renders
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
          initialTemplateResponse: wrapTemplateResponse(prefilledTemplate),
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
          initialTemplateResponse: wrapTemplateResponse(prefilledTemplate),
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

  it('uses initialTemplateResponse as form template', () => {
    const prefilledTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'My Agent', deployment_id: 'dep-123' },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(prefilledTemplate),
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
          },
        }),
      { wrapper },
    );

    expect(result.current.template).not.toBeNull();
    expect(result.current.template?.target.deployment_id).toBe('dep-123');
  });

  it('hydrates existing configure values for custom interface auth and provisioning', async () => {
    const existingTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Custom Agent', deployment_id: 'dep-custom' },
      agent: {
        ...mockTemplate.agent,
        endpoints: { http: { port: 8080, expose: { enabled: true } } },
      },
      interfaces: {
        ...mockTemplate.interfaces,
        adapters: [],
        auth: { custom: { public: true, grants: [{ anyone: true }] } },
      },
      variables: {},
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          deploymentId: 'dep-custom',
          initialTemplateResponse: wrapTemplateResponse(existingTemplate, {
            interfaces: {
              adapters: [],
              auth: { custom: { public: true, grants: [{ anyone: true }] } },
            },
            provisioning: {
              agent: {
                compute: { cpu: '500m', memory: '1Gi' },
                volume: { mount: '/workspace', storage: { size: '20Gi' } },
              },
            },
          }),
        }),
      { wrapper },
    );

    expect(result.current.customSupported).toBe(true);
    expect(result.current.customPublic).toBe(true);
    expect(result.current.customGrants).toEqual([{ anyone: true }]);
    expect(result.current.selectedAdapters).toEqual([]);
    expect(result.current.agentCpu).toBe('500m');
    expect(result.current.agentMemory).toBe('1Gi');
    expect(result.current.agentVolumeMount).toBe('/workspace');
    expect(result.current.agentStorageSize).toBe('20Gi');
    expect(result.current.volumeAlreadyProvisioned).toBe(true);
    expect(result.current.trySubmit()).toBe(true);

    await waitFor(() => {
      expect(result.current.initialValues).toMatchObject({
        customPublic: true,
        selectedAdapters: [],
        agentCpu: '500m',
        agentMemory: '1Gi',
        agentVolumeMount: '/workspace',
        agentStorageSize: '20Gi',
      });
    });
  });

  it('requires a messaging adapter for existing messaging-only deployments', async () => {
    const existingTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Messaging Agent', deployment_id: 'dep-messaging' },
      variables: {},
      interfaces: {
        ...mockTemplate.interfaces,
        adapters: [],
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          deploymentId: 'dep-messaging',
          initialTemplateResponse: wrapTemplateResponse(existingTemplate, {
            interfaces: { adapters: [] },
          }),
        }),
      { wrapper },
    );

    expect(result.current.messagingSupported).toBe(true);
    expect(result.current.customSupported).toBe(false);
    expect(result.current.selectedAdapters).toEqual([]);

    let valid = true;
    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(false);
    await waitFor(() => {
      expect(result.current.errors.adapters).toBe('Select at least one messaging type');
    });
  });

  it('limits deployment display names to the shared maximum length', async () => {
    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(mockTemplate),
        }),
      { wrapper },
    );

    act(() => {
      result.current.setDeployName('a'.repeat(DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH + 1));
    });

    await waitFor(() => {
      expect(result.current.errors.deployName).toBe(`Name must be ${DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH} characters or fewer`);
    });

    let valid = true;
    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(false);
    await waitFor(() => {
      expect(result.current.errors.deployName).toBe(`Name must be ${DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH} characters or fewer`);
    });
  });

  it('counts deployment display-name length by code point', async () => {
    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(mockTemplate),
        }),
      { wrapper },
    );

    act(() => {
      result.current.setDeployName('🚀'.repeat(DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH));
    });

    await waitFor(() => {
      expect(result.current.errors.deployName).toBeUndefined();
    });

    act(() => {
      result.current.setDeployName('🚀'.repeat(DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH + 1));
    });

    await waitFor(() => {
      expect(result.current.errors.deployName).toBe(`Name must be ${DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH} characters or fewer`);
    });
  });

  it('allows empty messaging adapters when a protected custom interface has no grants', async () => {
    const existingTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Custom Agent', deployment_id: 'dep-custom' },
      agent: {
        ...mockTemplate.agent,
        endpoints: { http: { port: 8080, expose: { enabled: true } } },
      },
      variables: {},
      interfaces: {
        ...mockTemplate.interfaces,
        adapters: [],
        auth: { custom: { public: false, grants: [] } },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          deploymentId: 'dep-custom',
          initialTemplateResponse: wrapTemplateResponse(existingTemplate, {
            interfaces: {
              adapters: [],
              auth: { custom: { public: false, grants: [] } },
            },
          }),
        }),
      { wrapper },
    );

    expect(result.current.customSupported).toBe(true);
    expect(result.current.customPublic).toBe(false);
    expect(result.current.customGrants).toEqual([]);
    expect(result.current.selectedAdapters).toEqual([]);

    let valid = false;
    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(true);
    await waitFor(() => {
      expect(result.current.errors.adapters).toBeUndefined();
    });
  });

  it('counts advanced provisioning edits as deployment changes', async () => {
    const existingTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Custom Agent', deployment_id: 'dep-custom' },
      variables: {},
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          deploymentId: 'dep-custom',
          initialTemplateResponse: wrapTemplateResponse(existingTemplate, {
            provisioning: {
              agent: {
                compute: { cpu: '500m', memory: '1Gi' },
                volume: { mount: '/workspace', storage: { size: '20Gi' } },
              },
            },
          }),
        }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.initialValues?.agentMemory).toBe('1Gi');
      expect(result.current.deployChanged).toBe(false);
    });

    act(() => {
      result.current.setAgentMemory('2Gi');
    });

    await waitFor(() => {
      expect(result.current.deployChanged).toBe(true);
      expect(result.current.changeCount).toBe(1);
    });
  });

  it('keeps an existing local knowledge store selection clean until the mode changes', async () => {
    const existingTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Knowledge Agent', deployment_id: 'dep-knowledge' },
      variables: {},
      knowledge: {
        postgres: { provider: 'postgres' },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          deploymentId: 'dep-knowledge',
          initialTemplateResponse: wrapTemplateResponse(existingTemplate),
        }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.initialValues?.knowledgeBindingModes).toEqual({ postgres: 'local' });
    });
    expect(result.current.knowledgeBindings).toEqual({});
    expect(result.current.knowledgeBindingModes).toEqual({ postgres: 'local' });
    expect(result.current.deployChanged).toBe(false);
    expect(result.current.changeCount).toBe(0);

    act(() => {
      result.current.setKnowledgeBindingMode('postgres', 'shared');
    });

    await waitFor(() => {
      expect(result.current.knowledgeBindingModes.postgres).toBe('shared');
      expect(result.current.deployChanged).toBe(true);
      expect(result.current.changeCount).toBe(1);
    });

    act(() => {
      result.current.setKnowledgeBindingMode('postgres', 'local');
    });

    await waitFor(() => {
      expect(result.current.knowledgeBindingModes.postgres).toBe('local');
      expect(result.current.deployChanged).toBe(false);
      expect(result.current.changeCount).toBe(0);
    });
  });

  it('keeps an existing shared knowledge store selection clean and counts clearing it once', async () => {
    const bindingArn = 'arn:knowledge:acct:pg-store';
    const existingTemplate: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Knowledge Agent', deployment_id: 'dep-knowledge' },
      variables: {},
      knowledge: {
        postgres: { provider: 'postgres', binding: bindingArn },
      },
    };
    const response = wrapTemplateResponse(existingTemplate);
    response.bindings = {
      knowledge: {
        postgres: { arn: bindingArn, name: 'pg-store', provider: 'postgres', status: 'ready' },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          deploymentId: 'dep-knowledge',
          initialTemplateResponse: response,
        }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.initialValues?.knowledgeBindings).toEqual({ postgres: bindingArn });
    });
    expect(result.current.knowledgeBindings).toEqual({ postgres: bindingArn });
    expect(result.current.knowledgeBindingModes).toEqual({ postgres: 'shared' });
    expect(result.current.deployChanged).toBe(false);
    expect(result.current.changeCount).toBe(0);

    act(() => {
      result.current.setKnowledgeBindingMode('postgres', 'local');
    });

    await waitFor(() => {
      expect(result.current.knowledgeBindings).toEqual({});
      expect(result.current.knowledgeBindingModes.postgres).toBe('local');
      expect(result.current.deployChanged).toBe(true);
      expect(result.current.changeCount).toBe(1);
    });
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
          initialTemplateResponse: wrapTemplateResponse(prefilledTemplate),
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

describe('useDeployForm fresh deploy (no initialValues)', () => {
  it('populates select field defaults on first render without effects', () => {
    const templateWithSelect: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        ENVIRONMENT: {
          default: 'production',
          targets: ['agent'],
          'display-as': 'select',
          options: ['production', 'staging', 'development'],
        },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(templateWithSelect),
        }),
      { wrapper },
    );

    // First render — no act() needed, defaults are synchronous
    expect(result.current.variableValues.ENVIRONMENT).toBe('production');
  });

  it('populates boolean defaults on first render', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        DEBUG: { targets: ['agent'], datatype: 'boolean' },
        VERBOSE: { default: 'true', targets: ['agent'], datatype: 'boolean' },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    expect(result.current.variableValues.DEBUG).toBe('false');
    expect(result.current.variableValues.VERBOSE).toBe('true');
  });

  it('requires a messaging adapter for fresh messaging-only deploys', async () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    expect(result.current.messagingSupported).toBe(true);
    expect(result.current.customSupported).toBe(false);

    act(() => {
      result.current.setSelectedAdapters([]);
    });

    let valid = true;
    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(false);
    await waitFor(() => {
      expect(result.current.errors.adapters).toBe('Select at least one messaging type');
    });
  });

  it('allows empty messaging adapters when the agent serves a custom interface', async () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      agent: {
        ...mockTemplate.agent,
        endpoints: { http: { port: 8080, expose: { enabled: true } } },
      },
      variables: {},
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    expect(result.current.messagingSupported).toBe(true);
    expect(result.current.customSupported).toBe(true);
    expect(result.current.customPublic).toBe(false);
    // Fresh custom deploys default to the "Astro members" grant (anyone with an
    // Astro account) so the form never starts in a "no one has access" state.
    expect(result.current.customGrants).toEqual([{ anyone: true }]);

    act(() => {
      result.current.setSelectedAdapters([]);
    });

    let valid = false;
    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(true);
    await waitFor(() => {
      expect(result.current.errors.adapters).toBeUndefined();
    });
  });

  it('populates ingestion schedule defaults on first render', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
      ingestion: {
        nightly: {
          image: 'sync:latest',
          trigger: { type: 'schedule', schedule: '0 3 * * *' },
        },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    expect(result.current.ingestionSchedules).toEqual({ nightly: '0 3 * * *' });
  });

  it('reports validation error for empty schedule, passes when schedule is provided', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        OPENAI_API_KEY: { default: '', targets: ['agent'], secret: true, optional: false, description: 'API key' },
      },
      ingestion: {
        nightly: {
          image: 'sync:latest',
          trigger: { type: 'schedule', schedule: '' },
        },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
          initialValues: {
            deployName: 'My Agent',
            targetAccount: 'testuser',
            variableValues: { OPENAI_API_KEY: 'sk-filled' },
            selectedAdapters: ['web'],
            ingestionSchedules: {},
          },
        }),
      { wrapper },
    );

    // trySubmit with empty schedule → should fail
    let valid: boolean;
    act(() => {
      valid = result.current.trySubmit();
    });
    expect(valid!).toBe(false);
    expect(result.current.errors.ingestionSchedules).toEqual(['nightly']);

    // Fill the schedule
    act(() => {
      result.current.setIngestionSchedules({ nightly: '0 3 * * *' });
    });

    // trySubmit again → should pass
    act(() => {
      valid = result.current.trySubmit();
    });
    expect(valid!).toBe(true);
    expect(result.current.errors.ingestionSchedules).toBeUndefined();
  });

  it('seeds webGrants from response interfaces auth', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
      interfaces: {
        auth: { web: { grants: [{ user_id: 'user_xyz' }] } },
        adapters: ['web'],
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    expect(result.current.webGrants).toEqual([{ user_id: 'user_xyz' }]);
  });

  it('seeds adapter-specific default grants on a fresh deploy when template has none', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
      interfaces: { adapters: ['web', 'slack'] },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    // mockAuthContext.user.id is 'user-1' — web defaults to the deploying user,
    // slack defaults to anyone (workspace-wide).
    expect(result.current.webGrants).toEqual([{ user_id: 'user-1' }]);
    expect(result.current.slackGrants).toEqual([{ anyone: true }]);
  });

  it('toggling an adapter on seeds its default grants when currently empty', async () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
      interfaces: { adapters: ['web'] },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    // Slack starts off; even so, its grants are already seeded by the initial
    // effect. Clear them to simulate a user who explicitly removed everything,
    // then toggle slack on — the default should come back.
    act(() => {
      result.current.setSlackGrants([]);
    });
    expect(result.current.slackGrants).toEqual([]);

    act(() => {
      result.current.setSelectedAdapters(['web', 'slack']);
    });
    expect(result.current.slackGrants).toEqual([{ anyone: true }]);
  });

  it('does not overwrite existing grants when an adapter is toggled on', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
      interfaces: { adapters: ['web'] },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    act(() => {
      result.current.setSlackGrants([{ user_id: 'specific-user' }]);
    });

    act(() => {
      result.current.setSelectedAdapters(['web', 'slack']);
    });
    expect(result.current.slackGrants).toEqual([{ user_id: 'specific-user' }]);
  });

  it('handles null template gracefully (loader failure)', () => {
    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'my-agent'),
      { wrapper },
    );

    expect(result.current.deployName).toBe('My Agent');
    expect(result.current.variableValues).toEqual({});
    expect(result.current.selectedAdapters).toEqual(['web']);
  });

  it('reset restores computed defaults when no initialValues', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        ENVIRONMENT: {
          default: 'production',
          targets: ['agent'],
          'display-as': 'select',
          options: ['production', 'staging'],
        },
      },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(tpl),
        }),
      { wrapper },
    );

    // User changes the value
    act(() => {
      result.current.setVariableValues({ ENVIRONMENT: 'staging' });
    });
    expect(result.current.variableValues.ENVIRONMENT).toBe('staging');

    // Reset restores the computed default
    act(() => {
      result.current.reset();
    });
    expect(result.current.variableValues.ENVIRONMENT).toBe('production');
    expect(result.current.deployName).toBe('Code Reviewer');
  });
});

// --- adapterDisplayFields: object variable sub-fields ---

describe('adapterDisplayFields object variable sub-fields', () => {
  const slackConfigVar = {
    targets: ['interface.slack'] as string[],
    optional: true,
    datatype: 'object',
    fields: {
      actionable_reactions: { label: 'Actionable Reactions', description: 'Emoji names the bot acts on', placeholder: 'ticket, bug', datatype: 'csv', optional: true },
      allowed_channel_ids: { label: 'Allowed Channel IDs', description: 'Restrict to specific channels', placeholder: 'C12345, C67890', datatype: 'csv', optional: true },
      allowed_user_ids: { label: 'Allowed User IDs', description: 'Restrict to specific users', placeholder: 'U12345, U67890', datatype: 'csv', optional: true },
    },
  };

  it('expands object variable fields with server-driven labels and placeholders', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        SLACK_BOT_TOKEN: { targets: ['interface.slack'], secret: true, optional: true, label: 'Slack Bot Token', placeholder: 'xoxb-...' },
        SLACK_CONFIG: slackConfigVar,
      },
      interfaces: { adapters: ['slack'] },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(tpl, { interfaces: { adapters: ['slack'] } }),
          initialValues: { selectedAdapters: ['slack'] },
        }),
      { wrapper },
    );

    const slackFields = result.current.adapterDisplayFields.slack;
    expect(slackFields).toBeDefined();

    const fieldMap = Object.fromEntries(slackFields.map(([key, display]) => [key, display]));

    // Real server-driven fields
    expect(fieldMap.SLACK_BOT_TOKEN?.label).toBe('Slack Bot Token');
    expect(fieldMap.SLACK_BOT_TOKEN?.placeholder).toBe('xoxb-...');
    expect(fieldMap.SLACK_BOT_TOKEN?.vaultReferenceAllowed).not.toBe(false);

    // Sub-fields from SLACK_CONFIG.fields — labels driven by server schema
    expect(fieldMap['SLACK_CONFIG.actionable_reactions']?.label).toBe('Actionable Reactions');
    expect(fieldMap['SLACK_CONFIG.actionable_reactions']?.description).toBe('Emoji names the bot acts on');
    expect(fieldMap['SLACK_CONFIG.actionable_reactions']?.placeholder).toBe('ticket, bug');
    expect(fieldMap['SLACK_CONFIG.actionable_reactions']?.optional).toBe(true);
    expect(fieldMap['SLACK_CONFIG.actionable_reactions']?.vaultReferenceAllowed).toBe(false);

    expect(fieldMap['SLACK_CONFIG.allowed_channel_ids']?.label).toBe('Allowed Channel IDs');
    expect(fieldMap['SLACK_CONFIG.allowed_channel_ids']?.placeholder).toBe('C12345, C67890');

    expect(fieldMap['SLACK_CONFIG.allowed_user_ids']?.label).toBe('Allowed User IDs');
    expect(fieldMap['SLACK_CONFIG.allowed_user_ids']?.placeholder).toBe('U12345, U67890');
  });

  it('excludes the parent object variable key from display fields', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        SLACK_BOT_TOKEN: { targets: ['interface.slack'], secret: true, optional: true },
        SLACK_CONFIG: slackConfigVar,
      },
      interfaces: { adapters: ['slack'] },
    };

    const { wrapper } = createAuthWrapper();

    const { result } = renderHook(
      () =>
        useDeployForm('testuser', 'code-reviewer', {
          initialTemplateResponse: wrapTemplateResponse(tpl, { interfaces: { adapters: ['slack'] } }),
          initialValues: { selectedAdapters: ['slack'] },
        }),
      { wrapper },
    );

    const slackFields = result.current.adapterDisplayFields.slack;
    const keys = slackFields.map(([key]) => key);
    expect(keys).not.toContain('SLACK_CONFIG');
    expect(keys).toContain('SLACK_CONFIG.actionable_reactions');
  });
});

// --- computeInitialValues ---

function makeTemplate(overrides: Partial<DeploymentTemplate> = {}): DeploymentTemplate {
  return { ...mockTemplate, ...overrides };
}

describe('computeInitialValues', () => {
  it('routes agent-targeted variables to variableValues', () => {
    const tpl = makeTemplate({
      variables: {
        OPENAI_API_KEY: { value: 'sk-123', targets: ['agent'], secret: true },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.variableValues).toEqual({ OPENAI_API_KEY: 'sk-123' });
    expect(result.adapterCredentials).toEqual({});
  });

  it('routes interface-targeted variables to adapterCredentials', () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_BOT_TOKEN: { value: 'xoxb-test', targets: ['interface.slack'], secret: true },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.adapterCredentials).toEqual({ SLACK_BOT_TOKEN: 'xoxb-test' });
    expect(result.variableValues).toEqual({});
  });

  it('converts secret ref to {{secrets.NAME}}', () => {
    const tpl = makeTemplate({
      variables: {
        API_KEY: { ref: 'my-key', targets: ['agent'], secret: true },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.variableValues).toEqual({ API_KEY: '{{secrets.my-key}}' });
  });

  it('converts non-secret ref to {{vars.NAME}}', () => {
    const tpl = makeTemplate({
      variables: {
        CONFIG: { ref: 'my-config', targets: ['agent'], secret: false },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.variableValues).toEqual({ CONFIG: '{{vars.my-config}}' });
  });

  it('ref takes precedence over value', () => {
    const tpl = makeTemplate({
      variables: {
        MY_VAR: { ref: 'vault-name', value: 'direct-value', targets: ['agent'], secret: true },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.variableValues).toEqual({ MY_VAR: '{{secrets.vault-name}}' });
  });

  it('defaults to ["web"] when no response interfaces provided', () => {
    // Messaging support is keyed off the sidecar image, not interfaces presence.
    const tpl = makeTemplate({ interfaces: { image: 'messaging:latest', adapters: ['slack'] } });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.selectedAdapters).toEqual(['web']);
  });

  it('prefers response interfaces over template.interfaces', () => {
    const tpl = makeTemplate({ interfaces: { adapters: ['web'] } });
    const result = computeInitialValues(tpl, 'acme', { adapters: ['web', 'slack'] });
    expect(result.selectedAdapters).toEqual(['web', 'slack']);
  });

  it('defaults to ["web"] when response interfaces has empty adapters', () => {
    const tpl = makeTemplate({ interfaces: { image: 'messaging:latest', adapters: [] } });
    const result = computeInitialValues(tpl, 'acme', { adapters: [] });
    expect(result.selectedAdapters).toEqual(['web']);
  });

  it('preserves empty response adapters for existing deployments', () => {
    const tpl = makeTemplate({ interfaces: { image: 'messaging:latest', adapters: [] } });
    const result = computeInitialValues(tpl, 'acme', { adapters: [] }, undefined, { preserveEmptyAdapters: true });
    expect(result.selectedAdapters).toEqual([]);
  });

  it('reads webGrants from response interfaces auth', () => {
    const tpl = makeTemplate({});
    const result = computeInitialValues(tpl, 'acme', { adapters: ['web'], auth: { web: { grants: [{ user_id: 'u1' }] } } });
    expect(result.webGrants).toEqual([{ user_id: 'u1' }]);
  });

  it('webGrants is empty when response interfaces has no auth', () => {
    const tpl = makeTemplate({});
    const result = computeInitialValues(tpl, 'acme', { adapters: ['web'] });
    expect(result.webGrants).toEqual([]);
    expect(result.slackGrants).toEqual([]);
  });

  it('decomposes object variable into sub-field adapter credentials', () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_CONFIG: {
          value: '{"actionable_reactions":["ticket"],"allowed_channel_ids":[],"allowed_user_ids":[]}',
          targets: ['interface.slack'],
          optional: true,
          datatype: 'object',
          fields: {
            actionable_reactions: { label: 'Actionable Reactions', datatype: 'csv', optional: true },
            allowed_channel_ids: { label: 'Allowed Channel IDs', datatype: 'csv', optional: true },
            allowed_user_ids: { label: 'Allowed User IDs', datatype: 'csv', optional: true },
          },
        },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.adapterCredentials?.['SLACK_CONFIG.actionable_reactions']).toBe('ticket');
    expect(result.variableValues?.SLACK_CONFIG).toBeUndefined();
  });

  it('extracts ingestion schedule defaults', () => {
    const tpl = makeTemplate({
      ingestion: {
        nightly: { image: 'sync:latest', trigger: { type: 'schedule', schedule: '0 3 * * *' } },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.ingestionSchedules).toEqual({ nightly: '0 3 * * *' });
  });

  it('handles template with no variables', () => {
    const tpl = makeTemplate({ variables: undefined as unknown as DeploymentTemplate['variables'] });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.variableValues).toEqual({});
    expect(result.adapterCredentials).toEqual({});
  });

  it('uses boolean default for unset boolean variables', () => {
    const tpl = makeTemplate({
      variables: {
        DEBUG: { targets: ['agent'], datatype: 'boolean' },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.variableValues?.DEBUG).toBe('false');
  });
});

// --- useDeployForm: reset after template loads ---

describe('useDeployForm reset', () => {
  it('reset after template loads restores seeded values, not computedDefaults', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, display_name: 'Seeded Name' },
      variables: {
        OPENAI_API_KEY: { value: 'sk-seeded', targets: ['agent'], secret: true },
      },
    };

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(tpl),
      }),
      { wrapper },
    );

    // Modify a value
    act(() => {
      result.current.setVariableValues({ OPENAI_API_KEY: 'sk-changed' });
    });
    expect(result.current.variableValues.OPENAI_API_KEY).toBe('sk-changed');

    // Reset should restore seeded template values (sk-seeded), not empty computedDefaults
    act(() => {
      result.current.reset();
    });
    expect(result.current.variableValues.OPENAI_API_KEY).toBe('sk-seeded');
    expect(result.current.deployName).toBe('Seeded Name');
  });
});

// --- useDeployForm: knowledge binding wire format ---
//
// The server treats `bindings: undefined` as "no input from client → restore
// from stored spec" and any non-nil `bindings.knowledge` (even {}) as
// "explicit intent". After the user has interacted with the form, the hook
// must always send a (possibly empty) knowledge map so the server preserves
// clears. These tests pin that wire format.

describe('useDeployForm knowledge binding wire format', () => {
  // Returns the request bodies sent to the deployment-template endpoint, in
  // order. The caller registers this before triggering reshape so the
  // handler is in place when the mutation fires.
  function captureTemplateRequests() {
    const captured: Array<Record<string, unknown>> = [];
    server.use(
      http.post('/api/v1/agents/:account/:name/deployment-template', async ({ request }) => {
        const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
        captured.push(body);
        return HttpResponse.json(wrapTemplateResponse(mockTemplate, body as Parameters<typeof wrapTemplateResponse>[1]));
      }),
    );
    return captured;
  }

  it('setKnowledgeBindings({}) sends bindings: { knowledge: {} } so the server clears stored bindings', async () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, deployment_id: 'dep-123' },
    };
    const captured = captureTemplateRequests();

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(tpl),
      }),
      { wrapper },
    );

    await act(async () => {
      result.current.setKnowledgeBindings({});
    });

    await waitFor(() => expect(captured.length).toBeGreaterThan(0));
    expect(captured[0].bindings).toEqual({ knowledge: {} });
  });

  it('setKnowledgeBindings strips empty-string ARNs but still sends a knowledge map', async () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, deployment_id: 'dep-123' },
    };
    const captured = captureTemplateRequests();

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(tpl),
      }),
      { wrapper },
    );

    // postgres set to "" means "unbind"; users left bound.
    await act(async () => {
      result.current.setKnowledgeBindings({
        postgres: '',
        users: 'arn:knowledge:acct:users-store',
      });
    });

    await waitFor(() => expect(captured.length).toBeGreaterThan(0));
    expect(captured[0].bindings).toEqual({
      knowledge: { users: 'arn:knowledge:acct:users-store' },
    });
  });

  it('setSelectedAdapters with empty bindings state still sends bindings: { knowledge: {} }', async () => {
    // This guards the case where the user's first interaction is to change
    // adapters before binding anything. The hook must not collapse "no
    // bindings yet" into `bindings: undefined`, which would let the server
    // restore stored bindings on top.
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, deployment_id: 'dep-123' },
    };
    const captured = captureTemplateRequests();

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(tpl),
      }),
      { wrapper },
    );

    await act(async () => {
      result.current.setSelectedAdapters(['web', 'slack']);
    });

    await waitFor(() => expect(captured.length).toBeGreaterThan(0));
    expect(captured[0].bindings).toEqual({ knowledge: {} });
  });

  it('setSelectedAdapters preserves bindings seeded from the initial template response', async () => {
    // Configure-flow contract: the first template response can already carry
    // bindings.knowledge from the stored spec. The seeding effect hydrates
    // knowledgeBindings, and the user's first interaction (e.g. toggling an
    // adapter before touching bindings) must echo those ARNs back. If the
    // seeding/ShapeTemplate/ApplyStoredBindingsToRequest contract drifts so
    // that this reshape sends `bindings.knowledge: {}`, the server will wipe
    // the user's stored bindings on first interaction.
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, deployment_id: 'dep-123' },
    };
    const seededResp = wrapTemplateResponse(tpl);
    seededResp.bindings = {
      knowledge: {
        users: { arn: 'arn:knowledge:acct:users-store', name: 'users', provider: 'pg', status: 'ready' },
        docs: { arn: 'arn:knowledge:acct:docs-store', name: 'docs', provider: 'pg', status: 'ready' },
      },
    };
    const captured = captureTemplateRequests();

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', { initialTemplateResponse: seededResp }),
      { wrapper },
    );

    // Wait for the seeding effect to hydrate knowledgeBindings before
    // exercising setSelectedAdapters — otherwise the captured request would
    // reflect the empty initial state, not the seeded one.
    await waitFor(() => {
      expect(result.current.knowledgeBindings).toEqual({
        users: 'arn:knowledge:acct:users-store',
        docs: 'arn:knowledge:acct:docs-store',
      });
    });

    await act(async () => {
      result.current.setSelectedAdapters(['web', 'slack']);
    });

    await waitFor(() => expect(captured.length).toBeGreaterThan(0));
    expect(captured[0].bindings).toEqual({
      knowledge: {
        users: 'arn:knowledge:acct:users-store',
        docs: 'arn:knowledge:acct:docs-store',
      },
    });
  });

  it('requires a selected store when a knowledge entry is set to shared', async () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      target: { ...mockTemplate.target, deployment_id: 'dep-123' },
      variables: {},
      knowledge: {
        postgres: { provider: 'postgres' },
      },
    };

    const { wrapper } = createAuthWrapper();
    const { result } = renderHook(
      () => useDeployForm('testuser', 'code-reviewer', {
        initialTemplateResponse: wrapTemplateResponse(tpl),
      }),
      { wrapper },
    );

    await act(async () => {
      result.current.setKnowledgeBindingMode('postgres', 'shared');
    });

    let valid = true;
    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(false);
    await waitFor(() => {
      expect(result.current.errors.knowledgeBindings).toEqual(['postgres']);
    });

    await act(async () => {
      result.current.setKnowledgeBindings({
        postgres: 'arn:knowledge:testuser:shared-postgres',
      });
    });
    act(() => {
      result.current.setVariableValues({
        OPENAI_API_KEY: 'sk-shared-knowledge-test',
      });
    });

    await waitFor(() => {
      expect(result.current.errors.knowledgeBindings).toBeUndefined();
    });

    act(() => {
      valid = result.current.trySubmit();
    });

    expect(valid).toBe(true);
  });
});
