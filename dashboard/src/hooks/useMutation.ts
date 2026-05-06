"use client";

import { useCallback, useState } from "react";

interface MutationState<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
}

interface MutationActions<TArg, TData> {
  mutate: (arg: TArg) => Promise<TData | null>;
  reset: () => void;
}

export function useMutation<TArg = void, TData = unknown>(
  fn: (arg: TArg) => Promise<TData>
): MutationState<TData> & MutationActions<TArg, TData> {
  const [state, setState] = useState<MutationState<TData>>({
    data: null,
    loading: false,
    error: null,
  });

  const mutate = useCallback(
    async (arg: TArg): Promise<TData | null> => {
      setState({ data: null, loading: true, error: null });
      try {
        const data = await fn(arg);
        setState({ data, loading: false, error: null });
        return data;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Unknown error";
        setState({ data: null, loading: false, error: message });
        return null;
      }
    },
    [fn]
  );

  const reset = useCallback(() => {
    setState({ data: null, loading: false, error: null });
  }, []);

  return { ...state, mutate, reset };
}
