"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Circle, ArrowRight } from "lucide-react";
import { api } from "@/lib/api";
import { qk } from "@/lib/queries";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

type OnboardingStatus = {
  email_verified: boolean;
  whatsapp_linked: boolean;
  has_destination: boolean;
};

export default function OnboardingPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["onboarding"],
    queryFn: () => api.get<OnboardingStatus>("/me/onboarding"),
  });

  return (
    <div className="mx-auto max-w-2xl space-y-6 py-10">
      <div>
        <h1 className="text-3xl font-bold">Welcome to NiosEngine</h1>
        <p className="mt-2 text-muted-foreground">
          Let's get your WhatsApp number linked and your CRM connected.
        </p>
      </div>

      <ol className="space-y-4">
        <Step
          n={1}
          title="Choose a plan"
          body="Pick the plan that fits your volume. You can change it anytime."
          cta={{ label: "Choose plan", href: "/settings/billing" }}
          done={false}
          loading={isLoading}
        />
        <Step
          n={2}
          title="Link WhatsApp"
          body="Open WhatsApp on your phone, go to Linked devices, and scan the QR code."
          cta={{ label: "Link WhatsApp", href: "/whatsapp" }}
          done={!!data?.whatsapp_linked}
          loading={isLoading}
        />
        <Step
          n={3}
          title="Connect a destination"
          body="Pick Google Sheets, Airtable, or Notion. We'll show you the exact columns to set up."
          cta={{ label: "Connect CRM", href: "/destinations" }}
          done={!!data?.has_destination}
          loading={isLoading}
        />
        <Step
          n={4}
          title="Customize the nurture prompt"
          body="Tune how the AI talks to your leads."
          cta={{ label: "Open nurture editor", href: "/settings/nurture" }}
          done={false}
          loading={isLoading}
        />
      </ol>
    </div>
  );
}

function Step({
  n,
  title,
  body,
  cta,
  done,
  loading,
}: {
  n: number;
  title: string;
  body: string;
  cta: { label: string; href: string };
  done: boolean;
  loading: boolean;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start gap-4">
          <div
            className={`flex h-10 w-10 flex-none items-center justify-center rounded-full text-sm font-medium ${
              done
                ? "bg-emerald-500 text-white"
                : "border-2 border-dashed border-muted-foreground/30 text-muted-foreground"
            }`}
          >
            {done ? <CheckCircle2 className="h-5 w-5" /> : n}
          </div>
          <div className="flex-1">
            <CardTitle className="flex items-center gap-2 text-base">
              {title}
              {done && (
                <span className="text-xs font-normal text-emerald-600">Done</span>
              )}
            </CardTitle>
            <CardDescription>{body}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-9 w-32" />
        ) : (
          <Button asChild variant={done ? "outline" : "default"}>
            <Link href={cta.href}>
              {cta.label}
              <ArrowRight className="ml-2 h-4 w-4" />
            </Link>
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
