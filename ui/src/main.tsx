import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router/dom";
import { AuthGate } from "./AuthGate";
import { router } from "./router";
import { listenForPushNavigation } from "./state/push";
import "./index.css";

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("#root element missing from index.html");

// Lets a notification click deep-link to /n/<node_id> even when a grove tab
// is already open and focused -- see public/sw.js's notificationclick
// handler and state/push.ts's listenForPushNavigation doc comment.
listenForPushNavigation(router);

createRoot(rootEl).render(
  <StrictMode>
    <AuthGate>
      <RouterProvider router={router} />
    </AuthGate>
  </StrictMode>,
);
