import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { slugToTitle, computeInitialValues, useDeployForm } from './useDeployForm';
import { mockAuthContext } from '@/test/test-utils';
import { mockTemplate, wrapTemplateResponse } from '@/test/msw/handlers';
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

  it('populates web auth default on first render', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {},
      interfaces: {
        auth: { web: { type: 'oidc' } },
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

    expect(result.current.webAuthEnabled).toBe(true);
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

// --- adapterDisplayFields: Slack virtual fields ---

describe('adapterDisplayFields Slack virtual fields', () => {
  it('includes virtual config fields with proper labels and placeholders', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        SLACK_BOT_TOKEN: { targets: ['interface.slack'], secret: true, optional: true, label: 'Slack Bot Token', placeholder: 'xoxb-...' },
        SLACK_APP_TOKEN: { targets: ['interface.slack'], secret: true, optional: true, label: 'Slack App Token', placeholder: 'xapp-...' },
        SLACK_CONFIG: {
          value: '{"actionable_reactions":[],"allowed_channel_ids":[],"allowed_user_ids":[]}',
          targets: ['interface.slack'],
          optional: true,
          label: 'Slack Configuration',
        },
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

    // Virtual SLACK_CONFIG fields — should have hardcoded labels, not slugToTitle output
    expect(fieldMap.SLACK_ACTIONABLE_REACTIONS?.label).toBe('Actionable Reactions');
    expect(fieldMap.SLACK_ACTIONABLE_REACTIONS?.description).toBe('Emoji names the bot acts on');
    expect(fieldMap.SLACK_ACTIONABLE_REACTIONS?.placeholder).toBe('ticket, bug');
    expect(fieldMap.SLACK_ACTIONABLE_REACTIONS?.optional).toBe(true);

    expect(fieldMap.SLACK_ALLOWED_CHANNEL_IDS?.label).toBe('Allowed Channel IDs');
    expect(fieldMap.SLACK_ALLOWED_CHANNEL_IDS?.placeholder).toBe('C12345, C67890');

    expect(fieldMap.SLACK_ALLOWED_USER_IDS?.label).toBe('Allowed User IDs');
    expect(fieldMap.SLACK_ALLOWED_USER_IDS?.placeholder).toBe('U12345, U67890');
  });

  it('does not include SLACK_CONFIG key itself in display fields', () => {
    const tpl: DeploymentTemplate = {
      ...mockTemplate,
      variables: {
        SLACK_BOT_TOKEN: { targets: ['interface.slack'], secret: true, optional: true },
        SLACK_CONFIG: {
          value: '{}',
          targets: ['interface.slack'],
          optional: true,
        },
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
    expect(keys).toContain('SLACK_ACTIONABLE_REACTIONS');
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
    const tpl = makeTemplate({ interfaces: { adapters: ['slack'] } });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.selectedAdapters).toEqual(['web']);
  });

  it('prefers response interfaces over template.interfaces', () => {
    const tpl = makeTemplate({ interfaces: { adapters: ['web'] } });
    const result = computeInitialValues(tpl, 'acme', { adapters: ['web', 'slack'] });
    expect(result.selectedAdapters).toEqual(['web', 'slack']);
  });

  it('defaults to ["web"] when response interfaces has empty adapters', () => {
    const tpl = makeTemplate({ interfaces: { adapters: [] } });
    const result = computeInitialValues(tpl, 'acme', { adapters: [] });
    expect(result.selectedAdapters).toEqual(['web']);
  });

  it('reads webAuthEnabled from response interfaces auth', () => {
    const tpl = makeTemplate({});
    const result = computeInitialValues(tpl, 'acme', { adapters: ['web'], auth: { web: { type: 'oidc' } } });
    expect(result.webAuthEnabled).toBe(true);
  });

  it('webAuthEnabled is false when response interfaces has no auth', () => {
    const tpl = makeTemplate({});
    const result = computeInitialValues(tpl, 'acme', { adapters: ['web'] });
    expect(result.webAuthEnabled).toBe(false);
  });

  it('decomposes SLACK_CONFIG into virtual adapter credential fields', () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_CONFIG: {
          value: '{"actionable_reactions":["ticket"],"allowed_channel_ids":[],"allowed_user_ids":[]}',
          targets: ['interface.slack'],
          optional: true,
        },
      },
    });
    const result = computeInitialValues(tpl, 'acme');
    expect(result.adapterCredentials?.SLACK_ACTIONABLE_REACTIONS).toBe('ticket');
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

