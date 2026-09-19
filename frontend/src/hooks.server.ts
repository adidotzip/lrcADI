import type { HandleFetch } from "@sveltejs/kit";

export const handleFetch: HandleFetch = ({ event, request, fetch }) => {
  // CF-Connecting-IP is set by Cloudflare; x-forwarded-for is used by Vercel and other proxies
  const userIP =
    event.request.headers.get("CF-Connecting-IP") ??
    event.request.headers.get("x-forwarded-for")?.split(",")[0]?.trim();
  if (userIP) request.headers.set("X-Real-IP", userIP);

  return fetch(request);
};
