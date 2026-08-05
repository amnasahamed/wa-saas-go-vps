"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  Plus,
  Plug,
  MoreHorizontal,
  Trash2,
  Settings as SettingsIcon,
  Loader2,
  Sheet,
  Database,
  FileText,
} from "lucide-react";
import { api, type Destination } from "@/lib/api";
import { qk } from "@/lib/queries";
import { formatRelative } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

const TYPE_META = {
  google_sheets: { label: "Google Sheets", icon: Sheet, description: "Upsert to a spreadsheet. One row per phone." },
  airtable: { label: "Airtable", icon: Database, description: "Sync to a base with custom fields." },
  notion: { label: "Notion", icon: FileText, description: "Sync to a Notion database with custom properties." },
} as const;

type DestType = keyof typeof TYPE_META;

const configSchema = z.object({
  type: z.enum(["google_sheets", "airtable", "notion"]),
  name: z.string().min(2),
  accessToken: z.string().min(8, "Required"),
  spreadsheetId: z.string().optional(),
  sheetName: z.string().optional(),
  baseId: z.string().optional(),
  tableName: z.string().optional(),
  databaseId: z.string().optional(),
});

type ConfigFormValues = z.infer<typeof configSchema>;

export default function DestinationsPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: qk.destinations(),
    queryFn: () => api.get<{ items: Destination[] }>("/destinations"),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.patch<Destination>(`/destinations/${id}`, { enabled }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.destinations() });
    },
    onError: () => toast.error("Failed to update destination"),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete<{ ok: true }>(`/destinations/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.destinations() });
      toast.success("Destination disconnected");
    },
    onError: () => toast.error("Failed to disconnect destination"),
  });

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Destinations</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Where your leads land. Connect one or more — every extraction fans out to all enabled destinations.
          </p>
        </div>
        <AddDestinationDialog />
      </div>

      {isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      ) : !data?.items.length ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
            <Plug className="h-10 w-10 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              No destinations yet. Add Google Sheets, Airtable, or Notion to start syncing.
            </p>
            <AddDestinationDialog />
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {data.items.map((d) => {
            const meta = TYPE_META[d.type];
            const Icon = meta.icon;
            return (
              <Card key={d.id}>
                <CardHeader>
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Icon className="h-4 w-4 text-muted-foreground" />
                      <CardTitle className="text-base">{d.name}</CardTitle>
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" aria-label="Destination actions">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem asChild>
                          <Link href={`/destinations/${d.id}`}>
                            <SettingsIcon className="mr-2 h-4 w-4" />
                            Edit mapping
                          </Link>
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onSelect={() => deleteMutation.mutate(d.id)}
                          className="text-destructive"
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          Disconnect
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  <CardDescription>{meta.label}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Sync to this destination</span>
                    <Switch
                      checked={d.enabled}
                      onCheckedChange={(checked) =>
                        toggleMutation.mutate({ id: d.id, enabled: checked })
                      }
                    />
                  </div>
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span>
                      Last sync: {d.lastSyncAt ? formatRelative(d.lastSyncAt) : "never"}
                    </span>
                    <Badge variant={d.enabled ? "success" : "muted"}>
                      {d.enabled ? "Enabled" : "Disabled"}
                    </Badge>
                  </div>
                  {d.lastError && (
                    <p className="rounded bg-destructive/10 p-2 text-xs text-destructive">
                      {d.lastError}
                    </p>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

function AddDestinationDialog() {
  const qc = useQueryClient();
  const [open, setOpen] = React.useState(false);
  const [step, setStep] = React.useState<1 | 2 | 3>(1);
  const form = useForm<ConfigFormValues>({
    resolver: zodResolver(configSchema),
    defaultValues: {
      type: "google_sheets",
      name: "",
      accessToken: "",
    },
  });

  const watchType = form.watch("type");

  const mutation = useMutation({
    mutationFn: (values: ConfigFormValues) => {
      const config: Record<string, unknown> = { access_token: values.accessToken };
      if (values.type === "google_sheets") {
        config.spreadsheet_id = values.spreadsheetId;
        config.sheet_name = values.sheetName;
      } else if (values.type === "airtable") {
        config.base_id = values.baseId;
        config.table_name = values.tableName;
      } else if (values.type === "notion") {
        config.database_id = values.databaseId;
      }
      return api.post<Destination>("/destinations", {
        type: values.type,
        name: values.name,
        config,
        field_mapping: {
          name: "Name",
          phone: "Phone",
          status: "Status",
          stage: "Stage",
        },
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.destinations() });
      toast.success("Destination connected");
      setOpen(false);
      setStep(1);
      form.reset();
    },
    onError: () => toast.error("Failed to connect destination"),
  });

  const onSubmit = form.handleSubmit((v) => mutation.mutate(v));

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Add destination
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Add destination</DialogTitle>
          <DialogDescription>
            Step {step} of 3 —{" "}
            {step === 1 ? "Pick a type" : step === 2 ? "Configure connection" : "Field mapping"}
          </DialogDescription>
        </DialogHeader>

        {step === 1 && (
          <div className="space-y-3">
            {(Object.keys(TYPE_META) as DestType[]).map((t) => {
              const meta = TYPE_META[t];
              const Icon = meta.icon;
              const selected = watchType === t;
              return (
                <button
                  key={t}
                  type="button"
                  onClick={() => form.setValue("type", t)}
                  className={cn(
                    "flex w-full items-start gap-3 rounded-lg border p-4 text-left transition-colors",
                    selected ? "border-primary bg-primary/5" : "hover:bg-muted"
                  )}
                >
                  <Icon className="mt-1 h-5 w-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium">{meta.label}</p>
                    <p className="text-sm text-muted-foreground">{meta.description}</p>
                  </div>
                </button>
              );
            })}
            <DialogFooter>
              <Button onClick={() => setStep(2)}>Continue</Button>
            </DialogFooter>
          </div>
        )}

        {step === 2 && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              setStep(3);
            }}
            className="space-y-3"
          >
            <div className="space-y-2">
              <Label htmlFor="name">Friendly name</Label>
              <Input id="name" placeholder="e.g. Coaching CRM" {...form.register("name")} />
            </div>
            {watchType === "google_sheets" && (
              <>
                <div className="space-y-2">
                  <Label>Spreadsheet ID</Label>
                  <Input placeholder="1abc…" {...form.register("spreadsheetId")} />
                </div>
                <div className="space-y-2">
                  <Label>Sheet name</Label>
                  <Input placeholder="Leads" {...form.register("sheetName")} />
                </div>
              </>
            )}
            {watchType === "airtable" && (
              <>
                <div className="space-y-2">
                  <Label>Base ID</Label>
                  <Input placeholder="app…" {...form.register("baseId")} />
                </div>
                <div className="space-y-2">
                  <Label>Table name</Label>
                  <Input placeholder="Leads" {...form.register("tableName")} />
                </div>
              </>
            )}
            {watchType === "notion" && (
              <div className="space-y-2">
                <Label>Database ID</Label>
                <Input placeholder="…" {...form.register("databaseId")} />
              </div>
            )}
            <div className="space-y-2">
              <Label>Access token</Label>
              <Input
                type="password"
                autoComplete="off"
                placeholder="Paste token"
                {...form.register("accessToken")}
              />
              <p className="text-xs text-muted-foreground">
                Stored encrypted at rest. Treat like a password.
              </p>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setStep(1)} type="button">
                Back
              </Button>
              <Button type="submit">Continue</Button>
            </DialogFooter>
          </form>
        )}

        {step === 3 && (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              We'll map these fields by default. You can customise the mapping after connecting.
            </p>
            <ul className="space-y-1 rounded-md border p-3 text-sm">
              <li>name → Name</li>
              <li>phone → Phone</li>
              <li>status → Status</li>
              <li>stage → Stage</li>
            </ul>
            <DialogFooter>
              <Button variant="outline" onClick={() => setStep(2)} type="button">
                Back
              </Button>
              <Button onClick={onSubmit} disabled={mutation.isPending}>
                {mutation.isPending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                Connect
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
