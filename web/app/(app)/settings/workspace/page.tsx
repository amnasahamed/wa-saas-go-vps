"use client";

import * as React from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Loader2, Eye, EyeOff } from "lucide-react";
import { api, type Workspace } from "@/lib/api";
import { qk } from "@/lib/queries";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";

const schema = z.object({
  name: z.string().min(2, "Min 2 characters"),
  countryCode: z.string().regex(/^\d{1,4}$/, "Digits only"),
  timezone: z.string().min(1),
  locale: z.string().min(2),
});

type FormValues = z.infer<typeof schema>;

export default function WorkspaceSettingsPage() {
  const { data, isLoading } = useQuery({
    queryKey: qk.workspace(),
    queryFn: () => api.get<Workspace>("/me/workspace"),
  });

  if (isLoading || !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-32 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <WorkspaceForm workspace={data} />
      <OpenAIKeyCard configured={data.openaiKeyConfigured} />
    </div>
  );
}

function WorkspaceForm({ workspace }: { workspace: Workspace }) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: workspace.name,
      countryCode: workspace.countryCode,
      timezone: workspace.timezone,
      locale: workspace.locale,
    },
  });

  React.useEffect(() => {
    form.reset({
      name: workspace.name,
      countryCode: workspace.countryCode,
      timezone: workspace.timezone,
      locale: workspace.locale,
    });
  }, [workspace, form]);

  const mutation = useMutation({
    mutationFn: (values: FormValues) => api.patch<Workspace>("/me/workspace", values),
    onSuccess: () => toast.success("Workspace updated"),
    onError: () => toast.error("Failed to save"),
  });

  const onSubmit = form.handleSubmit((values) => mutation.mutate(values));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Workspace</CardTitle>
        <CardDescription>Display name and localization defaults.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2 md:col-span-2">
              <Label htmlFor="name">Workspace name</Label>
              <Input id="name" {...form.register("name")} />
              {form.formState.errors.name && (
                <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="countryCode">Default country code</Label>
              <Input id="countryCode" placeholder="91" {...form.register("countryCode")} />
              {form.formState.errors.countryCode && (
                <p className="text-xs text-destructive">
                  {form.formState.errors.countryCode.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="timezone">Timezone</Label>
              <Input id="timezone" placeholder="Asia/Kolkata" {...form.register("timezone")} />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label htmlFor="locale">Locale</Label>
              <Input id="locale" placeholder="en-IN" {...form.register("locale")} />
            </div>
          </div>
          <div className="flex items-center justify-between border-t pt-4">
            <div className="text-xs text-muted-foreground">
              Plan: <Badge variant="muted" className="capitalize">{workspace.plan}</Badge> ·{" "}
              Slug: <code>{workspace.slug}</code>
            </div>
            <Button type="submit" disabled={mutation.isPending || !form.formState.isDirty}>
              {mutation.isPending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Save changes
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function OpenAIKeyCard({ configured }: { configured: boolean }) {
  const [reveal, setReveal] = React.useState(false);
  const [key, setKey] = React.useState("");
  const mutation = useMutation({
    mutationFn: (k: string) => api.post<{ ok: true }>("/me/openai-key", { key: k }),
    onSuccess: () => {
      toast.success("OpenAI key saved");
      setKey("");
    },
    onError: () => toast.error("Failed to save OpenAI key"),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">OpenAI</CardTitle>
        <CardDescription>
          Bring your own OpenAI key, or use the platform default.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2 text-sm">
          Status:{" "}
          {configured ? (
            <Badge variant="success">Custom key configured</Badge>
          ) : (
            <Badge variant="muted">Using platform default</Badge>
          )}
        </div>
        <Alert variant="info">
          <AlertDescription>
            Stored encrypted at rest with your tenant's KMS data key. Never logged in plaintext.
          </AlertDescription>
        </Alert>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (key.trim()) mutation.mutate(key.trim());
          }}
          className="space-y-2"
        >
          <Label htmlFor="openai-key">API key</Label>
          <div className="flex gap-2">
            <div className="relative flex-1">
              <Input
                id="openai-key"
                type={reveal ? "text" : "password"}
                placeholder="sk-..."
                value={key}
                onChange={(e) => setKey(e.target.value)}
                autoComplete="off"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setReveal((r) => !r)}
                className="absolute right-1 top-1/2 h-7 w-7 -translate-y-1/2"
                aria-label={reveal ? "Hide key" : "Reveal key"}
              >
                {reveal ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </Button>
            </div>
            <Button type="submit" disabled={!key.trim() || mutation.isPending}>
              {mutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Save"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
