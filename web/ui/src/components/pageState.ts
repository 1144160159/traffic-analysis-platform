export type PageStateKind = 'loading' | 'empty' | 'partial' | 'unavailable' | 'conflict' | 'final';

type PageStateInput<T> = {
  isLoading: boolean;
  data?: T;
  error?: unknown;
  partial?: boolean;
};

function responseStatus(error: unknown): number | undefined {
  if (!error || typeof error !== 'object' || !('response' in error)) return undefined;
  const response = (error as { response?: unknown }).response;
  if (!response || typeof response !== 'object' || !('status' in response)) return undefined;
  const value = Number((response as { status?: unknown }).status);
  return Number.isInteger(value) ? value : undefined;
}

export function resolvePageState<T>({ isLoading, data, error, partial = false }: PageStateInput<T>): PageStateKind {
  if (isLoading && data === undefined) return 'loading';
  if (error && data === undefined) return responseStatus(error) === 409 ? 'conflict' : 'unavailable';
  if (data === undefined || data === null) return 'empty';
  if (partial || error) return 'partial';
  return 'final';
}
