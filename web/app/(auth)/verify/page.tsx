"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

const schema = z.object({
  token: z.string().min(8, "Paste the token from your email"),
});

type FormValues = z.infer<typeof schema>;

export default function VerifyPage() {
  return (
    <React.Suspense fallback={null}>
      <VerifyForm />
    </React.Suspense>
  );
}

function VerifyForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { verifyEmail } = useAuth();
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const email = params.get("email") ?? "";

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { token: params.get("token") ?? "" },
  });

  const onSubmit = form.handleSubmit(async (values) => {
    setSubmitting(true);
    setError(null);
    const res = await verifyEmail(values.token);
    setSubmitting(false);
    if (res.ok) {
      router.push("/login?reason=verified");
    } else if (res.code === "token_invalid") {
      setError("That token is invalid or expired. Request a new one.");
    } else {
      setError("Verification failed. Try again.");
    }
  });

  return (
    <main className="container mx-auto max-w-md py-16">
      <h1 className="text-3xl font-bold">Check your email</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {email
          ? `We sent a verification link to ${email}. Click it, or paste the token below.`
          : "We sent a verification link to your inbox. Click it, or paste the token below."}
      </p>

      <Card className="mt-8">
        <CardHeader>
          <CardTitle>Enter verification token</CardTitle>
          <CardDescription>
            The token is in the email we sent. In dev, it's also returned by the signup endpoint.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="token">Verification token</Label>
              <Input
                id="token"
                placeholder="paste-token-here"
                autoComplete="off"
                {...form.register("token")}
              />
              {form.formState.errors.token && (
                <p className="text-xs text-destructive">
                  {form.formState.errors.token.message}
                </p>
              )}
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertTitle>Verification failed</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? "Verifying…" : "Verify email"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <p className="mt-6 text-sm text-muted-foreground">
        Did not get it?{" "}
        <Link href="/signup" className="font-medium text-primary hover:underline">
          Sign up again
        </Link>
      </p>
    </main>
  );
}
