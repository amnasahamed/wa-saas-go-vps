"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  Pause,
  Play,
  MoreHorizontal,
  Send,
  Loader2,
  Pencil,
  X,
  Check,
} from "lucide-react";
import { api, type LeadDetail, type Lead, type Message, type Extraction } from "@/lib/api";
import { qk } from "@/lib/queries";
import { useAuth } from "@/lib/auth";
import { formatDateTime, formatPhone, formatRelative } from "@/lib/format";
import {
  LEAD_STATUSES,
  LEAD_STAGES,
  STAGE_LABELS,
  STAGE_DESCRIPTIONS,
  type LeadStatus,
  type LeadStage,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const statusVariant = (status: string) => {
  switch (status) {
    case "Enrolled":
      return "success";
    case "Lost":
      return "destructive";
    case "Interested":
      return "default";
    case "Follow-up":
      return "warning";
    case "Contacted":
      return "secondary";
    default:
      return "muted";
  }
};

export default function LeadDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params.id;
  const router = useRouter();
  const { workspace } = useAuth();
  const qc = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: qk.lead(id),
    queryFn: () => api.get<LeadDetail>(`/leads/${id}`),
    refetchInterval: 10_000,
  });

  const patchLead = useMutation({
    mutationFn: (body: Partial<Lead>) => api.patch<Lead>(`/leads/${id}`, body),
    onSuccess: (updated) => {
      qc.setQueryData<LeadDetail | undefined>(qk.lead(id), (prev) =>
        prev ? { ...prev, lead: updated } : prev
      );
      qc.invalidateQueries({ queryKey: qk.leads() });
      toast.success("Lead updated");
    },
    onError: () => toast.error("Failed to update lead"),
  });

  const sendMessage = useMutation({
    mutationFn: (body: string) => api.post<Message>(`/leads/${id}/messages`, { body }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.lead(id) });
      toast.success("Message sent");
    },
    onError: () => toast.error("Failed to send message"),
  });

  if (isLoading) {
    return <LeadDetailSkeleton />;
  }

  if (error || !data) {
    return (
      <Alert variant="destructive">
        <AlertDescription>Failed to load lead. {String(error)}</AlertDescription>
      </Alert>
    );
  }

  const { lead, history, extractions } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.push("/leads")}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-2xl font-semibold">{lead.name || "Unnamed lead"}</h1>
          <p className="text-sm text-muted-foreground">{formatPhone(lead.phone)}</p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-[320px_1fr]">
        {/* Left header card */}
        <Card className="h-fit">
          <CardHeader>
            <CardTitle className="text-base">Lead details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>Status</Label>
              <Select
                value={lead.status}
                onValueChange={(v) => patchLead.mutate({ status: v as LeadStatus })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LEAD_STATUSES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Stage</Label>
              <Select
                value={lead.stage}
                onValueChange={(v) => patchLead.mutate({ stage: v as LeadStage })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LEAD_STAGES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {STAGE_LABELS[s]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{STAGE_DESCRIPTIONS[lead.stage as LeadStage] ?? ""}</p>
            </div>

            <Separator />

            <div className="flex items-center justify-between">
              <div>
                <Label className="text-sm">Nurture paused</Label>
                <p className="text-xs text-muted-foreground">
                  {lead.nurturePausedUntil
                    ? `Until ${formatDateTime(lead.nurturePausedUntil)}`
                    : "AI replies are running"}
                </p>
              </div>
              <Switch
                checked={lead.nurturePaused}
                onCheckedChange={(checked) =>
                  patchLead.mutate({
                    nurturePausedUntil: checked
                      ? new Date(Date.now() + 24 * 3600 * 1000).toISOString()
                      : null,
                  })
                }
              />
            </div>

            <Separator />

            <div className="space-y-1 text-xs text-muted-foreground">
              <Row label="Last inbound" value={formatRelative(lead.lastInboundAt)} />
              <Row label="Last contacted" value={formatRelative(lead.lastContactedAt)} />
              <Row label="Followups" value={String(lead.followupCount)} />
              <Row label="Created" value={formatDateTime(lead.createdAt)} />
            </div>
          </CardContent>
        </Card>

        {/* Right tabs */}
        <Tabs defaultValue="conversation" className="w-full">
          <TabsList>
            <TabsTrigger value="conversation">
              Conversation
              {history.length > 0 && (
                <Badge variant="muted" className="ml-2">
                  {history.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="fields">Fields</TabsTrigger>
            <TabsTrigger value="activity">
              Activity
              {extractions.length > 0 && (
                <Badge variant="muted" className="ml-2">
                  {extractions.length}
                </Badge>
              )}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="conversation">
            <ConversationView
              lead={lead}
              messages={history}
              onSend={(body) => sendMessage.mutate(body)}
              sending={sendMessage.isPending}
            />
          </TabsContent>

          <TabsContent value="overview">
            <OverviewTab lead={lead} workspace={workspace} />
          </TabsContent>

          <TabsContent value="fields">
            <FieldsTab
              lead={lead}
              onSave={(fields) =>
                patchLead.mutate({ fields: fields as Lead["fields"] })
              }
              saving={patchLead.isPending}
            />
          </TabsContent>

          <TabsContent value="activity">
            <ActivityTab extractions={extractions} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <span>{label}</span>
      <span className="font-medium text-foreground">{value}</span>
    </div>
  );
}

function LeadDetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-48" />
      <div className="grid gap-6 lg:grid-cols-[320px_1fr]">
        <Skeleton className="h-96" />
        <Skeleton className="h-96" />
      </div>
    </div>
  );
}

function ConversationView({
  lead,
  messages,
  onSend,
  sending,
}: {
  lead: Lead;
  messages: Message[];
  onSend: (body: string) => void;
  sending: boolean;
}) {
  const [body, setBody] = React.useState("");
  const scrollRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages.length]);

  const send = () => {
    const trimmed = body.trim();
    if (!trimmed) return;
    onSend(trimmed);
    setBody("");
  };

  return (
    <Card className="flex h-[600px] flex-col">
      <CardHeader>
        <CardTitle className="text-base">Conversation</CardTitle>
        <CardDescription>
          {lead.phone} · {messages.length} message{messages.length === 1 ? "" : "s"}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col p-0">
        <ScrollArea className="flex-1 px-6">
          <div ref={scrollRef} className="space-y-3 py-4">
            {messages.length === 0 ? (
              <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
                No messages yet.
              </div>
            ) : (
              messages.map((m) => <MessageBubble key={m.id} message={m} />)
            )}
          </div>
        </ScrollArea>
        <div className="border-t bg-muted/30 p-4">
          <div className="flex gap-2">
            <Textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder="Reply as counselor…"
              rows={2}
              className="resize-none"
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  send();
                }
              }}
            />
            <Button onClick={send} disabled={!body.trim() || sending} size="icon" className="h-auto">
              {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
            </Button>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">⌘/Ctrl + Enter to send</p>
        </div>
      </CardContent>
    </Card>
  );
}

