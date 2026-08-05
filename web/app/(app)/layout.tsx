"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { Sidebar } from "@/components/shell/sidebar";
import { Topbar } from "@/components/shell/topbar";
import { Skeleton } from "@/components/ui/skeleton";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const router = useRouter();

  React.useEffect(() => {
    if (status === "anonymous") {
      router.replace("/login");
    }
  }, [status, router]);

  if (status !== "authenticated") {
    return (
      <div className="flex h-screen">
        <div className="hidden w-60 border-r bg-card md:block" />
        <div className="flex flex-1 flex-col">
          <div className="h-14 border-b" />
          <div className="flex-1 p-6">
            <Skeleton className="h-8 w-48" />
            <div className="mt-6 space-y-4">
              <Skeleton className="h-32 w-full" />
              <Skeleton className="h-32 w-full" />
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-screen bg-background">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <main className="flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}
