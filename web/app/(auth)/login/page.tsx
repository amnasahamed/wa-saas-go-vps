"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
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
  email: z.string().email("Invalid email"),
  password: z.string().min(8, "Min 8 characters"),
});

type FormValues = z.infer<typeof schema>;

export default function LoginPage() {
  return (
    <React.Suspense fallback={null}>
      <LoginForm />
    </React.Suspense>
  );
}

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { login } = useAuth();
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "" },
  });

  React.useEffect(() => {
    const reason = params.get("reason");
    if (reason === "verify") {
      setError("Please verify your email before signing in.");
    }
  }, [params]);

  const onSubmit = form.handleSubmit(async (values) => {
    setSubmitting(true);
    setError(null);
    const res = await login(values.email, values.password);
    setSubmitting(false);
    if (res.ok) {
      toast.success("Signed in");
      router.push("/");
    } else if (res.code === "email_not_verified") {
      router.push(`/verify?email=${encodeURIComponent(values.email)}`);
    } else if (res.code === "invalid_credentials") {
      setError("Wrong email or password.");
    } else {
      setError("Sign-in failed. Try again.");
    }
  });

  return (
    <main className="container mx-auto max-w-md py-16">
      <h1 className="text-3xl font-bold">Sign in</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Welcome back. Sign in to manage your leads.
      </p>

      <Card className="mt-8">
        <CardHeader>
          <CardTitle>Sign in</CardTitle>
          <CardDescription>Use the email and password you signed up with.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
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
                autoComplete="current-password"
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
                <AlertTitle>Sign-in failed</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <p className="mt-6 text-sm text-muted-foreground">
        New here?{" "}
        <Link href="/signup" className="font-medium text-primary hover:underline">
          Create an account
        </Link>
      </p>
    </main>
  );
}
