import { useEffect, useState } from "react";
import { Outlet, useLocation } from "react-router";
import { TreeRail } from "./TreeRail";
import { StatusBar } from "./StatusBar";
import { MobileTopBar } from "./MobileTopBar";
import { BottomTabs } from "./BottomTabs";
import { CommandPalette } from "../palette/CommandPalette";

export function AppShell() {
  const location = useLocation();
  const [mobileTreeOpen, setMobileTreeOpen] = useState(false);

  // Any navigation (tapping a node, using the command palette, browser
  // back/forward, ...) should close the drawer -- it's a transient overlay,
  // not a persistent nav state.
  useEffect(() => {
    setMobileTreeOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex h-dvh w-full flex-col overflow-hidden bg-canvas text-ink">
      <MobileTopBar onOpenTree={() => setMobileTreeOpen(true)} />
      <div className="flex min-h-0 flex-1">
        <TreeRail mobileOpen={mobileTreeOpen} onMobileClose={() => setMobileTreeOpen(false)} />
        <main className="min-w-0 flex-1 overflow-hidden">
          <Outlet />
        </main>
      </div>
      <StatusBar />
      <BottomTabs onOpenTree={() => setMobileTreeOpen(true)} />
      <CommandPalette />
    </div>
  );
}
