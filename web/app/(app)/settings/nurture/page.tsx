"use client";

import * as React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  History,
  RotateCcw,
  Save,
  Trash2,
  Play,
  Loader2,
  Code,
  CheckCircle2,
  Circle,
  ChevronRight,
} from "lucide-react";
import { api, type PromptPreview, type PromptVersion, type Workspace, type StageUsage } from "@/lib/api";
import {
  LEAD_STAGES,
  STAGE_LABELS,
  STAGE_DESCRIPTIONS,
  type LeadStage,
} from "@/lib/api";
import { qk } from "@/lib/queries";
import { approxTokens, truncate } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

type PreviewMessage = {
  sender: "customer" | "ai";
  body: string;
};

const DEFAULT_HISTORY: PreviewMessage[] = [
  { sender: "customer", body: "Hi, I failed my 12th exam last year. Can I still get NIOS certification?" },
  { sender: "ai", body: "Hi! Yes, you can. May I know your name?" },
  { sender: "customer", body: "My name is Anjali." },
  { sender: "ai", body: "Thanks Anjali. Which class did you fail — 10th or 12th?" },
  { sender: "customer", body: "12th, plus two commerce." },
  { sender: "ai", body: "Got it. Did you fail the exam, or are you still appearing?" },
];

export default function NurturePage() {
  const qc = useQueryClient();
  const { data: workspace, isLoading } = useQuery({
    queryKey: qk.workspace(),
    queryFn: () => api.get<Workspace>("/me/workspace"),
  });
  const { data: defaultPrompt } = useQuery({
    queryKey: qk.promptDefault(),
    queryFn: () => api.get<{ template: string }>("/prompts/default"),
  });
  const { data: stageUsage } = useQuery({
    queryKey: qk.stageUsage(),
    queryFn: () => api.get<StageUsage[]>("/me/stage-usage"),
  });

  const [template, setTemplate] = React.useState<string>("");
  const [versionLabel, setVersionLabel] = React.useState("");
  const [history, setHistory] = React.useState<PreviewMessage[]>(DEFAULT_HISTORY);
  const [preview, setPreview] = React.useState<PromptPreview | null>(null);
  const [showRaw, setShowRaw] = React.useState(false);

  React.useEffect(() => {
    if (workspace && template === "") {
      setTemplate(workspace.promptOverride ?? defaultPrompt?.template ?? "");
    }
  }, [workspace, defaultPrompt, template]);

  const stageCount = React.useMemo(() => {
    const map: Record<string, number> = {};
    stageUsage?.forEach((s) => (map[s.stage] = s.count));
    return map;
  }, [stageUsage]);

  const saveMutation = useMutation({
    mutationFn: (body: { promptOverride: string | null }) =>
      api.patch<Workspace>("/me/workspace", body),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: qk.workspace() });
      toast.success(vars.promptOverride ? "Prompt saved" : "Override discarded");
    },
    onError: () => toast.error("Failed to save prompt"),
  });

  const previewMutation = useMutation({
    mutationFn: () =>
      api.post<PromptPreview>("/prompts/preview", {
        template,
        history: history.map((h) => ({
          direction: h.sender === "customer" ? "inbound" : "outbound",
          sender: h.sender,
          body: h.body,
        })),
      }),
    onSuccess: (res) => setPreview(res),
    onError: () => toast.error("Preview failed"),
  });

  const dirty = !!workspace && template !== (workspace.promptOverride ?? "");
  const isOverriding = !!workspace?.promptOverride;

  const handleReset = () => {
    setTemplate(defaultPrompt?.template ?? "");
    setVersionLabel("");
  };

  const handleSave = () => {
    saveMutation.mutate({ promptOverride: template });
  };

  const handleDiscard = () => {
    saveMutation.mutate({ promptOverride: null });
    setTemplate(defaultPrompt?.template ?? "");
    setVersionLabel("");
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Nurture prompt</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Tune how the AI talks to your leads. {isOverriding ? "Using a custom override." : "Using the default template."}
          </p>
        </div>
        <HistoryDialog onRestore={(t) => setTemplate(t)} />
      </div>

      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        {/* Editor */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">System prompt</CardTitle>
            <CardDescription>
              The prompt template sent to GPT-4o on every inbound message. Use{" "}
              <code className="rounded bg-muted px-1">{"{{history}}"}</code> as the placeholder for the conversation log.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="version">Version label</Label>
              <Input
                id="version"
                placeholder="e.g. NIOS v2 — added TOC note"
                value={versionLabel}
                onChange={(e) => setVersionLabel(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="template">Template</Label>
                <span className="text-xs text-muted-foreground">
                  {template.length} chars · ~{approxTokens(template)} tokens
                </span>
              </div>
              <Textarea
                id="template"
                rows={22}
                value={template}
                onChange={(e) => setTemplate(e.target.value)}
                className="font-mono text-xs"
              />
            </div>
            <Separator />
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-xs text-muted-foreground">
                {dirty ? "Unsaved changes" : "All changes saved"}
              </div>
              <div className="flex gap-2">
                <Button variant="outline" onClick={handleReset} disabled={!template || saveMutation.isPending}>
                  <RotateCcw className="mr-2 h-4 w-4" />
                  Reset to default
                </Button>
                {isOverriding && (
                  <Button variant="outline" onClick={handleDiscard} disabled={saveMutation.isPending}>
                    <Trash2 className="mr-2 h-4 w-4" />
                    Discard override
                  </Button>
                )}
                <Button onClick={handleSave} disabled={!dirty || saveMutation.isPending}>
                  {saveMutation.isPending ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Save className="mr-2 h-4 w-4" />
                  )}
                  Save draft
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Stage machine */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Stage machine</CardTitle>
            <CardDescription>
              Eight progressive stages the AI walks each lead through.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ol className="space-y-3">
              {LEAD_STAGES.map((stage, i) => {
                const isLast = i === LEAD_STAGES.length - 1;
                const isComplete = stage === "complete";
                const active = preview?.predictedStage === stage;
                return (
                  <li key={stage} className="flex gap-3">
                    <div className="flex flex-col items-center">
                      <div
                        className={cn(
                          "flex h-7 w-7 items-center justify-center rounded-full border-2 text-xs",
                          active
                            ? "border-primary bg-primary text-primary-foreground"
                            : isComplete
                            ? "border-emerald-500 bg-emerald-500/10 text-emerald-600"
                            : "border-muted bg-background text-muted-foreground"
                        )}
                      >
                        {active ? <Play className="h-3 w-3" /> : isComplete ? <CheckCircle2 className="h-3 w-3" /> : <Circle className="h-3 w-3" />}
                      </div>
                      {!isLast && <div className="mt-1 h-full w-px flex-1 bg-border" />}
                    </div>
                    <div className="flex-1 pb-3">
                      <div className="flex items-center justify-between">
                        <span className="text-sm font-medium">{STAGE_LABELS[stage]}</span>
                        {stageCount[stage] != null && stageCount[stage] > 0 && (
                          <Badge variant="muted">{stageCount[stage]}</Badge>
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-muted-foreground">{STAGE_DESCRIPTIONS[stage]}</p>
                    </div>
                  </li>
                );
              })}
            </ol>
            <Separator className="my-4" />
            <div>
              <h4 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">
                Field glossary
              </h4>
              <ul className="space-y-1 text-xs text-muted-foreground">
                <li><code className="text-foreground">name</code> — customer's name</li>
                <li><code className="text-foreground">class_level</code> — "10th" / "Plus Two" / other</li>
                <li><code className="text-foreground">exam_result</code> — "pass" / "fail" / "appearing"</li>
                <li><code className="text-foreground">stream</code> — Science / Commerce / Arts</li>
                <li><code className="text-foreground">subjects_count</code> — number of failed subjects</li>
                <li><code className="text-foreground">has_certificate</code> — yes / no</li>
                <li><code className="text-foreground">place</code> — exam centre city</li>
              </ul>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Preview */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Conversation preview</CardTitle>
          <CardDescription>
            Edit the bubbles below to simulate a conversation. The local stage machine predicts
            which stage the AI will be in.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Alert variant="info" className="mb-4">
            <AlertTitle>Heads up</AlertTitle>
            <AlertDescription>
              Preview uses the local stage machine only. Live replies are generated by GPT-4o using
              the same prompt above.
            </AlertDescription>
          </Alert>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              {history.map((m, i) => (
                <div key={i} className="space-y-1">
                  <Label className="text-xs capitalize text-muted-foreground">{m.sender}</Label>
                  <Textarea
                    value={m.body}
                    onChange={(e) =>
                      setHistory((h) => h.map((x, j) => (j === i ? { ...x, body: e.target.value } : x)))
                    }
                    rows={2}
                    className="text-sm"
                  />
                </div>
              ))}
              <Button
                onClick={() => previewMutation.mutate()}
                disabled={previewMutation.isPending || !template}
                className="mt-2"
              >
                {previewMutation.isPending ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Play className="mr-2 h-4 w-4" />
                )}
                Re-run preview
              </Button>
            </div>
            <div className="rounded-lg border bg-muted/30 p-4">
              {preview ? (
                <div className="space-y-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="default">
                      Predicted stage: {STAGE_LABELS[preview.predictedStage as LeadStage] ?? preview.predictedStage}
                    </Badge>
                    <Badge variant="muted">Language: {preview.languageHint}</Badge>
                  </div>
                  <Separator />
                  <div>
                    <h4 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">
                      AI reply (preview)
                    </h4>
                    <div className="rounded-md bg-background p-3 text-sm">
                      {preview.compiledPreview || "(no reply produced)"}
                    </div>
                  </div>
                  <Button variant="ghost" size="sm" onClick={() => setShowRaw(true)}>
                    <Code className="mr-2 h-4 w-4" />
                    Show composed prompt
                  </Button>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Click <strong>Re-run preview</strong> to see the AI's predicted reply.
                </p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Dialog open={showRaw} onOpenChange={setShowRaw}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>Composed prompt</DialogTitle>
            <DialogDescription>
              The exact string that would be sent to GPT-4o.
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className="max-h-96">
            <pre className="rounded-md bg-muted p-3 text-xs">{template}</pre>
            <Separator className="my-3" />
            <pre className="rounded-md bg-muted p-3 text-xs">
              {history.map((m, i) => `${i + 1}. [${m.sender}] ${m.body}`).join("\n")}
            </pre>
          </ScrollArea>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRaw(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function HistoryDialog({ onRestore }: { onRestore: (template: string) => void }) {
  const [open, setOpen] = React.useState(false);
  const { data, isLoading } = useQuery({
    queryKey: qk.promptVersions(),
    queryFn: () => api.get<PromptVersion[]>("/prompts/versions"),
    enabled: open,
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">
          <History className="mr-2 h-4 w-4" />
          History
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Prompt version history</DialogTitle>
          <DialogDescription>
            Past templates grouped from the extraction log. Restoring loads the template into the
            editor — you still need to save.
          </DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <Skeleton className="h-32" />
        ) : !data || data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No history yet.</p>
        ) : (
          <ScrollArea className="max-h-96">
            <ul className="space-y-2">
              {data.map((v) => (
                <li key={v.template.slice(0, 32)} className="rounded-lg border p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      First seen {new Date(v.firstSeen).toLocaleString()} · {v.uses} uses
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        onRestore(v.template);
                        setOpen(false);
                      }}
                    >
                      Restore
                      <ChevronRight className="ml-1 h-4 w-4" />
                    </Button>
                  </div>
                  <p className="mt-1 font-mono text-xs text-muted-foreground">
                    {truncate(v.template, 200)}
                  </p>
                </li>
              ))}
            </ul>
          </ScrollArea>
        )}
      </DialogContent>
    </Dialog>
  );
}
