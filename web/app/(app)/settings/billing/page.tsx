"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { ExternalLink, Loader2 } from "lucide-react";
import { api, type Workspace } from "@/lib/api";
import { qk } from "@/lib/queries";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";

type Subscription = {
  plan: "free" | "pro" | "agency";
  status: string;
  currentPeriodEnd: string | null;
  cancelAtPeriodEnd: boolean;
};

const PLAN_FEATURES = {
  free: { sessions: 1, destinations: 1, monthlyMessages: 200, price: "$0/mo" },
  pro: { sessions: 1, destinations: 3, monthlyMessages: 5000, price: "$49/mo" },
  agency: { sessions: 5, destinations: 10, monthlyMessages: 50000, price: "$199/mo" },
} as const;

export default function BillingPage() {
  const { data: workspace, isLoading: wLoading } = useQuery({
    queryKey: qk.workspace(),
    queryFn: () => api.get<Workspace>("/me/workspace"),
  });
  const { data: sub, isLoading: sLoading } = useQuery({
    queryKey: ["billing"],
    queryFn: () => api.get<Subscription | null>("/me/billing"),
  });

  if (wLoading || sLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-32 w-full" />
      </div>
    );
  }

  const plan = workspace?.plan ?? "free";
  const features = PLAN_FEATURES[plan];

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Current plan</CardTitle>
          <CardDescription>Manage your subscription and invoices.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-3">
            <Badge variant="default" className="capitalize">
              {plan}
            </Badge>
            <span className="text-2xl font-semibold">{features.price}</span>
          </div>
          <ul className="grid gap-2 text-sm md:grid-cols-3">
            <li>
              <span className="text-muted-foreground">WhatsApp sessions:</span>{" "}
              <span className="font-medium">{features.sessions}</span>
            </li>
            <li>
              <span className="text-muted-foreground">Destinations:</span>{" "}
              <span className="font-medium">{features.destinations}</span>
            </li>
            <li>
              <span className="text-muted-foreground">Monthly messages:</span>{" "}
              <span className="font-medium">{features.monthlyMessages.toLocaleString()}</span>
            </li>
          </ul>
          {sub && (
            <p className="text-xs text-muted-foreground">
              Status: {sub.status}
              {sub.currentPeriodEnd && ` · Renews ${new Date(sub.currentPeriodEnd).toLocaleDateString()}`}
              {sub.cancelAtPeriodEnd && " · Cancels at period end"}
            </p>
          )}
          <div className="flex gap-2">
            <Button asChild>
              <a href="/api/v1/me/billing/portal" target="_blank" rel="noopener">
                <ExternalLink className="mr-2 h-4 w-4" />
                Open billing portal
              </a>
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Plan comparison</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-3">
            {(Object.keys(PLAN_FEATURES) as Array<keyof typeof PLAN_FEATURES>).map((p) => (
              <div
                key={p}
                className={`rounded-lg border p-4 ${p === plan ? "border-primary bg-primary/5" : ""}`}
              >
                <div className="flex items-center justify-between">
                  <h3 className="font-semibold capitalize">{p}</h3>
                  {p === plan && <Badge>Current</Badge>}
                </div>
                <p className="mt-1 text-2xl font-semibold">{PLAN_FEATURES[p].price}</p>
                <ul className="mt-2 space-y-1 text-xs text-muted-foreground">
                  <li>{PLAN_FEATURES[p].sessions} WhatsApp session(s)</li>
                  <li>{PLAN_FEATURES[p].destinations} destination(s)</li>
                  <li>{PLAN_FEATURES[p].monthlyMessages.toLocaleString()} messages / month</li>
                </ul>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
