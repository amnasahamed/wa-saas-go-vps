import Link from "next/link";
import { Button } from "@/components/ui/button";

export default function HomePage() {
  return (
    <main className="container mx-auto max-w-4xl py-16">
      <h1 className="text-5xl font-bold tracking-tight">
        WhatsApp CRM, on autopilot.
      </h1>
      <p className="mt-6 text-lg text-muted-foreground">
        Capture every WhatsApp conversation, extract lead data with GPT-4o,
        and sync to Google Sheets, Airtable, or Notion — automatically.
      </p>
      <div className="mt-10 flex gap-4">
        <Button asChild size="lg">
          <Link href="/signup">Start free trial</Link>
        </Button>
        <Button asChild variant="outline" size="lg">
          <Link href="/login">Sign in</Link>
        </Button>
      </div>

      <section className="mt-20 grid gap-8 md:grid-cols-3">
        <Feature
          title="Capture every message"
          body="Link your WhatsApp number in 30 seconds. Inbound and outbound messages flow through one clean pipeline."
        />
        <Feature
          title="Extract CRM data"
          body="GPT-4o reads each conversation and pulls name, class, subjects, intent, lead status — into your schema."
        />
        <Feature
          title="Sync anywhere"
          body="Upsert to Google Sheets, Airtable, or Notion in real time. Match by phone, preserve existing rows."
        />
      </section>
    </main>
  );
}

function Feature({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-lg border bg-card p-6">
      <h3 className="font-semibold">{title}</h3>
      <p className="mt-2 text-sm text-muted-foreground">{body}</p>
    </div>
  );
}
