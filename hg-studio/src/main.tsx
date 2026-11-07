import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AuthProvider, RequireAuth } from "./auth";
import { Layout } from "./components/Layout";
import { LiveConsole } from "./pages/LiveConsole";
import { Login } from "./pages/Login";
import { Shows } from "./pages/Shows";
import { ShowSetup } from "./pages/ShowSetup";
import "./styles.css";

const router = createBrowserRouter([
  { path: "/login", element: <Login /> },
  {
    // The live console uses the whole screen, without the left rail.
    path: "/shows/:id/live",
    element: (
      <RequireAuth>
        <LiveConsole />
      </RequireAuth>
    ),
  },
  {
    element: (
      <RequireAuth>
        <Layout />
      </RequireAuth>
    ),
    children: [
      { path: "/shows", element: <Shows /> },
      { path: "/shows/:id/setup", element: <ShowSetup /> },
      { path: "*", element: <Navigate to="/shows" replace /> },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AuthProvider>
      <RouterProvider router={router} />
    </AuthProvider>
  </StrictMode>,
);
