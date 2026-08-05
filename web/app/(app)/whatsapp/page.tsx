"use client";

import * as React from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { RefreshCw, ShieldCheck, AlertTriangle, Phone } from "lucide-react";
import { api, type WhatsAppSession } from "@/lib/api";
import { qk } from "@/lib/queries";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
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

export default function WhatsAppPage() {
  const { data, isLoading, refetch } = useQuery({
    queryKey: qk.whatsapp(),
    queryFn: () => api.get<WhatsAppSession>("/whatsapp/session"),
    refetchInterval: 5_000,
  });

  const refreshMutation = useMutation({
    mutationFn: () => api.post<WhatsAppSession>("/whatsapp/session/refresh-qr"),
    onSuccess: () => {
      toast.success("QR refreshed");
      refetch();
    },
    onError: () => toast.error("Failed to refresh QR"),
  });

  if (isLoading || !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (data.status === "authenticated" || data.status === "ready") {
    return (
      <AuthenticatedView
        phoneNumber={data.phoneNumber}
        workerId={data.workerId}
        lastError={data.lastError}
      />
    );
  }

  if (data.status === "not_linked" || data.status === "disconnected") {
    return (
      <NotLinkedView
        onRefresh={() => refreshMutation.mutate()}
        refreshing={refreshMutation.isPending}
      />
    );
  }

  if (data.status === "pending_qr" || data.status === "error") {
    return (
      <QRCard
        qrDataUrl={data.qrDataUrl}
        qrExpiresAt={data.qrExpiresAt}
        lastError={data.lastError}
        onRefresh={() => refreshMutation.mutate()}
        refreshing={refreshMutation.isPending}
      />
    );
  }

  return (
    <Alert>
      <AlertTitle>Unknown status: {data.status}</AlertTitle>
      <AlertDescription>Please contact support.</AlertDescription>
    </Alert>
  );
}

function NotLinkedView({ onRefresh, refreshing }: { onRefresh: () => void; refreshing: boolean }) {
  return (
    <Card className="mx-auto max-w-xl">
      <CardHeader>
        <CardTitle>Link a WhatsApp number</CardTitle>
        <CardDescription>
          Connect a dedicated business number. Don't use your personal number — WhatsApp can ban
          accounts that share a session with their personal app.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Alert variant="info">
          <AlertTitle>How to link</AlertTitle>
          <AlertDescription>
            Open WhatsApp on your phone → Linked devices → Link a device.
          </AlertDescription>
        </Alert>
        <Button onClick={onRefresh} disabled={refreshing} className="w-full">
          {refreshing ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <Phone className="mr-2 h-4 w-4" />}
          Generate QR code
        </Button>
      </CardContent>
    </Card>
  );
}

function QRCard({
  qrDataUrl,
  qrExpiresAt,
  lastError,
  onRefresh,
  refreshing,
}: {
  qrDataUrl: string | null;
  qrExpiresAt: string | null;
  lastError: string | null;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const [secondsLeft, setSecondsLeft] = React.useState<number | null>(null);

  React.useEffect(() => {
    if (!qrExpiresAt) {
      setSecondsLeft(null);
      return;
    }
    const update = () => {
      const ms = new Date(qrExpiresAt).getTime() - Date.now();
      setSecondsLeft(Math.max(0, Math.round(ms / 1000)));
    };
    update();
    const id = setInterval(update, 1000);
    return () => clearInterval(id);
  }, [qrExpiresAt]);

  return (
    <Card className="mx-auto max-w-xl">
      <CardHeader>
        <CardTitle>Scan to link</CardTitle>
        <CardDescription>
          Open WhatsApp on your phone → Linked devices → Link a device.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {lastError && (
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>Last error</AlertTitle>
            <AlertDescription>{lastError}</AlertDescription>
          </Alert>
        )}
        <div className="mx-auto flex h-72 w-72 items-center justify-center rounded-lg border-2 border-dashed bg-background">
          {qrDataUrl ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={qrDataUrl} alt="WhatsApp QR" className="h-64 w-64" />
          ) : (
            <span className="text-sm text-muted-foreground">Generating QR…</span>
          )}
        </div>
        {secondsLeft != null && (
          <p className="text-center text-xs text-muted-foreground">
            Expires in {Math.floor(secondsLeft / 60)}:{String(secondsLeft % 60).padStart(2, "0")}
          </p>
        )}
        <div className="flex justify-center">
          <Button variant="outline" onClick={onRefresh} disabled={refreshing}>
            {refreshing ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
            Refresh QR
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function AuthenticatedView({
  phoneNumber,
  workerId,
  lastError,
}: {
  phoneNumber: string | null;
  workerId: string | null;
  lastError: string | null;
}) {
  return (
    <Card className="mx-auto max-w-xl">
      <CardHeader>
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-emerald-600" />
          <CardTitle>Connected</CardTitle>
        </div>
        <CardDescription>
          Your WhatsApp number is linked and receiving messages.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Phone</span>
            <span className="font-medium">{phoneNumber ?? "—"}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Worker</span>
            <span className="font-mono text-xs">{workerId ?? "—"}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Status</span>
            <Badge variant="success">Ready</Badge>
          </div>
        </div>
        {lastError && (
          <Alert variant="warning">
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>Last warning</AlertTitle>
            <AlertDescription>{lastError}</AlertDescription>
          </Alert>
        )}
        <DisconnectButton />
      </CardContent>
    </Card>
  );
}

function DisconnectButton() {
  const [open, setOpen] = React.useState(false);
  const mutation = useMutation({
    mutationFn: () => api.post<{ ok: true }>("/whatsapp/session/disconnect"),
    onSuccess: () => {
      toast.success("Disconnected");
      setOpen(false);
    },
    onError: () => toast.error("Failed to disconnect"),
  });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" className="w-full">
          Disconnect
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Disconnect WhatsApp?</DialogTitle>
          <DialogDescription>
            Inbound messages will stop being captured. You can re-link the number at any time.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
          >
            Disconnect
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
