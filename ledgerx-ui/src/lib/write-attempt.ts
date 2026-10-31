export type WriteAttempt = { signature: string; key: string };

// Retry an identical payload with the original key, including after a timeout
// where the server may have committed before the response was lost.
export function nextWriteAttempt(
  previous: WriteAttempt | null,
  payload: unknown,
): WriteAttempt {
  const signature = JSON.stringify(payload);
  return previous?.signature === signature
    ? previous
    : { signature, key: crypto.randomUUID() };
}
