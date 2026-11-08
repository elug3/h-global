import { useEffect } from "react";
import { isRouteErrorResponse, Links, Meta, Outlet, Scripts, ScrollRestoration } from "react-router";
import type { Route } from "./+types/root";
import { t } from "./i18n/ko";
import "./styles.css";

export const links: Route.LinksFunction = () => [
  { rel: "preconnect", href: "https://cdn.jsdelivr.net" },
  { rel: "stylesheet", href: "https://cdn.jsdelivr.net/gh/orioncactus/pretendard@v1.3.9/dist/web/static/pretendard.min.css" },
];

export function meta() {
  return [{ title: t.app.title }, { name: "description", content: t.app.description }];
}

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ko">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
        <meta name="theme-color" content="#ffffff" />
        <Meta />
        <Links />
      </head>
      <body>
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  // Marks the page as interactive (used by the browser tests to wait for it).
  useEffect(() => {
    document.documentElement.dataset.ready = "1";
  }, []);
  return <Outlet />;
}

export function ErrorBoundary({ error }: Route.ErrorBoundaryProps) {
  const notFound = isRouteErrorResponse(error) && error.status === 404;
  return (
    <main className="page narrow center-col error-page">
      <a className="logo" href="/">
        {t.app.name}
      </a>
      <h1>{notFound ? t.errors.not_found : t.errors.pageTitle}</h1>
      {!notFound && <p className="muted">{error instanceof Error ? error.message : t.errors.generic}</p>}
      <a className="btn btn-pri" href="/">
        {t.done.toHome}
      </a>
    </main>
  );
}
