"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

const schema = z.object({
  workspaceName: z.string().min(2, "Min 2 characters"),
  email: z.string().email("Invalid email"),
  password: z.string().min(8, "Min 8 characters"),
});

type FormValues = z.infer<typeof schema>;

export default function SignupPage() {
  const router = useRouter();
  const { signup } = useAuth();
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [devToken, setDevToken] = React.useState<string | null>(null);

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { workspaceName: "", email: "", password: "" },
  });

  const onSubmit = form.handleSubmit(async (values) => {
    setSubmitting(true);
    setError(null);
    const res = await signup({
      email: values.email,
      password: values.password,
      workspaceName: values.workspaceName,
    });
    setSubmitting(false);
    if (res.ok) {
      toast.success("Account created. Check your email.");
      if (res.verifyToken) {
        setDevToken(res.verifyToken);
      }
      router.push(`/verify?email=${encodeURIComponent(values.email)}`);
    } else {
      setError(res.message || "Sign-up failed. Please try again.");
    }
  });

  return (
    <main className="container mx-auto max-w-md py-16">
      <h1 className="text-3xl font-bold">Create your account</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        14-day free trial. No credit card required.
      </p>

      <Card className="mt-8">
        <CardHeader>
          <CardTitle>Get started</CardTitle>
          <CardDescription>Set up your workspace and admin account.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="workspaceName">Workspace name</Label>
              <Input
                id="workspaceName"
                placeholder="Acme Coaching"
                {...form.register("workspaceName")}
              />
              {form.formState.errors.workspaceName && (
                <p className="text-xs text-destructive">
                  {form.formState.errors.workspaceName.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" autoComplete="email" {...form.register("email")} />
              {form.formState.errors.email && (
                <p className="text-xs text-destructive">
                  {form.formState.errors.email.message}
                </p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                autoComplete="new-password"
                {...form.register("password")}
              />
              {form.formState.errors.password && (
                <p className="text-xs text-destructive">
                  {form.formState.errors.password.message}
                </p>
              )}
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertTitle>Sign-up failed</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            {devToken && (
              <Alert variant="info">
                <AlertTitle>Dev mode</AlertTitle>
                <AlertDescription>
                  Verification token (dev only): <code className="font-mono">{devToken}</code>
                </AlertDescription>
              </Alert>
            )}
            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? "Creating account…" : "Create account"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <p className="mt-6 text-sm text-muted-foreground">
        Already have an account?{" "}
        <Link href="/login" className="font-medium text-primary hover:underline">
          Sign in
        </Link>
      </p>
    </main>
  );
}
