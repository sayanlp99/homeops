import { Component, type ReactNode } from "react";
import { reportError, resetApp } from "@/lib/crash";

/** Shows what went wrong, with a way out, instead of a blank screen. */
export class ErrorBoundary extends Component<
  { children: ReactNode },
  { error: Error | null }
> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error) {
    reportError(error);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className="flex min-h-dvh flex-col items-center justify-center gap-3 px-6 pt-safe pb-safe text-center">
        <img src="/logo.svg" alt="" className="size-10" />
        <h1 className="text-[16px] font-semibold">HomeOps hit a problem</h1>
        <p className="max-w-sm text-[13px] text-muted">
          Reloading fetches the latest version and fresh data. Anything not yet
          saved on this device may be lost.
        </p>
        <button
          onClick={resetApp}
          className="h-9 rounded-md bg-accent px-4 text-[14px] font-medium text-accent-fg"
        >
          Reload HomeOps
        </button>
        <p className="max-w-sm font-mono text-[11px] break-words text-subtle">
          {error.message}
        </p>
      </div>
    );
  }
}
