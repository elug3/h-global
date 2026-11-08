import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AuthProvider, RequireAuth } from "./auth";
import { Layout } from "./components/Layout";
import { AfterShow } from "./pages/AfterShow";
import { LiveConsole } from "./pages/LiveConsole";
import { Login } from "./pages/Login";
import { OrderDetail } from "./pages/OrderDetail";
import { Orders } from "./pages/Orders";
import { ProductEdit } from "./pages/ProductEdit";
import { Products } from "./pages/Products";
import { Shows } from "./pages/Shows";
import { ShowSetup } from "./pages/ShowSetup";
import { StaffPage } from "./pages/StaffPage";
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
      { path: "/shows/:id/after", element: <AfterShow /> },
      { path: "/orders", element: <Orders /> },
      { path: "/orders/:id", element: <OrderDetail /> },
      { path: "/products", element: <Products /> },
      { path: "/products/:id", element: <ProductEdit /> },
      { path: "/staff", element: <StaffPage /> },
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
