"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Users,
  MessageSquare,
  Phone,
  Plug,
  Settings as SettingsIcon,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

type NavItem = {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  match?: (path: string) => boolean;
};

const navItems: NavItem[] = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard, match: (p) => p === "/" },
  { href: "/leads", label: "Leads", icon: Users, match: (p) => p.startsWith("/leads") },
  {
    href: "/whatsapp",
    label: "WhatsApp",
    icon: Phone,
    match: (p) => p.startsWith("/whatsapp"),
  },
  {
    href: "/destinations",
    label: "Destinations",
    icon: Plug,
    match: (p) => p.startsWith("/destinations"),
  },
  {
    href: "/settings",
    label: "Settings",
    icon: SettingsIcon,
    match: (p) => p.startsWith("/settings"),
  },
];

export function Sidebar() {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = React.useState(false);

  return (
    <aside
      className={cn(
        "sticky top-0 hidden h-screen flex-none border-r bg-card transition-all duration-200 md:flex md:flex-col",
        collapsed ? "w-16" : "w-60"
      )}
    >
      <div className="flex h-14 items-center border-b px-4">
        <Link
          href="/"
          className={cn(
            "flex items-center gap-2 font-semibold tracking-tight",
            collapsed && "justify-center"
          )}
        >
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
            N
          </div>
          {!collapsed && <span>NiosEngine</span>}
        </Link>
      </div>

      <nav className="flex-1 overflow-y-auto p-2">
        <ul className="space-y-1">
          {navItems.map((item) => {
            const active = item.match ? item.match(pathname) : pathname.startsWith(item.href);
            const Icon = item.icon;
            const link = (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  active
                    ? "bg-primary/10 text-primary"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                  collapsed && "justify-center px-0"
                )}
                aria-current={active ? "page" : undefined}
              >
                <Icon className="h-4 w-4 flex-none" />
                {!collapsed && <span className="truncate">{item.label}</span>}
              </Link>
            );
            return (
              <li key={item.href}>
                {collapsed ? (
                  <Tooltip>
                    <TooltipTrigger asChild>{link}</TooltipTrigger>
                    <TooltipContent side="right">{item.label}</TooltipContent>
                  </Tooltip>
                ) : (
                  link
                )}
              </li>
            );
          })}
        </ul>
      </nav>

      <div className="border-t p-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setCollapsed((c) => !c)}
          className={cn("w-full", collapsed ? "justify-center px-0" : "justify-start")}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {collapsed ? (
            <ChevronRight className="h-4 w-4" />
          ) : (
            <>
              <ChevronLeft className="mr-2 h-4 w-4" />
              Collapse
            </>
          )}
        </Button>
      </div>
    </aside>
  );
}
