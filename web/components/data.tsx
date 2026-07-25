"use client";

import { useCallback, useEffect, useState } from "react";
import { WarningCircle, ArrowClockwise } from "@phosphor-icons/react";
import { ApiError, apiBase } from "@/lib/api";
import { Button, Card, Skeleton } from "@/components/ui";

// useApi centralizes the load / error / retry cycle so every screen shows
// real states instead of a blank page when the backend is down.
export function useApi<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    fn()
      .then((d) => setData(d))
      .catch((e) => setError(e instanceof ApiError ? e : new ApiError(String(e), 0)))
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(load, [load]);
  return { data, error, loading, reload: load };
}

export function ErrorState({ error, onRetry }: { error: ApiError; onRetry: () => void }) {
  const offline = error.status === 0;
  return (
    <Card className="flex flex-col items-center justify-center px-6 py-16 text-center">
      <WarningCircle size={40} className="mb-3 text-amber-400" weight="duotone" />
      <div className="text-sm font-medium text-slate-200">
        {offline ? "Backend unreachable" : "Something went wrong"}
      </div>
      <div className="mt-1 max-w-md text-sm text-slate-500">
        {offline
          ? `Korugan could not reach the API at ${apiBase}. Start the backend, or set NEXT_PUBLIC_API_BASE.`
          : error.message}
      </div>
      <div className="mt-4">
        <Button variant="ghost" onClick={onRetry}>
          <ArrowClockwise size={16} /> Retry
        </Button>
      </div>
    </Card>
  );
}

// AsyncView wires loading skeleton, error state and empty-check in one place.
export function AsyncView<T>({
  state,
  skeletonRows = 6,
  children,
}: {
  state: { data: T | null; error: ApiError | null; loading: boolean; reload: () => void };
  skeletonRows?: number;
  children: (data: T) => React.ReactNode;
}) {
  if (state.loading && !state.data) return <Skeleton rows={skeletonRows} />;
  if (state.error) return <ErrorState error={state.error} onRetry={state.reload} />;
  if (!state.data) return null;
  return <>{children(state.data)}</>;
}
