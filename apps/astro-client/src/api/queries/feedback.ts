import { useMutation } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { FeedbackInput } from '../../lib/api';

export function useSubmitFeedback() {
  return useMutation({
    mutationFn: (input: FeedbackInput) => api.submitFeedback(input),
  });
}