function MessageBubble({ message }: { message: Message }) {
  const isInbound = message.direction === "inbound";
  const senderColor =
    message.sender === "ai"
      ? "bg-blue-500/10 text-foreground"
      : message.sender === "counselor"
      ? "bg-emerald-500/10 text-foreground"
      : isInbound
      ? "bg-muted text-foreground"
      : "bg-muted text-foreground";

  return (
    <div className={`flex ${isInbound ? "justify-start" : "justify-end"}`}>
      <div className={`max-w-[80%] rounded-lg px-3 py-2 ${senderColor}`}>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium capitalize">{message.sender}</span>
          <span>{formatTime(message.timestamp)}</span>
        </div>
        <p className="mt-1 whitespace-pre-wrap text-sm">{message.body}</p>
      </div>
    </div>
  );
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function OverviewTab({ lead, workspace }: { lead: Lead; workspace: { countryCode: string } | null }) {
  const keys = Object.keys(lead.fields ?? {});
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Identification</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <Row label="Phone" value={formatPhone(lead.phone)} />
          <Row label="Name" value={lead.name || "—"} />
          <Row label="Country code" value={workspace?.countryCode ?? "—"} />
          <Row label="Lead ID" value={lead.id} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Pipeline</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <Row label="Status" value={lead.status} />
          <Row label="Stage" value={STAGE_LABELS[lead.stage as LeadStage] ?? lead.stage} />
          <Row label="Fields captured" value={String(keys.length)} />
          <Row label="Followups" value={String(lead.followupCount)} />
        </CardContent>
      </Card>
    </div>
  );
}

function FieldsTab({
  lead,
  onSave,
  saving,
}: {
  lead: Lead;
  onSave: (fields: Record<string, unknown>) => void;
  saving: boolean;
}) {
  const [editing, setEditing] = React.useState<Record<string, string>>({});
  const initial = React.useMemo(() => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(lead.fields ?? {})) {
      out[k] = v == null ? "" : String(v);
    }
    return out;
  }, [lead.fields]);
  const [values, setValues] = React.useState<Record<string, string>>(initial);

  React.useEffect(() => {
    setValues(initial);
  }, [initial]);

  const dirty = JSON.stringify(values) !== JSON.stringify(initial);

  const keys = Object.keys(values);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Extracted fields</CardTitle>
        <CardDescription>
          GPT-4o fills these from the conversation. Edit manually if needed.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {keys.length === 0 ? (
          <p className="text-sm text-muted-foreground">No fields captured yet.</p>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {keys.map((k) => (
              <div key={k} className="space-y-1">
                <Label className="text-xs">{k}</Label>
                <Input
                  value={values[k] ?? ""}
                  onChange={(e) =>
                    setValues((v) => ({ ...v, [k]: e.target.value }))
                  }
                  onFocus={() => setEditing((s) => ({ ...s, [k]: "true" }))}
                />
              </div>
            ))}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => setValues(initial)} disabled={!dirty}>
            Reset
          </Button>
          <Button
            onClick={() => {
              onSave(values);
              setEditing({});
            }}
            disabled={!dirty || saving}
          >
            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            Save fields
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function ActivityTab({ extractions }: { extractions: Extraction[] }) {
  if (extractions.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-sm text-muted-foreground">
          No extraction activity yet.
        </CardContent>
      </Card>
    );
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Extraction history</CardTitle>
        <CardDescription>
          Every GPT-4o extraction that produced fields for this lead.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ol className="space-y-4">
          {extractions.map((ex) => (
            <li key={ex.id} className="rounded-lg border bg-card p-4">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  {formatDateTime(ex.createdAt)}
                </span>
                <Badge variant="muted">{Object.keys(ex.fields).length} fields</Badge>
              </div>
              <pre className="mt-2 max-h-40 overflow-auto rounded bg-muted/50 p-2 text-xs">
                {JSON.stringify(ex.fields, null, 2)}
              </pre>
            </li>
          ))}
        </ol>
      </CardContent>
    </Card>
  );
}
