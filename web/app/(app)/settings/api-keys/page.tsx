"use client";

import * as React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Copy, KeyRound, Loader2, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { qk } from "@/lib/queries";
import { formatDateTime } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type ApiKey = {
  id: string;
  name: string;
  prefix: string;
  scope: string;
  lastUsedAt: string | null;
  createdAt: string;
  revokedAt: string | null;
};

const schema = z.object({
  name: z.string().min(2, "Min 2 characters"),
});
type FormValues = z.infer<typeof schema>;

export default function ApiKeysPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: qk.apiKeys(),
    queryFn: () => api.get<{ items: ApiKey[] }>("/me/api-keys"),
  });

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: "" },
  });

  const mintMutation = useMutation({
    mutationFn: (values: FormValues) =>
      api.post<{ id: string; secret: string; prefix: string }>("/me/api-keys", values),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: qk.apiKeys() });
      toast.success("API key created", {
        description: `Copy the secret now — it won't be shown again. Prefix: ${res.prefix}`,
      });
      form.reset();
      navigator.clipboard?.writeText(res.secret).catch(() => {});
    },
    onError: () => toast.error("Failed to create API key"),
  });

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.delete<{ ok: true }>(`/me/api-keys/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.apiKeys() });
      toast.success("API key revoked");
    },
    onError: () => toast.error("Failed to revoke API key"),
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Mint a new key</CardTitle>
          <CardDescription>
            Bridge and webhook integrations authenticate with API keys. Treat them like passwords.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={form.handleSubmit((v) => mintMutation.mutate(v))}
            className="flex gap-2"
          >
            <div className="flex-1 space-y-1">
              <Label htmlFor="name" className="sr-only">
                Name
              </Label>
              <Input id="name" placeholder="e.g. Bridge worker #1" {...form.register("name")} />
              {form.formState.errors.name && (
                <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
              )}
            </div>
            <Button type="submit" disabled={mintMutation.isPending}>
              {mintMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="mr-2 h-4 w-4" />}
              Mint key
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Active keys</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : !data?.items.length ? (
            <p className="text-sm text-muted-foreground">No API keys yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Prefix</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead>Last used</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-12" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.items.map((k) => (
                  <TableRow key={k.id}>
                    <TableCell className="font-medium">{k.name}</TableCell>
                    <TableCell>
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{k.prefix}</code>
                    </TableCell>
                    <TableCell>
                      <Badge variant="muted">{k.scope}</Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatDateTime(k.lastUsedAt)}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatDateTime(k.createdAt)}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => revokeMutation.mutate(k.id)}
                        disabled={!!k.revokedAt || revokeMutation.isPending}
                        aria-label="Revoke key"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
