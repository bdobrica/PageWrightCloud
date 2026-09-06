interface SubmissionPayload {
  fqdn: string;
  message: string;
  conversation_id?: string;
}

// One logical submission retains its identity while its delivery is uncertain.
// The synchronous guard also covers multiple clicks before React rerenders.
export function createSubmissionIdentity(newKey: () => string = () => crypto.randomUUID()) {
  let current: { fingerprint: string; key: string } | undefined;
  let sending = false;
  return {
    begin(payload: SubmissionPayload): string | null {
      if (sending) return null;
      const fingerprint = JSON.stringify([
        payload.fqdn, payload.message, payload.conversation_id ?? '',
      ]);
      if (current?.fingerprint !== fingerprint) current = { fingerprint, key: newKey() };
      sending = true;
      return current.key;
    },
    finish(outcome: 'success' | 'rejected' | 'uncertain') {
      sending = false;
      if (outcome !== 'uncertain') current = undefined;
    },
  };
}

export function isRejectedSubmission(error: unknown): boolean {
  if (!error || typeof error !== 'object' || !('response' in error)) return false;
  const response = error.response;
  if (!response || typeof response !== 'object' || !('data' in response)) return false;
  const data = response.data;
  return !!data && typeof data === 'object' && 'submission_state' in data && data.submission_state === 'rejected';
}
