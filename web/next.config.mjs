/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Emit a self-contained server bundle in .next/standalone so the Docker
  // image can ship without node_modules. Required by docker/web.Dockerfile.
  output: "standalone",
  // The dashboard talks to cmd/api. In dev we proxy /api/v1/* to localhost:8080.
  //
  // In the VPS deployment Caddy handles this route itself (/api/v1/* -> api:8080)
  // so the browser stays same-origin and the HttpOnly nios_session cookie is
  // sent; this rewrite is then only a fallback for local dev.
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: `${process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080"}/v1/:path*`,
      },
    ];
  },
  // Scope file tracing to this directory. The dashboard is a self-contained
  // Next app, so nothing outside web/ needs tracing — and this keeps the
  // .next/standalone layout flat (server.js at its root) instead of mirroring
  // a parent path, which docker/web.Dockerfile depends on.
  outputFileTracingRoot: process.cwd(),
};

export default nextConfig;
