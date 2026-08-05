"use client";

import * as React from "react";
import { Button } from "@/components/ui/button";

export default function ErrorBoundary({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  React.useEffect(() => {
    // eslint-disable-next-line no-console
    console.error("App error:", error);
  }, [error]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <div className="max-w-md text-center">
        <h2 className="text-2xl font-semibold">Something went wrong</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          {error.message || "An unexpected error occurred."}
        </p>
        <Button onClick={reset} className="mt-4">
          Try again
        </Button>
      </div>
    </div>
  );
}
