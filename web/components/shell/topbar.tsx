"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { LogOut, Settings as SettingsIcon, User as UserIcon } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Badge } from "@/components/ui/badge";

function useBreadcrumbs(pathname: string): { href: string; label: string }[] {
  const segs = pathname.split("/").filter(Boolean);
  if (segs.length === 0) return [{ href: "/", label: "Dashboard" }];
  const crumbs: { href: string; label: string }[] = [{ href: "/", label: "Dashboard" }];
  let path = "";
  for (const s of segs) {
    path += "/" + s;
    const label = s
      .replace(/[-_]/g, " ")
      .replace(/\[id\]/g, "Detail")
      .replace(/\b\w/g, (c) => c.toUpperCase());
    crumbs.push({ href: path, label });
  }
  return crumbs;
}

export function Topbar() {
  const pathname = usePathname();
  const { user, workspace, logout } = useAuth();
  const crumbs = useBreadcrumbs(pathname);

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b bg-background/95 px-6 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm">
        {crumbs.map((c, i) => (
          <React.Fragment key={c.href}>
            {i > 0 && <span className="text-muted-foreground">/</span>}
            {i === crumbs.length - 1 ? (
              <span className="font-medium">{c.label}</span>
            ) : (
              <Link href={c.href} className="text-muted-foreground hover:text-foreground">
                {c.label}
              </Link>
            )}
          </React.Fragment>
        ))}
      </nav>

      <div className="flex items-center gap-3">
        {workspace && (
          <div className="hidden items-center gap-2 text-sm md:flex">
            <span className="text-muted-foreground">Workspace</span>
            <span className="font-medium">{workspace.name}</span>
            <Badge variant={workspace.plan === "agency" ? "default" : "muted"} className="capitalize">
              {workspace.plan}
            </Badge>
          </div>
        )}

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="User menu">
              <UserIcon className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel className="flex flex-col">
              <span className="font-medium">{user?.email ?? "Signed in"}</span>
              <span className="text-xs font-normal text-muted-foreground">
                {workspace?.name}
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/settings" className="flex items-center">
                <SettingsIcon className="mr-2 h-4 w-4" />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => void logout()}>
              <LogOut className="mr-2 h-4 w-4" />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
