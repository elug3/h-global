import { type RouteConfig, index, layout, route } from "@react-router/dev/routes";

export default [
  // Pages with the site header.
  layout("routes/layout.tsx", [
    index("routes/home.tsx"),
    route("shows/:id", "routes/show.tsx"),
    route("products/:id", "routes/product.tsx"),
    route("orders", "routes/orders.tsx"),
    route("orders/:id", "routes/order.tsx"),
  ]),
  // Focused flows with a back button only.
  route("login", "routes/login.tsx"),
  route("signup", "routes/signup.tsx"),
  route("checkout/:id", "routes/checkout.tsx"),
  route("orders/:id/done", "routes/done.tsx"),
  route("pay/:paymentId", "routes/test-pay.tsx"),
] satisfies RouteConfig;
