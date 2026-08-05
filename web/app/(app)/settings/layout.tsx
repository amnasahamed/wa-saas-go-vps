"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";

const tabs = [
  { href: "/settings/nurture", label: "Nurture" },
  { href: "/settings/workspace", label: "Workspace" },
  { href: "/settings/api-keys", label: "API keys" },
  { href: "/settings/billing", label: "Billing" },
];

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Tune how NiosEngine talks to your leads.
        </p>
      </div>
      <nav className="flex gap-1 border-b">
        {tabs.map((t) => {
          const active = pathname === t.href;
          return (
            <Link
              key={t.href}
              href={t.href}
              className={cn(
                "border-b-2 px-4 py-2 text-sm font-medium transition-colors",
                active
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              )}
            >
              {t.label}
            </Link>
          );
        })}
      </nav>
      <div>{children}</div>
    </div>
  );
}
