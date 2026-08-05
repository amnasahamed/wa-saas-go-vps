"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  Users,
  Sparkles,
  MessageSquare,
  GraduationCap,
  ArrowRight,
  Activity,
} from "lucide-react";
import { api, type Lead, type Workspace, type Message } from "@/lib/api";
import { qk } from "@/lib/queries";
import { formatRelative } from "@/lib/format";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

type Overview = {
  totals: {
    all: number;
    new_today: number;
    awaiting_reply: number;
    enrolled: number;
  };
  activity: Array<{
    kind: "stage" | "extraction" | "message";
    leadId: string;
    leadName: string;
    body: string;
    timestamp: string;
  }>;
};

export default function OverviewPage() {
  const { data, isLoading } = useQuery({
    queryKey: qk.overview(),
    queryFn: () => api.get<Overview>("/me/overview"),
    refetchInterval: 30_000,
  });
  const { data: workspace } = useQuery({
    queryKey: qk.workspace(),
    queryFn: () => api.get<Workspace>("/me/workspace"),
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Welcome back{workspace?.name ? `, ${workspace.name}` : ""}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Here's what's happening with your leads today.
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <KpiCard
          title="Total leads"
          value={data?.totals.all}
          icon={Users}
          loading={isLoading}
          href="/leads"
        />
        <KpiCard
          title="New today"
          value={data?.totals.new_today}
          icon={Sparkles}
          loading={isLoading}
        />
        <KpiCard
          title="Awaiting reply"
          value={data?.totals.awaiting_reply}
          icon={MessageSquare}
          loading={isLoading}
          href="/leads?status=Follow-up"
        />
        <KpiCard
          title="Enrolled"
          value={data?.totals.enrolled}
          icon={GraduationCap}
          loading={isLoading}
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">Recent activity</CardTitle>
            </div>
            <CardDescription>Stage transitions and extractions across all leads.</CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-3">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : !data?.activity.length ? (
              <p className="text-sm text-muted-foreground">No activity yet.</p>
            ) : (
              <ul className="space-y-2">
                {data.activity.map((a, i) => (
                  <li key={i} className="flex items-center justify-between rounded-md border p-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{a.leadName}</p>
                      <p className="truncate text-xs text-muted-foreground">{a.body}</p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge variant="muted">{a.kind}</Badge>
                      <span className="text-xs text-muted-foreground">
                        {formatRelative(a.timestamp)}
                      </span>
                      <Button variant="ghost" size="icon" asChild>
                        <Link href={`/leads/${a.leadId}`} aria-label="Open lead">
                          <ArrowRight className="h-4 w-4" />
                        </Link>
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Setup checklist</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <ChecklistItem
              done={workspace?.whatsapp?.status === "ready" || workspace?.whatsapp?.status === "authenticated"}
              label="Link WhatsApp"
              href="/whatsapp"
            />
            <ChecklistItem
              done={!!workspace?.openaiKeyConfigured}
              label="Add OpenAI key (optional)"
              href="/settings/workspace"
            />
            <ChecklistItem
              done={!!workspace?.promptOverride}
              label="Customize nurture prompt"
              href="/settings/nurture"
            />
            <ChecklistItem
              done={workspace?.plan !== "free"}
              label="Upgrade plan"
              href="/settings/billing"
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function KpiCard({
  title,
  value,
  icon: Icon,
  loading,
  href,
}: {
  title: string;
  value: number | undefined;
  icon: React.ComponentType<{ className?: string }>;
  loading: boolean;
  href?: string;
}) {
  const body = (
    <Card className="h-full">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-8 w-16" />
        ) : (
          <p className="text-2xl font-bold">{value ?? 0}</p>
        )}
      </CardContent>
    </Card>
  );
  return href ? <Link href={href}>{body}</Link> : body;
}

function ChecklistItem({
  done,
  label,
  href,
}: {
  done: boolean;
  label: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="flex items-center justify-between rounded-md border p-3 transition-colors hover:bg-muted"
    >
      <span className={done ? "text-muted-foreground line-through" : ""}>{label}</span>
      <Badge variant={done ? "success" : "outline"}>{done ? "Done" : "Open"}</Badge>
    </Link>
  );
}
