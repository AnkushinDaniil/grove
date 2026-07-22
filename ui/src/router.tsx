import { createBrowserRouter } from "react-router";
import { AppShell } from "./components/shell/AppShell";
import { WelcomeView } from "./components/node/WelcomeView";
import { NodeView } from "./components/node/NodeView";
import { InboxView } from "./components/inbox/InboxView";
import { ReviewsView } from "./components/reviews/ReviewsView";
import { ReviewWorkspace } from "./components/reviewWorkspace/ReviewWorkspace";
import { StatsView } from "./components/stats/StatsView";
import { ProfilesView } from "./components/profiles/ProfilesView";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <WelcomeView /> },
      { path: "n/:id", element: <NodeView /> },
      { path: "inbox", element: <InboxView /> },
      { path: "reviews", element: <ReviewsView /> },
      // `dir` is URL-encoded by the caller (see PRRow's openWorkspace);
      // react-router's useParams decodes it back automatically.
      { path: "review/:dir/:pr", element: <ReviewWorkspace /> },
      // Feedback lives inside StatsView as a second tab, not its own route.
      { path: "stats", element: <StatsView /> },
      { path: "profiles", element: <ProfilesView /> },
    ],
  },
]);
