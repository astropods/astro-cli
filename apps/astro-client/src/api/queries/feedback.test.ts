import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useSubmitFeedback } from './feedback';
import { createHookWrapper } from '@/test/test-utils';

describe('useSubmitFeedback', () => {
  it('submits feedback successfully', async () => {
    server.use(
      http.post('/api/v1/feedback', () =>
        HttpResponse.json({ id: 'fb-001' }, { status: 201 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSubmitFeedback(), { wrapper });

    result.current.mutate({ message: 'Great product!', page_url: '/dashboard' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.id).toBe('fb-001');
  });

  it('surfaces error when server returns 500', async () => {
    server.use(
      http.post('/api/v1/feedback', () =>
        HttpResponse.json({ error: 'failed to submit feedback' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSubmitFeedback(), { wrapper });

    result.current.mutate({ message: 'Some feedback' });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ code: 'failed to submit feedback' });
  });

  it('surfaces rate limit error', async () => {
    server.use(
      http.post('/api/v1/feedback', () =>
        HttpResponse.json(
          { error: 'too many feedback submissions, please try again later' },
          { status: 429 },
        ),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSubmitFeedback(), { wrapper });

    result.current.mutate({ message: 'Spam' });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({
      code: 'too many feedback submissions, please try again later',
    });
  });

  it('surfaces validation error for empty message', async () => {
    server.use(
      http.post('/api/v1/feedback', () =>
        HttpResponse.json({ error: 'message is required' }, { status: 400 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSubmitFeedback(), { wrapper });

    result.current.mutate({ message: '' });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ code: 'message is required' });
  });
});
