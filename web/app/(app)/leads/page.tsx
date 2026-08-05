"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { Search, Plus, Loader2, Users } from "lucide-react";
import { useForm } from "react-hook-form";
import { api, type LeadListItem } from "@/lib/api";
import { qk } from "@/lib/queries";
import { LEAD_STATUSES, LEAD_STAGES, STAGE_LABELS, type LeadStatus, type LeadStage } from "@/lib/api";
import { formatPhone, formatRelative } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

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

export default function LeadsPage() {
  const [status, setStatus] = React.useState<string>("all");
  const [searchInput, setSearchInput] = React.useState("");
  const [search, setSearch] = React.useState("");
  const [page, setPage] = React.useState(0);
  const limit = 25;

  // Debounce search input → applied value
  React.useEffect(() => {
    const id = setTimeout(() => setSearch(searchInput.trim()), 300);
    return () => clearTimeout(id);
  }, [searchInput]);

  // Reset page when filters change
  React.useEffect(() => {
    setPage(0);
  }, [status, search]);

  const { data, isLoading, isFetching } = useQuery({
    queryKey: qk.leads({ status, search, page }),
    queryFn: () => {
      const params = new URLSearchParams();
      params.set("limit", String(limit));
      params.set("offset", String(page * limit));
      if (status && status !== "all") params.set("status", status);
      if (search) params.set("search", search);
      return api.get<{ items: LeadListItem[]; total: number }>(`/leads?${params}`);
    },
    placeholderData: keepPreviousData,
  });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / limit));

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Leads</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            One row per WhatsApp phone number. {total} total.
          </p>
        </div>
        <Button asChild>
          <Link href="/onboarding">
            <Plus className="mr-2 h-4 w-4" />
            Add lead
          </Link>
        </Button>
      </div>

      <Card>
        <CardContent className="p-4">
          <div className="flex flex-wrap items-center gap-3">
            <div className="relative min-w-[240px] flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                placeholder="Search phone or name…"
                className="pl-9"
              />
            </div>
            <Select value={status} onValueChange={setStatus}>
              <SelectTrigger className="w-44">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                {LEAD_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {isFetching && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
          </div>
        </CardContent>
      </Card>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Phone</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Stage</TableHead>
              <TableHead>Last inbound</TableHead>
              <TableHead>Last contacted</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 7 }).map((__, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-5 w-20" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-12 text-center text-muted-foreground">
                  <Users className="mx-auto mb-2 h-10 w-10" />
                  <p>No leads yet. Send a message to your linked WhatsApp number to see one appear here.</p>
                </TableCell>
              </TableRow>
            ) : (
              items.map((lead: LeadListItem) => (
                <TableRow key={lead.id} className="cursor-pointer">
                  <TableCell>
                    <Link href={`/leads/${lead.id}`} className="font-medium hover:underline">
                      {formatPhone(lead.phone)}
                    </Link>
                  </TableCell>
                  <TableCell>{lead.name || "—"}</TableCell>
                  <TableCell>
                    <Badge variant={statusVariant(lead.status) as never}>{lead.status}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {STAGE_LABELS[lead.stage as LeadStage] ?? lead.stage}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatRelative(lead.lastInboundAt)}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatRelative(lead.lastContactedAt)}
                  </TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" asChild>
                      <Link href={`/leads/${lead.id}`}>Open</Link>
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            Page {page + 1} of {totalPages}
          </p>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page === 0}>
              Previous
            </Button>
            <Button
              variant="outline"
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={page >= totalPages - 1}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
