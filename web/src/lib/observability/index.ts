/**
 * Lightweight browser observability.
 *
 * In dev, mirrors uncaught errors and unhandled promise rejections to the
 * console with a recognizable prefix. In prod it is a no-op sink — wire a real
 * sink (Sentry / OTel web SDK) by passing a custom {@link Reporter} to
 * {@link installObservers}.
 *
 * An OTel web SDK integration is intentionally NOT pulled in here to keep the
 * bundle small; it should be lazy-loaded behind `VITE_OTEL=1` and listed in
 * `optionalDependencies` when adopted. See {@link lazyOtelReporter}.
 */

/** Minimal sink for an uncaught error/rejection. */
export interface Reporter {
  report(error: unknown, context: { source: "window.onerror" | "unhandledrejection"; message: string }): void;
}

/** Console mirror used in development builds. */
export const devConsoleReporter: Reporter = {
  report(error, ctx) {
    // eslint-disable-next-line no-console
    console.error(`[observability:${ctx.source}]`, ctx.message, error);
  },
};

const noopReporter: Reporter = { report: () => {} };

let installed = false;

/**
 * Install global error + rejection handlers. Idempotent. Defaults to the dev
 * console reporter in dev and a no-op in prod unless an explicit `reporter`
 * is supplied.
 */
export function installObservers(reporter?: Reporter): () => void {
  if (installed && !reporter) return () => {};
  installed = true;

  const sink = reporter ?? (import.meta.env.DEV ? devConsoleReporter : noopReporter);

  const onError = (event: ErrorEvent) => {
    sink.report(event.error ?? event.message, {
      source: "window.onerror",
      message: event.message,
    });
  };
  const onRejection = (event: PromiseRejectionEvent) => {
    sink.report(event.reason, {
      source: "unhandledrejection",
      message: reasonToMessage(event.reason),
    });
  };

  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);

  return () => {
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
    installed = false;
  };
}

function reasonToMessage(reason: unknown): string {
  if (reason instanceof Error) return reason.message;
  if (typeof reason === "string") return reason;
  try {
    return JSON.stringify(reason);
  } catch {
    return String(reason);
  }
}

/**
 * Placeholder for a lazy OTel web SDK reporter.
 *
 * To adopt: gate on `import.meta.env.VITE_OTEL === "1"`, dynamically
 * `import('@opentelemetry/sdk')`, build a tracer, and return a Reporter that
 * records exception spans. Keeping this as a stub avoids adding the (large)
 * OTel packages to the default bundle.
 */
export async function lazyOtelReporter(): Promise<Reporter> {
  if (import.meta.env.VITE_OTEL !== "1") {
    return noopReporter;
  }
  // Dynamic import kept commented so the bundler never includes OTel today.
  // const otel = await import(/* @vite-ignore */ "@opentelemetry/sdk");
  // ...build reporter from otel...
  return noopReporter;
}
